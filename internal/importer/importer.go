// Package importer brings sites and databases over from XAMPP, Laravel Herd,
// Laragon, WampServer or any MySQL/MariaDB server.
//
// Scan builds a preview (api.ImportPlan) without changing anything. Run copies
// the selected databases into Apnoro MySQL itself and returns the configuration
// changes (parks, links, PHP pins, secure flags, PHP versions to install) as a
// Result for the core to apply; it never writes config.json. Project files are
// never copied: sites are served in place.
package importer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"apnoro/internal/api"
	"apnoro/internal/config"
	"apnoro/internal/external"
	"apnoro/internal/sites"
)

// Deps are the Apnoro-side facts the importer needs, supplied by the core.
type Deps struct {
	MySQLPort         int
	MySQLPassword     string
	ExistingSites     func() []api.Site
	InstalledPHP      func() []string // installed minors
	ExistingDatabases func() ([]api.Database, error)
}

// Result is what the core must apply after Run.
type Result struct {
	Park       []string          // directories to park
	Links      []config.Link     // folders to link
	DocRoots   map[string]string // site -> DocRoot override relative to the site path (config.SiteSettings.DocRoot)
	PHP        map[string]string // site -> PHP minor to pin
	Secure     []string          // sites to secure
	InstallPHP []string          // PHP minors to install before pinning
	Databases  []string          // databases copied into Apnoro
	Notes      []string          // skipped items and warnings, for the user
}

// Scan previews an import. src is optional except for api.EnvMySQL; for the
// stacks it supplies credentials (and optionally a port) for their MySQL.
func Scan(kind string, src *api.MySQLSource, d Deps) (api.ImportPlan, error) {
	plan := api.ImportPlan{Kind: kind}
	var root string
	if kind != api.EnvMySQL {
		r, ok := external.Root(kind)
		if !ok {
			return plan, fmt.Errorf("import: %s is not installed", kind)
		}
		root = r
		plan.Source = r
		list, parked, notes, err := external.SourceSites(kind, root)
		if err != nil {
			return plan, fmt.Errorf("import: %w", err)
		}
		plan.Sites, plan.ParkedDirs, plan.Notes = list, parked, notes
	} else {
		if src == nil {
			return plan, errors.New("import: MySQL connection details are required")
		}
		plan.Source = fmt.Sprintf("%s:%d", hostOr(src.Host), portOr(src.Port))
	}
	annotate(&plan, existingSites(d), installed(d))

	// Databases.
	dbs, note := scanDatabases(kind, root, src, d)
	if note != "" {
		plan.Notes = append(plan.Notes, note)
	}
	have := map[string]bool{}
	if d.ExistingDatabases != nil {
		if list, err := d.ExistingDatabases(); err == nil {
			for _, x := range list {
				have[strings.ToLower(x.Name)] = true
			}
		}
	}
	for _, x := range dbs {
		plan.Databases = append(plan.Databases, api.ImportDatabase{Name: x.Name, SizeBytes: x.SizeBytes, Exists: have[strings.ToLower(x.Name)]})
	}
	sort.Slice(plan.Databases, func(i, j int) bool { return plan.Databases[i].Name < plan.Databases[j].Name })
	return plan, nil
}

func hostOr(h string) string {
	if h == "" {
		return "127.0.0.1"
	}
	return h
}

func portOr(p int) int {
	if p == 0 {
		return 3306
	}
	return p
}

// scanDatabases lists source databases without starting anything: a running
// server is queried; a stopped XAMPP is read from its data directory.
func scanDatabases(kind, root string, src *api.MySQLSource, d Deps) ([]api.Database, string) {
	ctx := context.Background()
	s, err := resolveSource(ctx, kind, src, d, false)
	if err == nil {
		defer s.close()
		dbs, err := s.databases(ctx)
		if err != nil {
			return nil, "Could not list databases: " + err.Error()
		}
		return dbs, ""
	}
	if errors.Is(err, errNoSource) {
		switch kind {
		case api.EnvXAMPP:
			dbs := external.DataDirDatabases(filepath.Join(root, "mysql", "data"))
			if len(dbs) > 0 {
				return dbs, "XAMPP's MySQL is stopped: Apnoro will start it temporarily on a private port to copy the databases (sizes are on-disk estimates)."
			}
			return nil, ""
		case api.EnvHerd:
			if h, err := external.ReadHerd(root); err == nil {
				for _, svc := range h.Services {
					if svc == "mysql" || svc == "mariadb" {
						return nil, "To import Herd databases, start Herd's MySQL service (default root, no password, port 3306) or enter its connection details."
					}
				}
			}
			return nil, ""
		default:
			return nil, "To import databases, start " + kind + "'s MySQL or enter a server's connection details."
		}
	}
	return nil, "Databases: " + err.Error()
}

