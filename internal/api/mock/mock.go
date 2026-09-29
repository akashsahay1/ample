// Package mock provides an in-memory api.Backend used for GUI development
// (AMPLS_MOCK=1). State mutations persist for the lifetime of the process.
package mock

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ampls/internal/api"
)

type service struct {
	running bool
	pid     int
	version string
	ports   []int
}

type phpEntry struct {
	full      string
	installed bool
	eol       bool
	size      int64
	settings  api.PHPSettings
}

type siteEntry struct {
	name, path, framework string
	php                   string // "" = default
	secure, linked        bool
}

// Backend is the in-memory mock.
type Backend struct {
	mu       sync.Mutex
	services map[string]*service
	php      map[string]*phpEntry
	def      string
	sites    []*siteEntry
	parked   []string
	dbs      []api.Database
	password string
	settings api.Settings
	trusted  bool
	nextPID  int
}

var _ api.Backend = (*Backend)(nil)

const home = `C:\AMPLS`

var sitesDir = `C:\Users\dev\AMPLS\Sites`

var validNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func defaultExts(xdebug bool) []api.Extension {
	names := []string{"bcmath", "curl", "fileinfo", "gd", "intl", "mbstring", "mysqli", "opcache", "openssl", "pdo_mysql", "pdo_sqlite", "sodium", "sqlite3", "xdebug", "zip"}
	out := make([]api.Extension, 0, len(names))
	for _, n := range names {
		en := n != "xdebug" && n != "sodium" && n != "pdo_sqlite" && n != "sqlite3"
		if n == "xdebug" {
			en = xdebug
		}
		out = append(out, api.Extension{Name: n, Enabled: en})
	}
	return out
}

func defaultIni(ver string) api.PHPSettings {
	return api.PHPSettings{
		Version: ver,
		Ini: map[string]string{
			"memory_limit":        "512M",
			"upload_max_filesize": "128M",
			"post_max_size":       "128M",
			"max_execution_time":  "120",
			"display_errors":      "On",
		},
		Extensions: defaultExts(false),
	}
}

// New returns a mock Backend seeded with data matching the design mockups.
func New() *Backend {
	b := &Backend{
		services: map[string]*service{
			api.ServiceApache: {running: true, pid: 11824, version: "2.4.65", ports: []int{80, 443}},
			api.ServiceMySQL:  {running: true, pid: 9920, version: "8.4.6", ports: []int{3306}},
		},
		php: map[string]*phpEntry{
			"8.5": {full: "8.5.0", installed: true, size: 33 << 20},
			"8.4": {full: "8.4.13", size: 32 << 20},
			"8.3": {full: "8.3.26", installed: true, size: 31 << 20},
			"8.2": {full: "8.2.29", size: 30 << 20},
			"8.1": {full: "8.1.33", eol: true, size: 29 << 20},
			"7.4": {full: "7.4.33", installed: true, eol: true, size: 26 << 20},
		},
		def:    "8.5",
		parked: []string{sitesDir, `D:\work`},
		sites: []*siteEntry{
			{name: "blog", path: sitesDir + `\blog`, framework: "laravel", secure: true},
			{name: "shop", path: sitesDir + `\shop`, framework: "wordpress", php: "8.3", secure: true},
			{name: "legacy-crm", path: `D:\work\legacy-crm`, framework: "php", php: "7.4", linked: true},
			{name: "api", path: `D:\work\api`, framework: "laravel"},
		},
		dbs: []api.Database{
			{Name: "blog", Tables: 14, SizeBytes: 2_200_000},
			{Name: "shop_wp", Tables: 12, SizeBytes: 19_300_000},
			{Name: "legacy_crm", Tables: 41, SizeBytes: 100_600_000},
			{Name: "api", Tables: 9, SizeBytes: 840_000},
		},
		password: "",
		settings: api.Settings{
			TLD: "test", HTTPPort: 80, HTTPSPort: 443, MySQLPort: 3306,
			StartServicesOnLaunch: true, StopServicesOnQuit: true, Home: home,
		},
		nextPID: 12000,
	}
	for v, p := range b.php {
		p.eol = p.eol || v == "7.4"
		if p.installed {
			p.settings = defaultIni(v)
		}
	}
	return b
}

