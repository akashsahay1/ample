// Package core implements api.Backend by orchestrating the runtime packages.
// It is shared by the GUI (AMPLS.exe) and the CLI (ampls.exe).
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampls/internal/apache"
	"ampls/internal/api"
	"ampls/internal/certs"
	"ampls/internal/config"
	"ampls/internal/hosts"
	"ampls/internal/mysql"
	"ampls/internal/paths"
	"ampls/internal/php"
	"ampls/internal/services"
	"ampls/internal/sites"
)

// Version is the AMPLS version, overridden via -ldflags by the binaries.
var Version = "1.0.0"

var _ api.Backend = (*Core)(nil)

type Core struct {
	mu sync.Mutex // serialises config-changing operations and service control

	// BackgroundSync makes ListSites apply newly discovered sites (folders dropped
	// into a parked directory) in a background goroutine. Only long-running
	// processes (the GUI) may enable it: a CLI process would exit mid-restart.
	BackgroundSync bool
	bgSyncing      atomic.Bool

	availMu    sync.Mutex
	avail      []php.Release
	availFetch time.Time

	verMu sync.Mutex
	vers  map[string]exeVersion // exe path -> cached `-v` output

	errMu   sync.Mutex
	lastErr map[string]string // service -> last start error (cleared on success/stop)
}

type exeVersion struct {
	mod     time.Time
	size    int64
	version string
}

func New() *Core { return &Core{} }

// cachedVersion runs fn (which spawns `httpd -v` / `mysqld --version`) only when
// the executable changed, so frequent Overview polling does not spawn processes.
func (c *Core) cachedVersion(exe string, fn func() string) string {
	fi, err := os.Stat(exe)
	if err != nil {
		return ""
	}
	c.verMu.Lock()
	defer c.verMu.Unlock()
	if v, ok := c.vers[exe]; ok && v.mod.Equal(fi.ModTime()) && v.size == fi.Size() {
		return v.version
	}
	s := fn()
	if s != "" {
		if c.vers == nil {
			c.vers = map[string]exeVersion{}
		}
		c.vers[exe] = exeVersion{mod: fi.ModTime(), size: fi.Size(), version: s}
	}
	return s
}

func (c *Core) setServiceErr(name string, err error) {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	if c.lastErr == nil {
		c.lastErr = map[string]string{}
	}
	if err == nil {
		delete(c.lastErr, name)
	} else {
		c.lastErr[name] = err.Error()
	}
}

func (c *Core) serviceErr(name string) string {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr[name]
}

// ---------- helpers ----------

func (c *Core) cfg() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// defaultPHP returns the configured default PHP if installed, else the newest installed.
func defaultPHP(cfg *config.Config) string {
	inst, _ := php.List()
	for _, i := range inst {
		if i.Minor == cfg.DefaultPHP {
			return i.Minor
		}
	}
	if len(inst) > 0 {
		return inst[0].Minor
	}
	return ""
}

func phpInstalled(minor string) bool {
	_, err := os.Stat(php.CGIPath(minor))
	return err == nil
}

func (c *Core) discover(cfg *config.Config) ([]api.Site, error) {
	cfg.DefaultPHP = defaultPHP(cfg)
	return sites.Discover(cfg)
}

// effectivePHP is the version a site's vhost actually uses: its own if
// installed, else the default.
func effectivePHP(s api.Site, def string) string {
	if s.PHP != "" && phpInstalled(s.PHP) {
		return s.PHP
	}
	return def
}

// siteKey fingerprints everything the generated Apache config depends on.
// cfg.DefaultPHP must already be resolved (see discover).
func siteKey(ss []api.Site, cfg *config.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%s|", cfg.Ports.HTTP, cfg.Ports.HTTPS, cfg.DefaultPHP)
	for _, s := range ss {
		fmt.Fprintf(&b, "%s|%s|%s|%t;", s.Domain, s.DocRoot, effectivePHP(s, cfg.DefaultPHP), s.Secure)
	}
	return b.String()
}

// The fingerprint of the config last written to disk is persisted so that every
// process (GUI, CLI) knows whether Apache needs a restart.
func appliedKeyPath() string { return filepath.Join(paths.RunDir(), "sites.applied") }

