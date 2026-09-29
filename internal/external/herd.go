package external

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ampls/internal/php"
)

// Laravel Herd for Windows keeps its state under %USERPROFILE%\.config\herd:
//
//	config\valet\config.json        {"tld":"test","paths":[<parked dirs>...]}
//	config\valet\Sites\<name>       symlink/junction per linked site (this dir is also in "paths")
//	config\valet\Nginx\<name>.<tld>.conf   per-site nginx config written for isolated/secured
//	                                sites; first line "# ISOLATED_PHP_VERSION=8.4" when isolated
//	config\valet\Certificates\<name>.<tld>.crt/.key   secured sites
//	config\php.json                 {"installed_8.4":"8.4.25", ...}
//	config\config.json              app settings, "activeVersion":"8.5" (global PHP)
//	bin\php84\php.exe, bin\php85\   bundled PHP builds; bin\php.bat -> active version
//	bin\herd.bat + herd.phar        CLI (needs the desktop app running)

// HerdPHP is one PHP build bundled with Herd.
type HerdPHP struct {
	Minor string // "8.4"
	Full  string // "8.4.25" (from php.json, else Minor)
	Dir   string // ...\bin\php84
}

// HerdLink is one entry of Herd's Sites (links) directory.
type HerdLink struct {
	Name string
	Path string // link target
}

// HerdInfo is what AMPLS reads from Herd's configuration.
type HerdInfo struct {
	ConfigDir string            // %USERPROFILE%\.config\herd
	TLD       string            // "test"
	Paths     []string          // parked directories (excluding the links dir)
	LinksDir  string            // config\valet\Sites
	Links     []HerdLink        // linked sites
	Secured   map[string]bool   // site name -> has certificate
	Isolated  map[string]string // site name -> PHP minor
	PHP       []HerdPHP         // bundled versions, newest first
	ActivePHP string            // global PHP minor
	Services  []string          // Herd Pro services that appear configured (best effort)
}

// PHPFull returns the full version for a minor.
func (h HerdInfo) PHPFull(minor string) (string, bool) {
	for _, p := range h.PHP {
		if p.Minor == minor {
			return p.Full, true
		}
	}
	return "", false
}

// IsLinksDir reports whether dir is Herd's links directory (config\valet\Sites).
func (h HerdInfo) IsLinksDir(dir string) bool {
	if h.LinksDir != "" && normPath(dir) == normPath(h.LinksDir) {
		return true
	}
	return strings.HasSuffix(normPath(dir), "/.config/herd/config/valet/sites")
}

var isolatedRe = regexp.MustCompile(`^#\s*ISOLATED_PHP_VERSION\s*=\s*([0-9]+\.[0-9]+)`)
var phpDirRe = regexp.MustCompile(`^php(\d)(\d+)$`)

