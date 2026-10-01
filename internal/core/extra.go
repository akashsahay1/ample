package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ampls/internal/apache"
	"ampls/internal/api"
	"ampls/internal/certs"
	"ampls/internal/config"
	"ampls/internal/mysql"
	"ampls/internal/paths"
	"ampls/internal/php"
	"ampls/internal/projects"
	"ampls/internal/services"
	"ampls/internal/sites"
)

func cleanPath(p string) string { return filepath.Clean(strings.TrimSpace(p)) }
func baseName(p string) string  { return filepath.Base(cleanPath(p)) }
func logPath(name string) string {
	return filepath.Join(paths.LogsDir(), name)
}

// ---------- MySQL ----------

func (c *Core) mysqlConn() (int, string, error) {
	cfg, err := c.cfg()
	if err != nil {
		return 0, "", err
	}
	if !services.Status(api.ServiceMySQL).Running {
		return 0, "", errors.New("MySQL is not running")
	}
	return cfg.Ports.MySQL, cfg.MySQL.RootPassword, nil
}

func (c *Core) MySQLInfo() (api.MySQLInfo, error) {
	cfg, err := c.cfg()
	if err != nil {
		return api.MySQLInfo{}, err
	}
	return api.MySQLInfo{
		Running:  services.Status(api.ServiceMySQL).Running,
		Version:  mysql.Version(),
		Host:     "127.0.0.1",
		Port:     cfg.Ports.MySQL,
		User:     "root",
		Password: cfg.MySQL.RootPassword,
		DataDir:  paths.MySQLDataDir(),
	}, nil
}

func (c *Core) ListDatabases() ([]api.Database, error) {
	port, pw, err := c.mysqlConn()
	if err != nil {
		return nil, err
	}
	return mysql.ListDatabases(port, pw)
}

func (c *Core) CreateDatabase(name string) error {
	port, pw, err := c.mysqlConn()
	if err != nil {
		return err
	}
	return mysql.CreateDatabase(port, pw, name)
}

func (c *Core) DropDatabase(name string) error {
	port, pw, err := c.mysqlConn()
	if err != nil {
		return err
	}
	return mysql.DropDatabase(port, pw, name)
}

func (c *Core) ImportSQL(database, file string) error {
	port, pw, err := c.mysqlConn()
	if err != nil {
		return err
	}
	return mysql.Import(port, pw, database, file)
}

func (c *Core) ExportDatabase(database, file string) error {
	port, pw, err := c.mysqlConn()
	if err != nil {
		return err
	}
	return mysql.Export(port, pw, database, file)
}

func (c *Core) SetMySQLPassword(password string) error {
	port, old, err := c.mysqlConn()
	if err != nil {
		return err
	}
	if err := mysql.SetRootPassword(port, old, password); err != nil {
		return err
	}
	_, err = config.Update(func(cfg *config.Config) error {
		cfg.MySQL.RootPassword = password
		return nil
	})
	return err
}

// ---------- projects ----------

func dbName(site string) string { return strings.ReplaceAll(site, "-", "_") }

func (c *Core) NewProject(req api.NewProjectRequest, progress api.ProgressFunc) (api.Site, error) {
	task := "project:" + req.Name
	report := func(p api.Progress) {
		p.Task = task
		if progress != nil {
			progress(p)
		}
	}
	site, err := c.newProject(req, report)
	if err != nil {
		report(api.Progress{Done: true, Percent: 100, Error: err.Error()})
		return api.Site{}, err
	}
	report(api.Progress{Done: true, Percent: 100, Message: site.Domain + " is ready"})
	return site, nil
}