func appliedKey() string {
	b, err := os.ReadFile(appliedKeyPath())
	if err != nil {
		return ""
	}
	return string(b)
}

func setAppliedKey(k string) {
	if err := os.MkdirAll(paths.RunDir(), 0o755); err == nil {
		_ = os.WriteFile(appliedKeyPath(), []byte(k), 0o644)
	}
}

// sync regenerates Apache config, site certs and hosts entries from config.
// restart: restart Apache if it is running and the generated config changed.
func (c *Core) sync(restart bool) error { return c.syncOpts(restart, true) }

func (c *Core) syncOpts(restart, requestHosts bool) error {
	cfg, err := c.cfg()
	if err != nil {
		return err
	}
	ss, err := c.discover(cfg)
	if err != nil {
		return err
	}
	def := cfg.DefaultPHP
	var vhosts []apache.VHost
	var domains []string
	for _, s := range ss {
		ver := effectivePHP(s, def)
		v := apache.VHost{
			Domain:  s.Domain,
			Aliases: []string{"*." + s.Domain},
			DocRoot: s.DocRoot,
			PHPCGI:  php.CGIPath(ver),
			Secure:  s.Secure,
		}
		if s.Secure {
			if err := certs.EnsureCA(); err != nil {
				return fmt.Errorf("certificate authority: %w", err)
			}
			v.CertFile, v.KeyFile, err = certs.EnsureSiteCert(s.Domain)
			if err != nil {
				return fmt.Errorf("certificate for %s: %w", s.Domain, err)
			}
		}
		vhosts = append(vhosts, v)
		domains = append(domains, s.Domain)
	}
	defCGI := ""
	if def != "" {
		defCGI = php.CGIPath(def)
	}
	if err := apache.WriteConfig(apache.Options{HTTPPort: cfg.Ports.HTTP, HTTPSPort: cfg.Ports.HTTPS, DefaultPHPCGI: defCGI}, vhosts); err != nil {
		return err
	}
	if requestHosts {
		if err := hosts.Request(domains, cfg.TLD); err != nil {
			// Not fatal: sites still work via http://127.0.0.1 with Host header, and the user can retry.
			fmt.Fprintln(os.Stderr, "ampls: hosts update failed:", err)
		}
	}
	key := siteKey(ss, cfg)
	changed := key != appliedKey()
	setAppliedKey(key)
	if restart && changed && services.Status(api.ServiceApache).Running {
		return c.restartApache()
	}
	return nil
}

// ---------- overview & services ----------

func (c *Core) Overview() (api.Overview, error) {
	cfg, err := c.cfg()
	if err != nil {
		return api.Overview{}, err
	}
	o := api.Overview{Home: paths.Home(), AppVersion: Version, DefaultPHP: defaultPHP(cfg), CATrusted: certs.IsCATrusted()}
	for _, n := range []string{api.ServiceApache, api.ServiceMySQL} {
		o.Services = append(o.Services, c.serviceStatus(n, cfg))
	}
	inst, _ := php.List()
	for _, i := range inst {
		o.PHPVersions = append(o.PHPVersions, i.Minor)
	}
	if ss, err := c.discover(cfg); err == nil {
		o.SiteCount = len(ss)
	}
	return o, nil
}

func (c *Core) serviceStatus(name string, cfg *config.Config) api.ServiceStatus {
	st := services.Status(name)
	s := api.ServiceStatus{Name: name, Running: st.Running, PID: st.PID}
	if !st.Running {
		s.Error = c.serviceErr(name)
	}
	switch name {
	case api.ServiceApache:
		s.Version = c.cachedVersion(apache.HttpdPath(), apache.Version)
		s.Ports = []int{cfg.Ports.HTTP, cfg.Ports.HTTPS}
	case api.ServiceMySQL:
		s.Version = c.cachedVersion(mysql.MysqldPath(), mysql.Version)
		s.Ports = []int{cfg.Ports.MySQL}
	}
	return s
}

func serviceTitle(name string) string {
	switch name {
	case api.ServiceApache:
		return "Apache"
	case api.ServiceMySQL:
		return "MySQL"
	}
	return name
}

