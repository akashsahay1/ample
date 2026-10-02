package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"apnoro/internal/api"
	"apnoro/internal/certs"
	"apnoro/internal/config"
	"apnoro/internal/external"
	"apnoro/internal/importer"
	"apnoro/internal/mysql"
	"apnoro/internal/php"
	"apnoro/internal/services"
)

var _ api.Coexistence = (*Core)(nil)

var envTitles = map[string]string{
	api.EnvXAMPP:   "XAMPP",
	api.EnvHerd:    "Laravel Herd",
	api.EnvLaragon: "Laragon",
	api.EnvWAMP:    "WAMP",
}

// describeOwner renders a port owner for error messages, e.g.
// "XAMPP (C:\xampp\apache\bin\httpd.exe)".
func describeOwner(c api.PortConflict) string {
	who := c.Process
	if t, ok := envTitles[c.Env]; ok {
		who = t + " " + strings.TrimSuffix(c.Process, ".exe")
	}
	if c.Path != "" {
		who += " (" + c.Path + ")"
	}
	return strings.TrimSpace(who)
}

func (c *Core) DetectEnvironments() ([]api.ExternalEnv, error) {
	return external.Detect()
}

func (c *Core) PortConflicts() ([]api.PortConflict, error) {
	cfg, err := c.cfg()
	if err != nil {
		return nil, err
	}
	const https = "apache-https"
	ports := map[string]int{api.ServiceApache: cfg.Ports.HTTP, api.ServiceMySQL: cfg.Ports.MySQL}
	if cfg.Ports.HTTPS != cfg.Ports.HTTP {
		ports[https] = cfg.Ports.HTTPS
	}
	out := external.Conflicts(ports)
	for i := range out {
		if out[i].Service == https {
			out[i].Service = api.ServiceApache
		}
	}
	return out, nil
}

func (c *Core) StopEnvironment(kind string) error {
	if _, ok := envTitles[kind]; !ok {
		return fmt.Errorf("cannot stop %q", kind)
	}
	return external.Stop(kind)
}

func (c *Core) importDeps(cfg *config.Config) importer.Deps {
	return importer.Deps{
		MySQLPort:     cfg.Ports.MySQL,
		MySQLPassword: cfg.MySQL.RootPassword,
		ExistingSites: func() []api.Site {
			ss, _ := c.discover(cfg)
			return ss
		},
		InstalledPHP: func() []string {
			inst, _ := php.List()
			out := make([]string, 0, len(inst))
			for _, i := range inst {
				out = append(out, i.Minor)
			}
			return out
		},
		ExistingDatabases: func() ([]api.Database, error) {
			if !services.Status(api.ServiceMySQL).Running {
				return nil, errors.New("MySQL is not running")
			}
			return mysql.ListDatabases(cfg.Ports.MySQL, cfg.MySQL.RootPassword)
		},
	}
}

func (c *Core) ScanImport(kind string, src *api.MySQLSource) (api.ImportPlan, error) {
	cfg, err := c.cfg()
	if err != nil {
		return api.ImportPlan{}, err
	}
	return importer.Scan(kind, src, c.importDeps(cfg))
}

func (c *Core) RunImport(req api.ImportRequest, progress api.ProgressFunc) error {
	task := api.ImportTaskPrefix + req.Kind
	report := func(p api.Progress) {
		p.Task = task
		if progress != nil {
			progress(p)
		}
	}
	notes, err := c.runImport(req, report)
	msg := "Import finished"
	if len(notes) > 0 {
		msg += ": " + strings.Join(notes, "; ")
	}
	if err != nil {
		report(api.Progress{Done: true, Percent: 100, Message: msg, Error: err.Error()})
		return err
	}
	report(api.Progress{Done: true, Percent: 100, Message: msg})
	return nil
}

func (c *Core) runImport(req api.ImportRequest, report func(api.Progress)) ([]string, error) {
	cfg, err := c.cfg()
	if err != nil {
		return nil, err
	}
	if len(req.Databases) > 0 {
		report(api.Progress{Message: "Starting Apnoro MySQL", Percent: -1})
		if err := c.StartService(api.ServiceMySQL); err != nil {
			return nil, fmt.Errorf("Apnoro MySQL must be running to import databases: %w", err)
		}
	}

	res, runErr := importer.Run(context.Background(), req, c.importDeps(cfg), func(msg string, pct float64) {
		report(api.Progress{Message: msg, Percent: pct * 0.8}) // leave room for PHP installs and config
	})
	notes := append([]string{}, res.Notes...)

	// Apply whatever the importer produced even if part of it (e.g. one database) failed.
	for _, v := range res.InstallPHP {
		if phpInstalled(v) {
			continue
		}
		report(api.Progress{Message: "Installing PHP " + v, Percent: 85})
		if err := c.InstallPHP(v, nil); err != nil {
			notes = append(notes, fmt.Sprintf("PHP %s could not be installed (%v); its sites use the default version", v, err))
			for site, pin := range res.PHP {
				if pin == v {
					delete(res.PHP, site)
				}
			}
		}
	}

	report(api.Progress{Message: "Configuring sites", Percent: 92})
	err = c.mutate(func(cfg *config.Config) error {
		for _, d := range res.Park {
			dup := false
			for _, p := range cfg.Parked {
				if samePath(p, d) {
					dup = true
				}
			}
			if !dup {
				cfg.Parked = append(cfg.Parked, cleanPath(d))
			}
		}
		for _, l := range res.Links {
			dup := false
			for _, e := range cfg.Links {
				if e.Name == l.Name {
					dup = true
				}
			}
			if dup {
				notes = append(notes, l.Name+" was already linked; kept the existing link")
				continue
			}
			cfg.Links = append(cfg.Links, l)
		}
		set := func(site string, fn func(*config.SiteSettings)) {
			s := cfg.Sites[site]
			fn(&s)
			cfg.Sites[site] = s
		}
		for site, v := range res.PHP {
			if phpInstalled(v) {
				set(site, func(s *config.SiteSettings) { s.PHP = v })
			}
		}
		for site, dr := range res.DocRoots {
			set(site, func(s *config.SiteSettings) { s.DocRoot = dr })
		}
		for _, site := range res.Secure {
			set(site, func(s *config.SiteSettings) { s.Secure = true })
		}
		return nil
	})
	if err != nil {
		return notes, errors.Join(runErr, err)
	}
	if len(res.Secure) > 0 && !certs.IsCATrusted() {
		notes = append(notes, "run `apnoro trust` (or Settings › Trust HTTPS certificate) so browsers accept the secured sites")
	}
	return notes, runErr
}