func existingSites(d Deps) []api.Site {
	if d.ExistingSites == nil {
		return nil
	}
	return d.ExistingSites()
}

func installed(d Deps) []string {
	if d.InstalledPHP == nil {
		return nil
	}
	return d.InstalledPHP()
}

// annotate sets ImportSite.Conflict and plan.MissingPHP (pure; used by tests).
func annotate(plan *api.ImportPlan, existing []api.Site, installedPHP []string) {
	byName := map[string]api.Site{}
	byPath := map[string]api.Site{}
	for _, s := range existing {
		byName[s.Name] = s
		byPath[norm(s.Path)] = s
	}
	seen := map[string]bool{}
	missing := map[string]bool{}
	have := map[string]bool{}
	for _, v := range installedPHP {
		have[v] = true
	}
	for i := range plan.Sites {
		s := &plan.Sites[i]
		switch {
		case s.Conflict != "":
		case s.Name == "" || !sites.ValidName(s.Name):
			s.Conflict = fmt.Sprintf("%q is not a valid site name", s.Name)
		case byPath[norm(s.Path)].Name != "":
			s.Conflict = "already served by Apnoro as " + byPath[norm(s.Path)].Domain
		case byName[s.Name].Name != "":
			s.Conflict = "name " + s.Name + " is taken by " + byName[s.Name].Path
		case seen[s.Name]:
			s.Conflict = "another imported site is also named " + s.Name
		}
		seen[s.Name] = true
		if s.PHP != "" && !have[s.PHP] {
			missing[s.PHP] = true
		}
	}
	plan.MissingPHP = nil
	for v := range missing {
		plan.MissingPHP = append(plan.MissingPHP, v)
	}
	sort.Strings(plan.MissingPHP)
}

func norm(p string) string { return strings.ToLower(filepath.Clean(p)) }

// Run performs an import: copies the requested databases into Apnoro MySQL
// (which must be running) and returns the config changes for the core.
// The config part of Result is filled even when err != nil (e.g. one database
// failed), so the core should apply it and then report the error.
// progress receives messages and 0..100 percentages (-1 = indeterminate).
func Run(ctx context.Context, req api.ImportRequest, d Deps, progress func(msg string, pct float64)) (Result, error) {
	if progress == nil {
		progress = func(string, float64) {}
	}
	res := Result{DocRoots: map[string]string{}, PHP: map[string]string{}}
	progress("Reading "+req.Kind+" configuration", 0)

	var plan api.ImportPlan
	if req.Kind != api.EnvMySQL {
		root, ok := external.Root(req.Kind)
		if !ok {
			return res, fmt.Errorf("import: %s is not installed", req.Kind)
		}
		list, parked, _, err := external.SourceSites(req.Kind, root)
		if err != nil {
			return res, fmt.Errorf("import: %w", err)
		}
		plan = api.ImportPlan{Kind: req.Kind, Source: root, Sites: list, ParkedDirs: parked}
		annotate(&plan, existingSites(d), installed(d))
	}
	planSites(&res, plan, req, installed(d))

	if len(req.Databases) > 0 {
		if err := runDatabases(ctx, req, d, &res, progress); err != nil {
			return res, err
		}
	}
	progress("Import finished", 100)
	return res, nil
}