// each runs fn for every service, prefixing errors with the service name.
func each(fn func(string) error, names ...string) error {
	var errs []error
	for _, n := range names {
		if err := fn(n); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", serviceTitle(n), err))
		}
	}
	return errors.Join(errs...)
}

func (c *Core) StartAll() error {
	return each(c.StartService, api.ServiceMySQL, api.ServiceApache)
}

func (c *Core) StopAll() error {
	return each(c.StopService, api.ServiceApache, api.ServiceMySQL)
}

func (c *Core) RestartAll() error {
	return each(c.RestartService, api.ServiceMySQL, api.ServiceApache)
}

func (c *Core) StartService(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.start(name)
}

func (c *Core) StopService(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.stop(name)
	if err == nil {
		c.setServiceErr(name, nil)
	}
	return err
}

func (c *Core) RestartService(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stop(name); err != nil {
		return err
	}
	return c.start(name)
}

func (c *Core) restartApache() error {
	if err := c.stop(api.ServiceApache); err != nil {
		return err
	}
	return c.start(api.ServiceApache)
}

func portFree(port int, what string) error {
	if used, owner := services.PortInUse(port); used {
		if owner != "" {
			return fmt.Errorf("%s port %d is already in use by %s", what, port, owner)
		}
		return fmt.Errorf("%s port %d is already in use by another program", what, port)
	}
	return nil
}

func (c *Core) start(name string) error {
	if services.Status(name).Running {
		c.setServiceErr(name, nil)
		return nil
	}
	err := c.startService(name)
	c.setServiceErr(name, err)
	return err
}

func (c *Core) startService(name string) error {
	cfg, err := c.cfg()
	if err != nil {
		return err
	}
	switch name {
	case api.ServiceApache:
		if _, err := os.Stat(apache.HttpdPath()); err != nil {
			return errors.New("Apache is not installed in the AMPLS data directory")
		}
		if defaultPHP(cfg) == "" {
			return errors.New("no PHP version is installed: install one from PHP Versions first")
		}
		if err := c.sync(false); err != nil {
			return err
		}
		if err := apache.ConfigTest(); err != nil {
			return fmt.Errorf("apache config test failed: %w", err)
		}
		if err := portFree(cfg.Ports.HTTP, "HTTP"); err != nil {
			return err
		}
		if ss, err := c.discover(cfg); err == nil {
			for _, s := range ss {
				if s.Secure { // httpd.conf only listens on HTTPS when a site is secured
					if err := portFree(cfg.Ports.HTTPS, "HTTPS"); err != nil {
						return err
					}
					break
				}
			}
		}
		if err := services.Start(name, apache.HttpdPath(), apache.StartArgs(), logPath("apache-stdout.log")); err != nil {
			return err
		}
		if err := services.WaitPort(cfg.Ports.HTTP, 15*time.Second); err != nil {
			return fmt.Errorf("%w (see the apache-error log)", err)
		}
		return nil
	case api.ServiceMySQL:
		if _, err := os.Stat(mysql.MysqldPath()); err != nil {
			return errors.New("MySQL is not installed in the AMPLS data directory")
		}
		if err := mysql.WriteConfig(cfg.Ports.MySQL); err != nil {
			return err
		}
		if !mysql.Initialized() {
			if err := mysql.Initialize(); err != nil {
				return fmt.Errorf("initialize MySQL: %w", err)
			}
		}
		if err := portFree(cfg.Ports.MySQL, "MySQL"); err != nil {
			return err
		}
		if err := services.Start(name, mysql.MysqldPath(), mysql.StartArgs(), logPath("mysql-stdout.log")); err != nil {
			return err
		}
		if err := services.WaitPort(cfg.Ports.MySQL, 60*time.Second); err != nil {
			return fmt.Errorf("%w (see the mysql log)", err)
		}
		return nil
	}
	return fmt.Errorf("unknown service %q", name)
}