func (b *Backend) installedLocked() []string {
	var out []string
	for v, p := range b.php {
		if p.installed {
			out = append(out, v)
		}
	}
	sortVersions(out)
	return out
}

func sortVersions(v []string) {
	sort.Slice(v, func(i, j int) bool { return compare(v[i], v[j]) > 0 })
}

func compare(a, b string) int {
	var a1, a2, b1, b2 int
	fmt.Sscanf(a, "%d.%d", &a1, &a2)
	fmt.Sscanf(b, "%d.%d", &b1, &b2)
	if a1 != b1 {
		return a1 - b1
	}
	return a2 - b2
}

func (b *Backend) Overview() (api.Overview, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	svcs := []api.ServiceStatus{b.statusLocked(api.ServiceApache), b.statusLocked(api.ServiceMySQL)}
	return api.Overview{
		Services:    svcs,
		DefaultPHP:  b.def,
		PHPVersions: b.installedLocked(),
		SiteCount:   len(b.sites),
		Home:        home,
		AppVersion:  "1.0.0",
		CATrusted:   b.trusted,
	}, nil
}

func (b *Backend) statusLocked(name string) api.ServiceStatus {
	s := b.services[name]
	st := api.ServiceStatus{Name: name, Running: s.running, Version: s.version, Ports: s.ports}
	if s.running {
		st.PID = s.pid
	}
	return st
}

