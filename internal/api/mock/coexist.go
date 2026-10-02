package mock

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"apnoro/internal/api"
)

// Mock coexistence: XAMPP 8.2.12 running on :80/:3306, Laravel Herd stopped,
// Laragon/WAMP absent.

var _ api.Coexistence = (*Backend)(nil)

const (
	xamppRoot = `C:\xampp`
	herdRoot  = `C:\Users\dev\.config\herd`
	herdSites = `C:\Users\dev\Herd`
)

type mockSrcSite struct {
	name, domain, path, docRoot, php, source string
	secure                                   bool
}

var herdSrc = []mockSrcSite{
	{name: "crm", domain: "crm.test", path: herdSites + `\crm`, docRoot: herdSites + `\crm\public`, php: "8.3", secure: true, source: "parked"},
	{name: "invoicing", domain: "invoicing.test", path: herdSites + `\invoicing`, docRoot: herdSites + `\invoicing\public`, php: "8.2", source: "parked"},
	{name: "docs", domain: "docs.test", path: `D:\work\docs-site`, docRoot: `D:\work\docs-site`, secure: true, source: "link"},
	{name: "blog", domain: "blog.test", path: herdSites + `\blog`, docRoot: herdSites + `\blog\public`, source: "parked"},
}

var xamppSrc = []mockSrcSite{
	{name: "wordpress", domain: "localhost/wordpress", path: xamppRoot + `\htdocs\wordpress`, docRoot: xamppRoot + `\htdocs\wordpress`, source: "htdocs"},
	{name: "timesheet", domain: "localhost/timesheet", path: xamppRoot + `\htdocs\timesheet`, docRoot: xamppRoot + `\htdocs\timesheet`, source: "htdocs"},
	{name: "client-portal", domain: "client-portal.local", path: `D:\projects\client-portal`, docRoot: `D:\projects\client-portal\public`, secure: true, source: "vhost"},
}

var xamppDBs = []api.ImportDatabase{
	{Name: "wordpress", SizeBytes: 12_400_000},
	{Name: "timesheet", SizeBytes: 3_100_000},
	{Name: "client_portal", SizeBytes: 48_700_000},
	{Name: "blog", SizeBytes: 1_900_000},
}

var mysqlDBs = []api.ImportDatabase{
	{Name: "analytics", SizeBytes: 220_000_000},
	{Name: "legacy_crm", SizeBytes: 98_000_000},
	{Name: "staging_shop", SizeBytes: 31_500_000},
}

