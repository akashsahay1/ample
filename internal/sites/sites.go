// Package sites discovers the sites AMPLS serves: every immediate subfolder of a
// parked directory plus explicitly linked folders, each served as <name>.<tld>.
package sites

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"ampls/internal/api"
	"ampls/internal/config"
)

var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidName reports whether name is usable as a DNS label / site name.
func ValidName(name string) bool {
	return len(name) <= 63 && validName.MatchString(name)
}

// Slug turns a folder name into a site name: "My Blog" -> "my-blog".
// Characters outside [a-z0-9] become dashes; runs of dashes collapse and
// leading/trailing dashes are trimmed. The result may be "" (invalid).
func Slug(folder string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(folder) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-")
	}
	return s
}

// SafePath reports whether p can be written into generated Apache config as a
// quoted path. Folder names come from the filesystem (on macOS they may contain
// quotes, backslashes and newlines), so paths with double quotes, control
// characters, Apache ${VAR} interpolation or (outside Windows) backslashes are
// refused instead of being rendered, which would let a folder name inject
// directives.
func SafePath(p string) bool {
	if p == "" || strings.Contains(p, `"`) || strings.Contains(p, "${") {
		return false
	}
	// q() only escapes quotes; a trailing backslash would escape the closing
	// quote (backslashes are separators on Windows and are converted to /).
	if runtime.GOOS != "windows" && strings.HasSuffix(p, `\`) {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			return false
		}
	}
	return true
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// DocRoot returns the folder to serve for a project: public/ (Laravel, Symfony),
// web/ (Craft, Drupal composer layouts), else the project folder itself.
func DocRoot(path string) string {
	for _, sub := range []string{"public", "web"} {
		d := filepath.Join(path, sub)
		if isDir(d) {
			return d
		}
	}
	return path
}

// DetectFramework returns "laravel", "wordpress" or "php".
func DetectFramework(path string) string {
	if isFile(filepath.Join(path, "artisan")) {
		return "laravel"
	}
	if isFile(filepath.Join(path, "wp-config.php")) || isFile(filepath.Join(path, "wp-load.php")) {
		return "wordpress"
	}
	return "php"
}

func build(cfg *config.Config, name, path string, linked bool) api.Site {
	tld := cfg.TLD
	if tld == "" {
		tld = "test"
	}
	ov := cfg.Sites[name]
	s := api.Site{
		Name:      name,
		Domain:    name + "." + tld,
		Path:      path,
		Secure:    ov.Secure,
		Linked:    linked,
		Framework: DetectFramework(path),
	}
	scheme := "http"
	if s.Secure {
		scheme = "https"
	}
	s.URL = scheme + "://" + s.Domain
	if ov.PHP != "" {
		s.PHP, s.Isolated = ov.PHP, true
	} else {
		s.PHP = cfg.DefaultPHP
	}
	if ov.DocRoot != "" {
		if filepath.IsAbs(ov.DocRoot) {
			s.DocRoot = filepath.Clean(ov.DocRoot)
		} else {
			s.DocRoot = filepath.Join(path, ov.DocRoot)
		}
	} else {
		s.DocRoot = DocRoot(path)
	}
	return s
}

// Discover lists parked subfolders and links, sorted by name. A link wins over a
// parked folder of the same name; between parked dirs the first one wins.
func Discover(cfg *config.Config) ([]api.Site, error) {
	byName := map[string]api.Site{}
	for _, l := range cfg.Links {
		name := l.Name
		if !ValidName(name) {
			name = Slug(name)
		}
		if !ValidName(name) || l.Path == "" {
			continue
		}
		if _, dup := byName[name]; dup {
			continue
		}
		p, err := filepath.Abs(l.Path)
		if err != nil {
			p = l.Path
		}
		if s := build(cfg, name, p, true); SafePath(s.Path) && SafePath(s.DocRoot) {
			byName[name] = s
		}
	}
	for _, dir := range cfg.Parked {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // a missing parked dir is not fatal
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, ".") {
				continue
			}
			full := filepath.Join(abs, n)
			if !e.IsDir() {
				// follow symlinks / junctions to directories
				if e.Type()&os.ModeSymlink == 0 || !isDir(full) {
					continue
				}
			}
			name := Slug(n)
			if !ValidName(name) {
				continue
			}
			if _, dup := byName[name]; dup {
				continue
			}
			if s := build(cfg, name, full, false); SafePath(s.Path) && SafePath(s.DocRoot) {
				byName[name] = s
			}
		}
	}
	out := make([]api.Site, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Find returns the site with the given name.
func Find(cfg *config.Config, name string) (api.Site, bool) {
	all, _ := Discover(cfg)
	for _, s := range all {
		if s.Name == name {
			return s, true
		}
	}
	return api.Site{}, false
}

func normPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		p = a
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

func within(dir, root string) bool {
	if dir == root {
		return true
	}
	sep := string(filepath.Separator)
	if strings.HasSuffix(root, sep) {
		return strings.HasPrefix(dir, root)
	}
	return strings.HasPrefix(dir, root+sep)
}

// FindByPath returns the site whose folder contains dir (the deepest match wins).
func FindByPath(cfg *config.Config, dir string) (api.Site, bool) {
	all, _ := Discover(cfg)
	d := normPath(dir)
	var best api.Site
	bestLen := -1
	for _, s := range all {
		root := normPath(s.Path)
		if within(d, root) && len(root) > bestLen {
			best, bestLen = s, len(root)
		}
	}
	return best, bestLen >= 0
}