func (c *Core) stop(name string) error {
	if !services.Status(name).Running {
		return nil
	}
	var graceful func() error
	if name == api.ServiceMySQL {
		cfg, err := c.cfg()
		if err == nil {
			graceful = func() error { return mysql.Shutdown(cfg.Ports.MySQL, cfg.MySQL.RootPassword) }
		}
	}
	return services.Stop(name, graceful)
}

// ---------- sites ----------

func (c *Core) ListSites() ([]api.Site, error) {
	cfg, err := c.cfg()
	if err != nil {
		return nil, err
	}
	ss, err := c.discover(cfg)
	if err != nil {
		return nil, err
	}
	// New folders dropped into a parked directory: apply them in the background
	// (at most one sync in flight; sync itself restarts Apache only on change).
	if c.BackgroundSync && siteKey(ss, cfg) != appliedKey() && c.bgSyncing.CompareAndSwap(false, true) {
		go func() {
			defer c.bgSyncing.Store(false)
			c.mu.Lock()
			defer c.mu.Unlock()
			_ = c.sync(true)
		}()
	}
	return ss, nil
}

func (c *Core) mutate(fn func(cfg *config.Config) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := config.Update(fn); err != nil {
		return err
	}
	return c.sync(true)
}

func samePath(a, b string) bool {
	return strings.EqualFold(cleanPath(a), cleanPath(b))
}

func (c *Core) Park(dir string) error {
	dir = cleanPath(dir)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return c.mutate(func(cfg *config.Config) error {
		for _, p := range cfg.Parked {
			if samePath(p, dir) {
				return nil
			}
		}
		cfg.Parked = append(cfg.Parked, dir)
		return nil
	})
}

func (c *Core) Unpark(dir string) error {
	return c.mutate(func(cfg *config.Config) error {
		out := cfg.Parked[:0]
		for _, p := range cfg.Parked {
			if !samePath(p, dir) {
				out = append(out, p)
			}
		}
		cfg.Parked = out
		return nil
	})
}

func (c *Core) Link(name, path string) error {
	path = cleanPath(path)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if name == "" {
		name = sites.Slug(baseName(path))
	}
	if !sites.ValidName(name) {
		return fmt.Errorf("invalid site name %q: use lowercase letters, digits and dashes", name)
	}
	return c.mutate(func(cfg *config.Config) error {
		for i, l := range cfg.Links {
			if l.Name == name {
				cfg.Links[i].Path = path
				return nil
			}
		}
		cfg.Links = append(cfg.Links, config.Link{Name: name, Path: path})
		return nil
	})
}

func (c *Core) Unlink(name string) error {
	return c.mutate(func(cfg *config.Config) error {
		out := cfg.Links[:0]
		found := false
		for _, l := range cfg.Links {
			if l.Name == name {
				found = true
				continue
			}
			out = append(out, l)
		}
		if !found {
			return fmt.Errorf("no linked site named %q", name)
		}
		cfg.Links = out
		return nil
	})
}

func (c *Core) siteSettings(site string, fn func(s *config.SiteSettings)) error {
	return c.mutate(func(cfg *config.Config) error {
		if _, ok := sites.Find(cfg, site); !ok {
			return fmt.Errorf("no site named %q", site)
		}
		s := cfg.Sites[site]
		fn(&s)
		if s == (config.SiteSettings{}) {
			delete(cfg.Sites, site)
		} else {
			cfg.Sites[site] = s
		}
		return nil
	})
}

func (c *Core) SetSitePHP(site, version string) error {
	if version != "" && !phpInstalled(version) {
		return fmt.Errorf("PHP %s is not installed", version)
	}
	return c.siteSettings(site, func(s *config.SiteSettings) { s.PHP = version })
}

func (c *Core) SetSiteSecure(site string, secure bool) error {
	return c.siteSettings(site, func(s *config.SiteSettings) { s.Secure = secure })
}

// ---------- PHP ----------