// planSites fills the config part of Result (pure; used by tests).
func planSites(res *Result, plan api.ImportPlan, req api.ImportRequest, installedPHP []string) {
	if res.DocRoots == nil {
		res.DocRoots = map[string]string{}
	}
	if res.PHP == nil {
		res.PHP = map[string]string{}
	}
	have := map[string]bool{}
	for _, v := range installedPHP {
		have[v] = true
	}
	for _, dir := range req.ParkDirs {
		ok := false
		for _, p := range plan.ParkedDirs {
			if norm(p) == norm(dir) {
				res.Park = append(res.Park, p)
				ok = true
				break
			}
		}
		if !ok {
			res.Notes = append(res.Notes, "Not a folder of this import, not parked: "+dir)
		}
	}
	parked := func(path string) bool {
		for _, p := range res.Park {
			if norm(filepath.Dir(path)) == norm(p) {
				return true
			}
		}
		return false
	}
	wanted := map[string]bool{}
	for _, p := range req.Sites {
		wanted[norm(p)] = true
	}
	found := map[string]bool{}
	install := map[string]bool{}
	for _, s := range plan.Sites {
		viaPark := parked(s.Path)
		viaLink := wanted[norm(s.Path)]
		if !viaPark && !viaLink {
			continue
		}
		found[norm(s.Path)] = true
		name := s.Name
		if viaPark {
			// Parked folders are served under their folder name.
			name = sites.Slug(filepath.Base(s.Path))
		}
		if s.Conflict != "" {
			if viaPark {
				res.Notes = append(res.Notes, fmt.Sprintf("%s (in a parked folder): %s", s.Domain, s.Conflict))
			} else {
				res.Notes = append(res.Notes, fmt.Sprintf("Skipped %s: %s", s.Domain, s.Conflict))
			}
			continue
		}
		if viaLink && !viaPark {
			res.Links = append(res.Links, config.Link{Name: name, Path: s.Path})
		}
		if s.DocRoot != "" && norm(s.DocRoot) != norm(sites.DocRoot(s.Path)) {
			if rel, err := filepath.Rel(s.Path, s.DocRoot); err == nil && !strings.HasPrefix(rel, "..") {
				res.DocRoots[name] = rel
			}
		}
		if s.PHP != "" {
			switch {
			case have[s.PHP]:
				res.PHP[name] = s.PHP
			case req.InstallPHP:
				res.PHP[name] = s.PHP
				install[s.PHP] = true
			default:
				res.Notes = append(res.Notes, fmt.Sprintf("%s used PHP %s, which is not installed in Apnoro; it will use the default PHP.", name, s.PHP))
			}
		}
		if req.KeepSecure && s.Secure {
			res.Secure = append(res.Secure, name)
		}
	}
	for p := range wanted {
		if !found[p] {
			res.Notes = append(res.Notes, "Not a site of this import, skipped: "+p)
		}
	}
	for v := range install {
		res.InstallPHP = append(res.InstallPHP, v)
	}
	sort.Strings(res.InstallPHP)
	sort.Slice(res.Links, func(i, j int) bool { return res.Links[i].Name < res.Links[j].Name })
	sort.Strings(res.Secure)
}

func runDatabases(ctx context.Context, req api.ImportRequest, d Deps, res *Result, progress func(string, float64)) error {
	dst := &server{Host: "127.0.0.1", Port: d.MySQLPort, User: "root", Password: d.MySQLPassword, Desc: "Apnoro MySQL"}
	if _, _, err := dst.version(ctx); err != nil {
		return fmt.Errorf("import: Apnoro MySQL is not reachable on port %d (start it first): %w", d.MySQLPort, err)
	}
	progress("Connecting to "+req.Kind+" MySQL", -1)
	src, err := resolveSource(ctx, req.Kind, req.MySQL, d, true)
	if errors.Is(err, errNoSource) {
		return fmt.Errorf("import: %s's MySQL is not running; start it or enter its connection details", req.Kind)
	}
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	defer func() {
		progress("Stopping temporary server", -1)
		src.close()
	}()
	_, srcMaria, err := src.version(ctx)
	if err != nil {
		return fmt.Errorf("import: %s: %w", src.Desc, err)
	}
	srcDBs, err := src.databases(ctx)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	sizes := map[string]int64{}
	for _, x := range srcDBs {
		sizes[x.Name] = x.SizeBytes
	}

	n := len(req.Databases)
	var failed []string
	for i, name := range req.Databases {
		if err := ctx.Err(); err != nil {
			return err
		}
		base := 5 + 90*float64(i)/float64(n)
		span := 90 / float64(n)
		if _, ok := sizes[name]; !ok {
			res.Notes = append(res.Notes, "Database "+name+" not found on "+src.Desc+"; skipped.")
			continue
		}
		if external.SystemDatabases[strings.ToLower(name)] {
			res.Notes = append(res.Notes, "System database "+name+" is never imported.")
			continue
		}
		progress(fmt.Sprintf("Copying database %s (%d/%d)", name, i+1, n), base)
		cs, coll := src.charset(ctx, name)
		existed, err := prepareTarget(ctx, dst, name, cs, coll, req.Overwrite)
		if err != nil {
			failed = append(failed, name+": "+err.Error())
			continue
		}
		if existed && !req.Overwrite {
			res.Notes = append(res.Notes, "Database "+name+" already exists in Apnoro; skipped (choose overwrite to replace it).")
			continue
		}
		est := sizes[name]
		if est <= 0 {
			est = 1 << 20
		}
		err = copyDatabase(ctx, src, dst, name, srcMaria, func(b int64) {
			frac := float64(b) / float64(est)
			if frac > 0.95 {
				frac = 0.95
			}
			progress(fmt.Sprintf("Copying database %s (%d/%d): %s", name, i+1, n, humanBytes(b)), base+span*frac)
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failed = append(failed, name+": "+err.Error())
			continue
		}
		res.Databases = append(res.Databases, name)
	}
	if len(failed) > 0 {
		return fmt.Errorf("import: %d of %d databases failed: %s", len(failed), n, strings.Join(failed, "; "))
	}
	return nil
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}