// ReadHerd parses Herd's configuration directory (read-only).
func ReadHerd(configDir string) (HerdInfo, error) {
	h := HerdInfo{
		ConfigDir: configDir, TLD: "test",
		LinksDir: filepath.Join(configDir, "config", "valet", "Sites"),
		Secured:  map[string]bool{}, Isolated: map[string]string{},
	}
	valet := filepath.Join(configDir, "config", "valet")
	var vc struct {
		TLD   string   `json:"tld"`
		Paths []string `json:"paths"`
	}
	b, err := os.ReadFile(filepath.Join(valet, "config.json"))
	if err == nil {
		if err := json.Unmarshal(b, &vc); err != nil {
			return h, err
		}
		if vc.TLD != "" {
			h.TLD = strings.TrimPrefix(vc.TLD, ".")
		}
		for _, p := range vc.Paths {
			if p == "" || h.IsLinksDir(p) {
				continue
			}
			h.Paths = append(h.Paths, filepath.Clean(p))
		}
	}

	// Links: every entry of the Sites dir (symlink/junction, or a plain dir).
	if ents, err := os.ReadDir(h.LinksDir); err == nil {
		for _, e := range ents {
			full := filepath.Join(h.LinksDir, e.Name())
			target := full
			if t, err := os.Readlink(full); err == nil && t != "" {
				if !filepath.IsAbs(t) {
					t = filepath.Join(h.LinksDir, t)
				}
				target = filepath.Clean(strings.TrimPrefix(t, `\\?\`))
			} else if !e.IsDir() {
				continue
			}
			h.Links = append(h.Links, HerdLink{Name: e.Name(), Path: target})
		}
	}

	suffix := "." + h.TLD
	if ents, err := os.ReadDir(filepath.Join(valet, "Certificates")); err == nil {
		for _, e := range ents {
			n := strings.ToLower(e.Name())
			if strings.HasSuffix(n, suffix+".crt") {
				h.Secured[strings.TrimSuffix(n, suffix+".crt")] = true
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(valet, "Nginx")); err == nil {
		for _, e := range ents {
			n := e.Name()
			site := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(n, ".conf"), suffix))
			if e.IsDir() || site == "" {
				continue
			}
			minor, ssl := scanHerdNginx(filepath.Join(valet, "Nginx", n))
			if minor != "" {
				h.Isolated[site] = minor
			}
			if ssl {
				h.Secured[site] = true
			}
		}
	}

	// Bundled PHP.
	fulls := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(configDir, "config", "php.json")); err == nil {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			for k, v := range m {
				if s, ok := v.(string); ok && strings.HasPrefix(k, "installed_") && !strings.HasPrefix(k, "installed_internal") {
					fulls[strings.TrimPrefix(k, "installed_")] = s
				}
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(configDir, "bin")); err == nil {
		for _, e := range ents {
			m := phpDirRe.FindStringSubmatch(strings.ToLower(e.Name()))
			if !e.IsDir() || m == nil || !isFile(filepath.Join(configDir, "bin", e.Name(), "php.exe")) {
				continue
			}
			minor := m[1] + "." + m[2]
			full := fulls[minor]
			if full == "" {
				full = minor
			}
			h.PHP = append(h.PHP, HerdPHP{Minor: minor, Full: full, Dir: filepath.Join(configDir, "bin", e.Name())})
		}
	}
	sort.Slice(h.PHP, func(i, j int) bool { return php.Compare(h.PHP[i].Minor, h.PHP[j].Minor) > 0 })

	if b, err := os.ReadFile(filepath.Join(configDir, "config", "config.json")); err == nil {
		var c struct {
			ActiveVersion string `json:"activeVersion"`
		}
		if json.Unmarshal(b, &c) == nil {
			h.ActivePHP = c.ActiveVersion
		}
	}
	if h.ActivePHP == "" && len(h.PHP) > 0 {
		h.ActivePHP = h.PHP[0].Minor
	}
	h.Services = herdServices(configDir)
	return h, nil
}

// scanHerdNginx reads a per-site nginx config: isolated PHP minor and whether it listens with ssl.
func scanHerdNginx(file string) (minor string, ssl bool) {
	f, err := os.Open(file)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := isolatedRe.FindStringSubmatch(line); m != nil {
			minor = m[1]
			continue
		}
		if strings.HasPrefix(line, "listen ") && strings.Contains(line, "ssl") {
			ssl = true
		}
	}
	return minor, ssl
}

var herdServiceNames = []string{"mysql", "mariadb", "postgres", "redis", "meilisearch", "minio", "typesense", "mongodb", "reverb"}

// herdServices looks for Herd Pro service definitions: any *.json whose name
// mentions "service" under config\ (Herd Pro stores its services there), and
// service binaries under bin\. Best effort: Herd Pro is not installed on the
// development machine, so the exact layout is unverified.
func herdServices(configDir string) []string {
	found := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(configDir, "config"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(normPath(p), "/")-strings.Count(normPath(configDir), "/") > 3 {
				return filepath.SkipDir
			}
			return nil
		}
		n := strings.ToLower(d.Name())
		if !strings.HasSuffix(n, ".json") || !strings.Contains(n, "service") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 4<<20 {
			return nil
		}
		low := strings.ToLower(string(b))
		for _, s := range herdServiceNames {
			if strings.Contains(low, `"`+s) {
				found[s] = true
			}
		}
		return nil
	})
	for _, s := range herdServiceNames {
		if isDir(filepath.Join(configDir, "bin", s)) || isDir(filepath.Join(configDir, "services", s)) {
			found[s] = true
		}
	}
	var out []string
	for _, s := range herdServiceNames {
		if found[s] {
			out = append(out, s)
		}
	}
	return out
}