func (c *Core) newProject(req api.NewProjectRequest, report func(api.Progress)) (api.Site, error) {
	cfg, err := c.cfg()
	if err != nil {
		return api.Site{}, err
	}
	if !sites.ValidName(req.Name) {
		return api.Site{}, fmt.Errorf("invalid project name %q: use lowercase letters, digits and dashes", req.Name)
	}
	if _, exists := sites.Find(cfg, req.Name); exists {
		return api.Site{}, fmt.Errorf("a site named %s.%s already exists", req.Name, cfg.TLD)
	}
	dir := req.Directory
	if dir == "" {
		if len(cfg.Parked) == 0 {
			return api.Site{}, errors.New("choose a directory for the project")
		}
		dir = cfg.Parked[0]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return api.Site{}, err
	}
	ver := req.PHP
	if ver == "" {
		ver = defaultPHP(cfg)
	}
	if ver == "" {
		return api.Site{}, errors.New("no PHP version is installed: install one from PHP Versions first")
	}
	if !phpInstalled(ver) {
		return api.Site{}, fmt.Errorf("PHP %s is not installed", ver)
	}

	pr := projects.Request{
		Kind: req.Kind, Name: req.Name, Dir: dir,
		PHP: php.CLIPath(ver), ComposerPhar: paths.ComposerPhar(),
		DBPort: cfg.Ports.MySQL, DBPassword: cfg.MySQL.RootPassword,
	}
	if req.CreateDB && req.Kind != api.KindBlank {
		pr.DB = strings.TrimSpace(req.Database)
		if pr.DB == "" {
			pr.DB = dbName(req.Name)
		}
		if !mysql.ValidName(pr.DB) {
			return api.Site{}, fmt.Errorf("invalid database name %q: use letters, digits and underscores (max 64)", pr.DB)
		}
		report(api.Progress{Message: "Starting MySQL", Percent: -1})
		if err := c.StartService(api.ServiceMySQL); err != nil {
			return api.Site{}, err
		}
		exists := false
		if dbs, err := mysql.ListDatabases(cfg.Ports.MySQL, cfg.MySQL.RootPassword); err == nil {
			for _, d := range dbs {
				if strings.EqualFold(d.Name, pr.DB) {
					exists = true
				}
			}
		}
		if exists {
			report(api.Progress{Message: "Using existing database " + pr.DB, Percent: -1})
		} else {
			report(api.Progress{Message: "Creating database " + pr.DB, Percent: -1})
			if err := mysql.CreateDatabase(cfg.Ports.MySQL, cfg.MySQL.RootPassword, pr.DB); err != nil {
				return api.Site{}, err
			}
		}
	}
	path, err := projects.Create(context.Background(), pr, func(msg string, pct float64) {
		report(api.Progress{Message: msg, Percent: pct})
	})
	if err != nil {
		return api.Site{}, err
	}

	report(api.Progress{Message: "Configuring " + req.Name + "." + cfg.TLD, Percent: 95})
	parked := false
	for _, p := range cfg.Parked {
		if samePath(p, dir) {
			parked = true
		}
	}
	err = c.mutate(func(cfg *config.Config) error {
		if !parked {
			cfg.Links = append(cfg.Links, config.Link{Name: req.Name, Path: path})
		}
		if req.PHP != "" && req.PHP != defaultPHP(cfg) {
			s := cfg.Sites[req.Name]
			s.PHP = req.PHP
			cfg.Sites[req.Name] = s
		}
		return nil
	})
	if err != nil {
		return api.Site{}, err
	}
	if !services.Status(api.ServiceApache).Running {
		_ = c.StartService(api.ServiceApache)
	}
	cfg, err = c.cfg()
	if err != nil {
		return api.Site{}, err
	}
	cfg.DefaultPHP = defaultPHP(cfg)
	site, ok := sites.Find(cfg, req.Name)
	if !ok {
		return api.Site{}, fmt.Errorf("project created at %s but the site was not found", path)
	}
	return site, nil
}

// ---------- logs ----------

func (c *Core) LogNames() []string {
	names := []string{"apache-error", "apache-access", "mysql"}
	inst, _ := php.List()
	for _, i := range inst {
		names = append(names, "php-"+i.Minor)
	}
	return names
}