func (b *Backend) setRunning(name string, on bool) error {
	time.Sleep(400 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.services[name]
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	if on && !s.running {
		b.nextPID += 4
		s.pid = b.nextPID
	}
	s.running = on
	return nil
}

func (b *Backend) StartAll() error {
	if err := b.setRunning(api.ServiceMySQL, true); err != nil {
		return err
	}
	return b.setRunning(api.ServiceApache, true)
}
func (b *Backend) StopAll() error {
	if err := b.setRunning(api.ServiceApache, false); err != nil {
		return err
	}
	return b.setRunning(api.ServiceMySQL, false)
}
func (b *Backend) RestartAll() error {
	if err := b.StopAll(); err != nil {
		return err
	}
	return b.StartAll()
}
func (b *Backend) StartService(name string) error { return b.setRunning(name, true) }
func (b *Backend) StopService(name string) error  { return b.setRunning(name, false) }
func (b *Backend) RestartService(name string) error {
	if err := b.setRunning(name, false); err != nil {
		return err
	}
	return b.setRunning(name, true)
}

func (b *Backend) siteLocked(s *siteEntry) api.Site {
	php := s.php
	if php == "" {
		php = b.def
	}
	scheme := "http"
	if s.secure {
		scheme = "https"
	}
	domain := s.name + "." + b.settings.TLD
	doc := s.path
	if s.framework == "laravel" {
		doc = s.path + `\public`
	}
	return api.Site{
		Name: s.name, Domain: domain, URL: scheme + "://" + domain, Path: s.path, DocRoot: doc,
		PHP: php, Isolated: s.php != "", Secure: s.secure, Linked: s.linked, Framework: s.framework,
	}
}

func (b *Backend) ListSites() ([]api.Site, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]api.Site, 0, len(b.sites))
	for _, s := range b.sites {
		out = append(out, b.siteLocked(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (b *Backend) findLocked(name string) *siteEntry {
	for _, s := range b.sites {
		if s.name == name {
			return s
		}
	}
	return nil
}

func (b *Backend) Park(dir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range b.parked {
		if strings.EqualFold(p, dir) {
			return fmt.Errorf("%s is already parked", dir)
		}
	}
	b.parked = append(b.parked, dir)
	// pretend the directory contains one project
	name := slug(filepath.Base(dir)) + "-demo"
	if b.findLocked(name) == nil && validName(name) {
		b.sites = append(b.sites, &siteEntry{name: name, path: dir + `\` + name, framework: "php"})
	}
	return nil
}

func validName(n string) bool { return validNameRe.MatchString(n) }

func (b *Backend) Unpark(dir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	idx := -1
	for i, p := range b.parked {
		if strings.EqualFold(p, dir) {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("%s is not parked", dir)
	}
	b.parked = append(b.parked[:idx], b.parked[idx+1:]...)
	kept := b.sites[:0]
	for _, s := range b.sites {
		if !s.linked && strings.EqualFold(filepath.Dir(s.path), dir) {
			continue
		}
		kept = append(kept, s)
	}
	b.sites = kept
	return nil
}

func (b *Backend) Link(name, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == "" {
		name = slug(filepath.Base(path))
	}
	if !validName(name) {
		return fmt.Errorf("invalid site name %q", name)
	}
	if b.findLocked(name) != nil {
		return fmt.Errorf("a site named %q already exists", name)
	}
	b.sites = append(b.sites, &siteEntry{name: name, path: path, framework: "php", linked: true})
	return nil
}

func (b *Backend) Unlink(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, s := range b.sites {
		if s.name == name {
			if !s.linked {
				return fmt.Errorf("%s is in a parked directory and cannot be unlinked", name)
			}
			b.sites = append(b.sites[:i], b.sites[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("site %q not found", name)
}

func (b *Backend) SetSitePHP(site, version string) error {
	time.Sleep(300 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.findLocked(site)
	if s == nil {
		return fmt.Errorf("site %q not found", site)
	}
	if version != "" {
		if p, ok := b.php[version]; !ok || !p.installed {
			return fmt.Errorf("PHP %s is not installed", version)
		}
	}
	s.php = version
	return nil
}

func (b *Backend) SetSiteSecure(site string, secure bool) error {
	time.Sleep(300 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.findLocked(site)
	if s == nil {
		return fmt.Errorf("site %q not found", site)
	}
	s.secure = secure
	return nil
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			dash = false
		} else if !dash && sb.Len() > 0 {
			sb.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}

func simulate(task string, steps []string, total time.Duration, progress api.ProgressFunc) {
	if progress == nil {
		progress = func(api.Progress) {}
	}
	n := 20
	for i := 0; i <= n; i++ {
		pct := float64(i) * 100 / float64(n)
		msg := steps[min(i*len(steps)/(n+1), len(steps)-1)]
		progress(api.Progress{Task: task, Message: msg, Percent: pct})
		time.Sleep(total / time.Duration(n))
	}
}

func (b *Backend) NewProject(req api.NewProjectRequest, progress api.ProgressFunc) (api.Site, error) {
	task := "project:" + req.Name
	fail := func(err error) (api.Site, error) {
		if progress != nil {
			progress(api.Progress{Task: task, Done: true, Error: err.Error(), Percent: 100})
		}
		return api.Site{}, err
	}
	if !validName(req.Name) {
		return fail(fmt.Errorf("invalid project name %q (use lowercase letters, digits and dashes)", req.Name))
	}
	b.mu.Lock()
	exists := b.findLocked(req.Name) != nil
	b.mu.Unlock()
	if exists {
		return fail(fmt.Errorf("a site named %q already exists", req.Name))
	}
	var steps []string
	fw := "php"
	switch req.Kind {
	case api.KindLaravel:
		fw = "laravel"
		steps = []string{"Running composer create-project laravel/laravel…", "Installing dependencies…", "Generating application key…", "Configuring .env…"}
	case api.KindWordPress:
		fw = "wordpress"
		steps = []string{"Downloading WordPress…", "Extracting files…", "Writing wp-config.php…"}
	case api.KindBlank:
		steps = []string{"Creating folder…", "Writing index.php…"}
	default:
		return fail(fmt.Errorf("unknown project kind %q", req.Kind))
	}
	if req.CreateDB {
		steps = append(steps, "Creating database…")
	}
	steps = append(steps, "Configuring Apache virtual host…")
	simulate(task, steps, 3*time.Second, progress)

	dir := req.Directory
	if dir == "" {
		dir = sitesDir
	}
	b.mu.Lock()
	s := &siteEntry{name: req.Name, path: strings.TrimRight(dir, `\/`) + `\` + req.Name, framework: fw, php: req.PHP}
	b.sites = append(b.sites, s)
	if req.CreateDB {
		b.dbs = append(b.dbs, api.Database{Name: strings.ReplaceAll(req.Name, "-", "_"), Tables: 0})
	}
	site := b.siteLocked(s)
	b.mu.Unlock()
	if progress != nil {
		progress(api.Progress{Task: task, Message: "Done", Percent: 100, Done: true})
	}
	return site, nil
}

func (b *Backend) ListPHP() ([]api.PHPVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	counts := map[string]int{}
	for _, s := range b.sites {
		v := s.php
		if v == "" {
			v = b.def
		}
		counts[v]++
	}
	var vers []string
	for v := range b.php {
		vers = append(vers, v)
	}
	sortVersions(vers)
	out := make([]api.PHPVersion, 0, len(vers))
	for _, v := range vers {
		p := b.php[v]
		out = append(out, api.PHPVersion{
			Version: v, Full: p.full, Installed: p.installed, Default: v == b.def,
			EOL: p.eol, SiteCount: counts[v], DownloadSize: p.size,
		})
	}
	return out, nil
}

func (b *Backend) InstallPHP(version string, progress api.ProgressFunc) error {
	task := "php:install:" + version
	b.mu.Lock()
	p, ok := b.php[version]
	b.mu.Unlock()
	if !ok {
		err := fmt.Errorf("PHP %s is not available", version)
		if progress != nil {
			progress(api.Progress{Task: task, Done: true, Error: err.Error()})
		}
		return err
	}
	mb := p.size >> 20
	if progress == nil {
		progress = func(api.Progress) {}
	}
	for i := 0; i <= 20; i++ {
		pct := float64(i) * 5
		msg := fmt.Sprintf("Downloading… %d of %d MB", int64(pct)*mb/100, mb)
		if i > 15 {
			msg = "Extracting…"
		}
		if i == 20 {
			msg = "Writing php.ini…"
		}
		progress(api.Progress{Task: task, Message: msg, Percent: pct})
		time.Sleep(150 * time.Millisecond)
	}
	b.mu.Lock()
	p.installed = true
	p.settings = defaultIni(version)
	b.mu.Unlock()
	progress(api.Progress{Task: task, Message: "Installed", Percent: 100, Done: true})
	return nil
}

func (b *Backend) RemovePHP(version string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.php[version]
	if !ok || !p.installed {
		return fmt.Errorf("PHP %s is not installed", version)
	}
	if version == b.def {
		return fmt.Errorf("PHP %s is the default version; make another version default first", version)
	}
	for _, s := range b.sites {
		if s.php == version {
			return fmt.Errorf("PHP %s is used by %s.%s", version, s.name, b.settings.TLD)
		}
	}
	p.installed = false
	return nil
}

func (b *Backend) SetDefaultPHP(version string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.php[version]
	if !ok || !p.installed {
		return fmt.Errorf("PHP %s is not installed", version)
	}
	b.def = version
	return nil
}

func (b *Backend) GetPHPSettings(version string) (api.PHPSettings, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.php[version]
	if !ok || !p.installed {
		return api.PHPSettings{}, fmt.Errorf("PHP %s is not installed", version)
	}
	s := p.settings
	ini := make(map[string]string, len(s.Ini))
	for k, v := range s.Ini {
		ini[k] = v
	}
	return api.PHPSettings{Version: version, Ini: ini, Extensions: append([]api.Extension(nil), s.Extensions...)}, nil
}

func (b *Backend) SavePHPSettings(s api.PHPSettings) error {
	time.Sleep(300 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.php[s.Version]
	if !ok || !p.installed {
		return fmt.Errorf("PHP %s is not installed", s.Version)
	}
	p.settings = s
	return nil
}

func (b *Backend) PHPIniPath(version string) string {
	return home + `\php\` + version + `\php.ini`
}

func (b *Backend) MySQLInfo() (api.MySQLInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.services[api.ServiceMySQL]
	return api.MySQLInfo{
		Running: s.running, Version: s.version, Host: "127.0.0.1", Port: b.settings.MySQLPort,
		User: "root", Password: b.password, DataDir: home + `\data\mysql`,
	}, nil
}

func (b *Backend) mysqlUpLocked() error {
	if !b.services[api.ServiceMySQL].running {
		return fmt.Errorf("MySQL is not running")
	}
	return nil
}

func (b *Backend) ListDatabases() ([]api.Database, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.mysqlUpLocked(); err != nil {
		return nil, err
	}
	out := append([]api.Database(nil), b.dbs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

var dbName = regexp.MustCompile(`^[A-Za-z0-9_$-]{1,64}$`)

func (b *Backend) CreateDatabase(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.mysqlUpLocked(); err != nil {
		return err
	}
	if !dbName.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	for _, d := range b.dbs {
		if d.Name == name {
			return fmt.Errorf("database %q already exists", name)
		}
	}
	b.dbs = append(b.dbs, api.Database{Name: name})
	return nil
}

func (b *Backend) DropDatabase(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.mysqlUpLocked(); err != nil {
		return err
	}
	for i, d := range b.dbs {
		if d.Name == name {
			b.dbs = append(b.dbs[:i], b.dbs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("database %q not found", name)
}

func (b *Backend) ImportSQL(database, file string) error {
	time.Sleep(1200 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.mysqlUpLocked(); err != nil {
		return err
	}
	for i := range b.dbs {
		if b.dbs[i].Name == database {
			b.dbs[i].Tables += 5
			b.dbs[i].SizeBytes += 1_500_000
			return nil
		}
	}
	return fmt.Errorf("database %q not found", database)
}

func (b *Backend) ExportDatabase(database, file string) error {
	time.Sleep(800 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mysqlUpLocked()
}

func (b *Backend) SetMySQLPassword(password string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.mysqlUpLocked(); err != nil {
		return err
	}
	b.password = password
	return nil
}

func (b *Backend) LogNames() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	names := []string{"apache-error", "apache-access", "mysql"}
	for _, v := range b.installedLocked() {
		names = append(names, "php-"+v)
	}
	return names
}

func (b *Backend) ReadLog(name string, lines int) (string, error) {
	now := time.Now()
	var sb strings.Builder
	n := min(lines, 60)
	for i := n; i > 0; i-- {
		t := now.Add(-time.Duration(i) * 17 * time.Second)
		switch {
		case name == "apache-access":
			fmt.Fprintf(&sb, "127.0.0.1 - - [%s] \"GET /%s HTTP/1.1\" 200 %d\n", t.Format("02/Jan/2006:15:04:05 -0700"), []string{"", "login", "api/users", "wp-admin/", "favicon.ico"}[i%5], 512+i*37)
		case name == "apache-error":
			fmt.Fprintf(&sb, "[%s] [mpm_winnt:notice] [pid 11824:tid 412] AH00354: Child: Starting %d worker threads.\n", t.Format("Mon Jan 02 15:04:05.000000 2006"), 64)
		case name == "mysql":
			fmt.Fprintf(&sb, "%s 0 [System] [MY-010931] [Server] mysqld: ready for connections. port: 3306\n", t.UTC().Format(time.RFC3339))
		case strings.HasPrefix(name, "php-"):
			fmt.Fprintf(&sb, "[%s UTC] PHP Warning:  Undefined variable $user in %s\\blog\\app\\Http\\Controllers\\HomeController.php on line %d\n", t.UTC().Format("02-Jan-2006 15:04:05"), sitesDir, 20+i)
		default:
			return "", fmt.Errorf("unknown log %q", name)
		}
	}
	return sb.String(), nil
}

func (b *Backend) GetSettings() (api.Settings, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.settings
	s.Parked = append([]string(nil), b.parked...)
	return s, nil
}

func (b *Backend) SaveSettings(s api.Settings) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range []int{s.HTTPPort, s.HTTPSPort, s.MySQLPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("invalid port %d", p)
		}
	}
	s.Home = home
	s.TLD = b.settings.TLD
	if s.Parked != nil {
		b.parked = append([]string(nil), s.Parked...)
	}
	b.settings = s
	b.services[api.ServiceApache].ports = []int{s.HTTPPort, s.HTTPSPort}
	b.services[api.ServiceMySQL].ports = []int{s.MySQLPort}
	return nil
}

func (b *Backend) TrustCA() error {
	time.Sleep(500 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.trusted = true
	return nil
}