func (b *Backend) DetectEnvironments() ([]api.ExternalEnv, error) {
	time.Sleep(250 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	x := api.ExternalEnv{
		Kind: api.EnvXAMPP, Name: "XAMPP 8.2.12", Path: xamppRoot, PHP: "8.2.12",
		CanImport: true, Sites: len(xamppSrc), Databases: true,
		Notes: []string{"Ships MariaDB 10.4.32; databases are converted for MySQL 8.4 during import."},
	}
	if !b.xamppStopped {
		x.Running, x.CanStop, x.Ports = true, true, []int{80, 443, 3306}
	}
	h := api.ExternalEnv{
		Kind: api.EnvHerd, Name: "Laravel Herd", Path: herdRoot, PHP: "8.3.26", OnPath: true,
		CanImport: true, Sites: len(herdSrc),
		Notes: []string{"Bundled PHP: 8.3.26, 8.2.29"},
	}
	return []api.ExternalEnv{x, h}, nil
}

func (b *Backend) PortConflicts() ([]api.PortConflict, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conflictsLocked(), nil
}

func (b *Backend) conflictsLocked() []api.PortConflict {
	if b.xamppStopped {
		return nil
	}
	var out []api.PortConflict
	if b.settings.HTTPPort == 80 {
		out = append(out, api.PortConflict{Port: 80, Service: api.ServiceApache, Process: "httpd.exe", Path: xamppRoot + `\apache\bin\httpd.exe`, Env: api.EnvXAMPP})
	}
	if b.settings.HTTPSPort == 443 {
		out = append(out, api.PortConflict{Port: 443, Service: api.ServiceApache, Process: "httpd.exe", Path: xamppRoot + `\apache\bin\httpd.exe`, Env: api.EnvXAMPP})
	}
	if b.settings.MySQLPort == 3306 {
		out = append(out, api.PortConflict{Port: 3306, Service: api.ServiceMySQL, Process: "mysqld.exe", Path: xamppRoot + `\mysql\bin\mysqld.exe`, Env: api.EnvXAMPP})
	}
	return out
}

// portBlockedLocked reports why service cannot start, or nil.
func (b *Backend) portBlockedLocked(service string) error {
	for _, c := range b.conflictsLocked() {
		if c.Service == service {
			return fmt.Errorf("port %d is in use by %s (%s)", c.Port, c.Process, c.Path)
		}
	}
	return nil
}

func (b *Backend) StopEnvironment(kind string) error {
	time.Sleep(900 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	switch kind {
	case api.EnvXAMPP:
		if b.xamppStopped {
			return fmt.Errorf("XAMPP is not running")
		}
		b.xamppStopped = true
		return nil
	case api.EnvHerd:
		return fmt.Errorf("Laravel Herd is not running")
	}
	return fmt.Errorf("%s was not found on this computer", kind)
}

func (b *Backend) ScanImport(kind string, src *api.MySQLSource) (api.ImportPlan, error) {
	time.Sleep(600 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	var plan api.ImportPlan
	var list []mockSrcSite
	switch kind {
	case api.EnvHerd:
		plan = api.ImportPlan{Kind: kind, Source: herdRoot, ParkedDirs: []string{herdSites},
			Notes: []string{"Herd (free) has no databases. Herd Pro MySQL can be imported with “Other MySQL server”."}}
		list = herdSrc
	case api.EnvXAMPP:
		plan = api.ImportPlan{Kind: kind, Source: xamppRoot, ParkedDirs: []string{xamppRoot + `\htdocs`},
			Notes: []string{"Ships MariaDB 10.4.32; databases are converted for MySQL 8.4 during import.", "System databases (mysql, phpmyadmin, performance_schema) are never imported."}}
		list = xamppSrc
		plan.Databases = b.markExistsLocked(xamppDBs)
	case api.EnvMySQL:
		if src == nil || src.Host == "" {
			return plan, fmt.Errorf("MySQL host is required")
		}
		if src.User != "root" && src.User != "admin" {
			return plan, fmt.Errorf("access denied for user '%s'@'%s' (using password: %s)", src.User, src.Host, map[bool]string{true: "YES", false: "NO"}[src.Password != ""])
		}
		plan = api.ImportPlan{Kind: kind, Source: fmt.Sprintf("%s:%d", src.Host, src.Port)}
		plan.Databases = b.markExistsLocked(mysqlDBs)
	default:
		return plan, fmt.Errorf("%s was not found on this computer", kind)
	}
	missing := map[string]bool{}
	for _, s := range list {
		is := api.ImportSite{Name: s.name, Domain: s.domain, Path: s.path, DocRoot: s.docRoot, PHP: s.php, Secure: s.secure, Source: s.source}
		if e := b.findLocked(s.name); e != nil {
			is.Conflict = "name " + s.name + " is taken by " + e.path
		}
		if s.php != "" && is.Conflict == "" {
			if p, ok := b.php[s.php]; !ok || !p.installed {
				missing[s.php] = true
			}
		}
		plan.Sites = append(plan.Sites, is)
	}
	for v := range missing {
		plan.MissingPHP = append(plan.MissingPHP, v)
	}
	sortVersions(plan.MissingPHP)
	return plan, nil
}

func (b *Backend) markExistsLocked(in []api.ImportDatabase) []api.ImportDatabase {
	out := make([]api.ImportDatabase, len(in))
	for i, d := range in {
		d.Exists = false
		for _, e := range b.dbs {
			if e.Name == d.Name {
				d.Exists = true
			}
		}
		out[i] = d
	}
	return out
}

func (b *Backend) RunImport(req api.ImportRequest, progress api.ProgressFunc) error {
	task := api.ImportTaskPrefix + req.Kind
	if progress == nil {
		progress = func(api.Progress) {}
	}
	fail := func(err error) error {
		progress(api.Progress{Task: task, Done: true, Percent: 100, Error: err.Error()})
		return err
	}
	plan, err := b.ScanImport(req.Kind, req.MySQL)
	if err != nil {
		return fail(err)
	}
	if len(req.ParkDirs)+len(req.Sites)+len(req.Databases) == 0 {
		return fail(fmt.Errorf("nothing selected to import"))
	}
	var steps []string
	if len(req.ParkDirs) > 0 {
		steps = append(steps, "Parking folders…")
	}
	if len(req.Sites) > 0 {
		steps = append(steps, "Linking sites…")
	}
	if req.InstallPHP && len(plan.MissingPHP) > 0 {
		steps = append(steps, "Installing PHP "+strings.Join(plan.MissingPHP, ", ")+"…")
	}
	for _, d := range req.Databases {
		steps = append(steps, "Copying database "+d+"…")
	}
	steps = append(steps, "Writing Apache configuration…")
	simulate(task, steps, 3*time.Second, progress)

	b.mu.Lock()
	norm := func(p string) string { return strings.ToLower(filepath.Clean(p)) }
	for _, d := range req.ParkDirs {
		found := false
		for _, p := range b.parked {
			found = found || strings.EqualFold(p, d)
		}
		if !found {
			b.parked = append(b.parked, d)
		}
	}
	want := map[string]bool{}
	for _, p := range req.Sites {
		want[norm(p)] = true
	}
	for _, s := range plan.Sites {
		viaPark := false
		for _, d := range req.ParkDirs {
			viaPark = viaPark || norm(filepath.Dir(s.Path)) == norm(d)
		}
		if (!viaPark && !want[norm(s.Path)]) || s.Conflict != "" {
			continue
		}
		pin := ""
		if s.PHP != "" {
			if p, ok := b.php[s.PHP]; ok && (p.installed || req.InstallPHP) {
				if !p.installed {
					p.installed = true
					p.settings = defaultIni(s.PHP)
				}
				pin = s.PHP
			}
		}
		fw := "php"
		if strings.HasSuffix(s.DocRoot, `\public`) {
			fw = "laravel"
		}
		b.sites = append(b.sites, &siteEntry{name: s.Name, path: s.Path, framework: fw, php: pin, secure: req.KeepSecure && s.Secure, linked: !viaPark})
	}
	for _, name := range req.Databases {
		var src *api.ImportDatabase
		for i := range plan.Databases {
			if plan.Databases[i].Name == name {
				src = &plan.Databases[i]
			}
		}
		if src == nil {
			continue
		}
		idx := -1
		for i, d := range b.dbs {
			if d.Name == name {
				idx = i
			}
		}
		nd := api.Database{Name: name, Tables: 12, SizeBytes: src.SizeBytes}
		switch {
		case idx < 0:
			b.dbs = append(b.dbs, nd)
		case req.Overwrite:
			b.dbs[idx] = nd
		}
	}
	b.mu.Unlock()
	progress(api.Progress{Task: task, Message: "Import complete", Percent: 100, Done: true})
	return nil
}