func (c *Core) ReadLog(name string, lines int) (string, error) {
	var file string
	switch {
	case name == "apache-error", name == "apache-access", name == "mysql":
		file = name + ".log"
	case strings.HasPrefix(name, "php-"):
		minor := strings.TrimPrefix(name, "php-")
		if !isMinor(minor) {
			return "", fmt.Errorf("unknown log %q", name)
		}
		file = "php-" + minor + "-error.log"
	default:
		return "", fmt.Errorf("unknown log %q", name)
	}
	if lines <= 0 {
		lines = 500
	}
	return tail(filepath.Join(paths.LogsDir(), file), lines)
}

func isMinor(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// tail returns the last n lines of a file, reading at most the final 2 MB.
func tail(path string, n int) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	const max = 2 << 20
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	off := int64(0)
	if fi.Size() > max {
		off = fi.Size() - max
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if off > 0 && len(ls) > 0 {
		ls = ls[1:] // first line is probably partial
	}
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n"), nil
}

// ---------- setup (installer) ----------

type SetupOptions struct {
	ParkDefault    bool   // create and park %USERPROFILE%\AMPLS\Sites
	TrustCAMachine bool   // add the CA to the machine Root store (needs admin; the installer uses `ampls trust --machine` instead)
	MySQLPassword  string // root password for a fresh MySQL datadir ("" = none); ignored when already initialized
}

// Setup prepares a freshly installed (or upgraded) data directory. Idempotent.
func (c *Core) Setup(opts SetupOptions, log func(string)) error {
	if log == nil {
		log = func(string) {}
	}
	log("Preparing " + paths.Home())
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	// An upgrade may have replaced the bundled PHP binaries in place: drop the
	// cached full-version files so php.List re-reads them.
	if entries, err := os.ReadDir(paths.PHPRoot()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				_ = os.Remove(filepath.Join(paths.PHPRoot(), e.Name(), php.VersionFile))
			}
		}
	}
	inst, err := php.List()
	if err != nil {
		return err
	}
	for _, i := range inst {
		log("Configuring PHP " + i.Full)
		if err := php.EnsureIni(i.Minor); err != nil {
			return fmt.Errorf("php %s: %w", i.Minor, err)
		}
	}
	cfg, err := config.Update(func(cfg *config.Config) error {
		if cfg.DefaultPHP == "" || !phpInstalled(cfg.DefaultPHP) {
			cfg.DefaultPHP = defaultPHP(cfg)
		}
		if opts.ParkDefault {
			d := paths.DefaultSitesDir()
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
			found := false
			for _, p := range cfg.Parked {
				if samePath(p, d) {
					found = true
				}
			}
			if !found {
				cfg.Parked = append(cfg.Parked, d)
			}
		} else if len(cfg.Parked) == 1 && samePath(cfg.Parked[0], paths.DefaultSitesDir()) {
			if _, err := os.Stat(paths.ConfigFile()); err != nil { // fresh install without the task
				cfg.Parked = []string{}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(mysql.MysqldPath()); err == nil {
		if err := mysql.WriteConfig(cfg.Ports.MySQL); err != nil {
			return err
		}
		if !mysql.Initialized() {
			log("Initializing MySQL data directory")
			if err := mysql.InitializeWithPassword(opts.MySQLPassword); err != nil {
				return fmt.Errorf("initialize MySQL: %w", err)
			}
			if _, err := config.Update(func(c *config.Config) error {
				c.MySQL.RootPassword = opts.MySQLPassword
				return nil
			}); err != nil {
				return err
			}
			if opts.MySQLPassword != "" {
				log("MySQL root password set")
			}
		} else if opts.MySQLPassword != "" {
			log("MySQL already initialized: kept the existing root password")
		}
	}
	log("Creating local certificate authority")
	if err := certs.EnsureCA(); err != nil {
		return err
	}
	if opts.TrustCAMachine {
		log("Trusting the AMPLS certificate")
		if err := certs.TrustCA(true); err != nil {
			log("warning: could not trust certificate: " + err.Error())
		}
	}
	if _, err := os.Stat(apache.HttpdPath()); err == nil {
		// The installer runs setup as the original (unelevated) user after the
		// hosts helper service is running, so hosts entries go through it.
		log("Writing Apache configuration and hosts entries")
		if err := c.syncOpts(false, true); err != nil {
			return err
		}
	}
	log("Done")
	return nil
}
