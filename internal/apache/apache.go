// Package apache generates the Apache httpd configuration (mod_fcgid based
// per-site PHP) and wraps the httpd binary.
package apache

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ampls/internal/download"
	"ampls/internal/paths"
	"ampls/internal/services"
)

// VHost is one generated site.
type VHost struct {
	Domain            string   // blog.test
	Aliases           []string // *.blog.test
	DocRoot           string
	PHPCGI            string // absolute php-cgi.exe
	Secure            bool
	CertFile, KeyFile string
}

// Options are global server settings.
type Options struct {
	HTTPPort, HTTPSPort int
	DefaultPHPCGI       string // for localhost + phpMyAdmin
}

// MainConfPath is <Home>/conf/httpd.conf.
func MainConfPath() string { return filepath.Join(paths.ConfDir(), "httpd.conf") }

// HttpdPath is the httpd executable.
func HttpdPath() string { return filepath.Join(paths.ApacheDir(), "bin", paths.Exe("httpd")) }

// StartArgs runs httpd in the foreground with the AMPLS config.
func StartArgs() []string {
	return []string{"-f", slash(MainConfPath()), "-d", slash(paths.ApacheDir())}
}

// WriteConfig writes conf/httpd.conf and conf/sites/*.conf, removing stale site files.
func WriteConfig(opts Options, vhosts []VHost) error {
	opts = withDefaults(opts)
	if err := checkConfigInputs(opts, paths.Home(), vhosts); err != nil {
		return err
	}
	for _, d := range []string{paths.ConfDir(), paths.SitesConfDir(), paths.LogsDir(), paths.RunDir(), paths.TmpDir(), paths.WWWDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("apache: write config: %w", err)
		}
	}
	secure := false
	for _, v := range vhosts {
		if v.Secure && v.CertFile != "" && v.KeyFile != "" {
			secure = true
		}
	}
	main := RenderMain(opts, paths.Home(), secure)
	if err := writeFile(MainConfPath(), main); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, v := range vhosts {
		if v.Domain == "" || v.DocRoot == "" || v.PHPCGI == "" {
			return fmt.Errorf("apache: write config: incomplete vhost %q", v.Domain)
		}
		name := ConfFileName(v.Domain)
		keep[strings.ToLower(name)] = true
		if err := writeFile(filepath.Join(paths.SitesConfDir(), name), RenderVHost(v, opts)); err != nil {
			return err
		}
	}
	entries, _ := os.ReadDir(paths.SitesConfDir())
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".conf") && !keep[strings.ToLower(e.Name())] {
			os.Remove(filepath.Join(paths.SitesConfDir(), e.Name()))
		}
	}
	return ensureWelcome()
}

func withDefaults(o Options) Options {
	if o.HTTPPort == 0 {
		o.HTTPPort = 80
	}
	if o.HTTPSPort == 0 {
		o.HTTPSPort = 443
	}
	return o
}

func writeFile(p, content string) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("apache: write config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("apache: write config: %w", err)
	}
	return nil
}

var unsafeName = regexp.MustCompile(`[^a-z0-9.\-]+`)

// ConfFileName maps a domain to a safe file name ("*.blog.test" -> "_.blog.test.conf").
func ConfFileName(domain string) string {
	n := unsafeName.ReplaceAllString(strings.ToLower(strings.TrimSpace(domain)), "_")
	n = strings.Trim(n, ".")
	if n == "" {
		n = "site"
	}
	return n + ".conf"
}

// ConfigTest runs httpd -t against the generated config.
func ConfigTest() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, HttpdPath(), append(StartArgs(), "-t")...)
	cmd.Dir = filepath.Dir(HttpdPath())
	services.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apache: config test failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

var versionRe = regexp.MustCompile(`Apache/([0-9]+\.[0-9]+\.[0-9]+)`)

// Version returns the installed httpd version ("2.4.68") or "".
func Version() string {
	if _, err := os.Stat(HttpdPath()); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, HttpdPath(), "-v")
	cmd.Dir = filepath.Dir(HttpdPath())
	services.Hide(cmd)
	out, _ := cmd.CombinedOutput()
	if m := versionRe.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}

// InstallFromZips installs Apache Lounge httpd (zip with an Apache24/ dir) into
// paths.ApacheDir() and copies mod_fcgid.so from the mod_fcgid zip into modules/.
func InstallFromZips(httpdZip, fcgidZip string) error {
	dir := paths.ApacheDir()
	stage := dir + ".staging"
	os.RemoveAll(stage)
	if err := download.UnzipSubdir(httpdZip, "Apache24", stage); err != nil {
		return fmt.Errorf("apache: install: %w", err)
	}
	if fcgidZip != "" {
		if err := download.ExtractOne(fcgidZip, "mod_fcgid.so", filepath.Join(stage, "modules", "mod_fcgid.so")); err != nil {
			os.RemoveAll(stage)
			return fmt.Errorf("apache: install mod_fcgid: %w", err)
		}
	}
	if _, err := os.Stat(dir); err == nil {
		old := dir + ".old"
		os.RemoveAll(old)
		if err := os.Rename(dir, old); err != nil {
			os.RemoveAll(stage)
			return fmt.Errorf("apache: install (is Apache running?): %w", err)
		}
		defer os.RemoveAll(old)
	}
	if err := os.Rename(stage, dir); err != nil {
		return fmt.Errorf("apache: install: %w", err)
	}
	return nil
}