func (c *Core) available() []php.Release {
	c.availMu.Lock()
	defer c.availMu.Unlock()
	if c.avail != nil && time.Since(c.availFetch) < 10*time.Minute {
		return c.avail
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rel, err := php.Available(ctx)
	if err != nil {
		return c.avail // offline: keep whatever we had
	}
	c.avail, c.availFetch = rel, time.Now()
	return rel
}

func (c *Core) ListPHP() ([]api.PHPVersion, error) {
	cfg, err := c.cfg()
	if err != nil {
		return nil, err
	}
	def := defaultPHP(cfg)
	counts := map[string]int{}
	if ss, err := c.discover(cfg); err == nil {
		for _, s := range ss {
			counts[s.PHP]++
		}
	}
	byMinor := map[string]*api.PHPVersion{}
	inst, err := php.List()
	if err != nil {
		return nil, err
	}
	for _, i := range inst {
		byMinor[i.Minor] = &api.PHPVersion{Version: i.Minor, Full: i.Full, Installed: true}
	}
	for _, r := range c.available() {
		if v, ok := byMinor[r.Minor]; ok {
			v.DownloadSize = r.Size
			continue
		}
		byMinor[r.Minor] = &api.PHPVersion{Version: r.Minor, Full: r.Full, DownloadSize: r.Size}
	}
	out := make([]api.PHPVersion, 0, len(byMinor))
	for _, v := range byMinor {
		v.Default = v.Version == def
		v.EOL = php.IsEOL(v.Version)
		v.SiteCount = counts[v.Version]
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return php.Compare(out[i].Version, out[j].Version) > 0 })
	return out, nil
}

func (c *Core) InstallPHP(version string, progress api.ProgressFunc) error {
	task := "php:install:" + version
	report := func(p api.Progress) {
		p.Task = task
		if progress != nil {
			progress(p)
		}
	}
	install := func() error {
		return php.Install(context.Background(), version, func(msg string, pct float64) {
			report(api.Progress{Message: msg, Percent: pct})
		})
	}
	err := install()
	if err != nil && phpInstalled(version) && strings.Contains(err.Error(), "in use") {
		// Reinstalling a version Apache's php-cgi processes are using: stop Apache,
		// retry (the verified download is cached) and start it again.
		c.mu.Lock()
		if services.Status(api.ServiceApache).Running {
			report(api.Progress{Message: "Stopping Apache to replace PHP " + version, Percent: -1})
			if serr := c.stop(api.ServiceApache); serr == nil {
				err = install()
				_ = c.start(api.ServiceApache) // a failure is surfaced in the service status
			}
		}
		c.mu.Unlock()
	}
	if err == nil {
		c.mu.Lock()
		_, err = config.Update(func(cfg *config.Config) error {
			if cfg.DefaultPHP == "" || !phpInstalled(cfg.DefaultPHP) {
				cfg.DefaultPHP = version
			}
			return nil
		})
		if err == nil {
			// Sites pinned to this version (or everything, if it is the first PHP)
			// now resolve differently.
			err = c.sync(true)
		}
		c.mu.Unlock()
	}
	if err != nil {
		report(api.Progress{Done: true, Error: err.Error(), Percent: 100})
		return err
	}
	report(api.Progress{Done: true, Message: "PHP " + version + " installed", Percent: 100})
	return nil
}

func (c *Core) RemovePHP(version string) error {
	cfg, err := c.cfg()
	if err != nil {
		return err
	}
	if !phpInstalled(version) {
		return fmt.Errorf("PHP %s is not installed", version)
	}
	if defaultPHP(cfg) == version {
		return fmt.Errorf("PHP %s is the default version: make another version the default first", version)
	}
	var pinned []string
	for name, s := range cfg.Sites {
		if s.PHP == version {
			pinned = append(pinned, name)
		}
	}
	if len(pinned) > 0 {
		sort.Strings(pinned)
		return fmt.Errorf("PHP %s is used by %s: switch those sites to another version first", version, strings.Join(pinned, ", "))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !services.Status(api.ServiceApache).Running {
		return php.Remove(version)
	}
	// php-cgi.exe processes of this version hold file locks.
	if err := c.stop(api.ServiceApache); err != nil {
		return err
	}
	err = php.Remove(version)
	if serr := c.start(api.ServiceApache); serr != nil {
		err = errors.Join(err, fmt.Errorf("restart Apache: %w", serr))
	}
	return err
}

func (c *Core) SetDefaultPHP(version string) error {
	if !phpInstalled(version) {
		return fmt.Errorf("PHP %s is not installed", version)
	}
	return c.mutate(func(cfg *config.Config) error {
		cfg.DefaultPHP = version
		return nil
	})
}

func (c *Core) GetPHPSettings(version string) (api.PHPSettings, error) {
	s, err := php.ReadSettings(version)
	if err != nil {
		return api.PHPSettings{}, err
	}
	out := api.PHPSettings{Version: version, Ini: s.Values}
	for _, e := range s.Extensions {
		out.Extensions = append(out.Extensions, api.Extension{Name: e.Name, Enabled: e.Enabled})
	}
	return out, nil
}

func (c *Core) SavePHPSettings(s api.PHPSettings) error {
	ps := php.Settings{Values: s.Ini}
	for _, e := range s.Extensions {
		ps.Extensions = append(ps.Extensions, php.Ext{Name: e.Name, Enabled: e.Enabled})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := php.WriteSettings(s.Version, ps); err != nil {
		return err
	}
	if services.Status(api.ServiceApache).Running {
		return c.restartApache() // respawn php-cgi with the new ini
	}
	return nil
}

func (c *Core) PHPIniPath(version string) string { return php.IniPath(version) }

// ---------- settings ----------

func (c *Core) GetSettings() (api.Settings, error) {
	cfg, err := c.cfg()
	if err != nil {
		return api.Settings{}, err
	}
	return api.Settings{
		TLD: cfg.TLD, HTTPPort: cfg.Ports.HTTP, HTTPSPort: cfg.Ports.HTTPS, MySQLPort: cfg.Ports.MySQL,
		Parked:                append([]string{}, cfg.Parked...),
		StartServicesOnLaunch: cfg.App.StartServicesOnLaunch,
		StopServicesOnQuit:    cfg.App.StopServicesOnQuit,
		LaunchAtLogin:         cfg.App.LaunchAtLogin || launchAtLoginEnabled(),
		Home:                  paths.Home(),
	}, nil
}

func (c *Core) SaveSettings(s api.Settings) error {
	for _, p := range []int{s.HTTPPort, s.HTTPSPort, s.MySQLPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("invalid port %d", p)
		}
	}
	if s.HTTPPort == s.HTTPSPort || s.HTTPPort == s.MySQLPort || s.HTTPSPort == s.MySQLPort {
		return errors.New("the HTTP, HTTPS and MySQL ports must all be different")
	}
	parked := []string{}
	for _, p := range s.Parked {
		if strings.TrimSpace(p) != "" {
			parked = append(parked, cleanPath(p))
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	old, err := c.cfg()
	if err != nil {
		return err
	}
	// Stop MySQL while the config still has the old port so the graceful
	// shutdown reaches it (instead of killing mysqld).
	restartMySQL := false
	if old.Ports.MySQL != s.MySQLPort && services.Status(api.ServiceMySQL).Running {
		if err := c.stop(api.ServiceMySQL); err != nil {
			return err
		}
		restartMySQL = true
	}
	_, err = config.Update(func(cfg *config.Config) error {
		cfg.Ports = config.Ports{HTTP: s.HTTPPort, HTTPS: s.HTTPSPort, MySQL: s.MySQLPort}
		cfg.Parked = parked
		cfg.App = config.AppSettings{
			StartServicesOnLaunch: s.StartServicesOnLaunch,
			StopServicesOnQuit:    s.StopServicesOnQuit,
			LaunchAtLogin:         s.LaunchAtLogin,
		}
		return nil
	})
	if err == nil {
		err = c.sync(true) // restarts Apache only if ports/sites changed
	}
	if restartMySQL {
		if serr := c.start(api.ServiceMySQL); serr != nil {
			err = errors.Join(err, fmt.Errorf("MySQL: %w", serr))
		}
	}
	if err != nil {
		return err
	}
	if err := setLaunchAtLogin(s.LaunchAtLogin); err != nil {
		return fmt.Errorf("launch at login: %w", err)
	}
	return nil
}

func (c *Core) TrustCA() error {
	if err := certs.EnsureCA(); err != nil {
		return err
	}
	return certs.TrustCA(false)
}
