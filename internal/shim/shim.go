// Package shim decides which PHP version the `php` command-line shim runs.
package shim

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ampls/internal/config"
	"ampls/internal/paths"
	"ampls/internal/sites"
)

// VersionFile pins a PHP minor version for a directory tree (content e.g. "8.3").
const VersionFile = ".ampls-php"

// ErrNoPHP is returned when no PHP version is installed.
var ErrNoPHP = errors.New("no PHP version is installed in AMPLS (install one with `ampls php:install 8.4` or from the AMPLS app)")

var minorRe = regexp.MustCompile(`^\d+\.\d+$`)

// ResolveVersion picks the PHP minor version for cwd:
//  1. nearest .ampls-php file walking up to the filesystem root
//  2. the isolated PHP of the AMPLS site containing cwd
//  3. cfg.DefaultPHP (if installed)
//  4. the newest installed version
//
// source describes where the answer came from.
func ResolveVersion(cwd string) (minor string, source string, err error) {
	if abs, e := filepath.Abs(cwd); e == nil {
		cwd = abs
	}
	if v, file := findVersionFile(cwd); v != "" {
		return v, VersionFile + " (" + file + ")", nil
	}
	cfg, cerr := config.Load()
	if cerr == nil {
		pinned := false
		for _, s := range cfg.Sites {
			if s.PHP != "" {
				pinned = true
				break
			}
		}
		if pinned { // only walk the sites when some site is isolated (keeps the shim fast)
			// config.json is user-editable: only accept a strict "<major>.<minor>"
			// so the version can never become a path ("../../evil").
			if s, ok := sites.FindByPath(cfg, cwd); ok && s.Isolated && minorRe.MatchString(s.PHP) {
				return s.PHP, "site " + s.Domain, nil
			}
		}
		if minorRe.MatchString(cfg.DefaultPHP) && installed(cfg.DefaultPHP) {
			return cfg.DefaultPHP, "default", nil
		}
	}
	if v := newestInstalled(); v != "" {
		return v, "newest installed", nil
	}
	return "", "", ErrNoPHP
}

func findVersionFile(dir string) (version, file string) {
	for {
		p := filepath.Join(dir, VersionFile)
		if b, err := readHead(p, 64); err == nil {
			line := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(string(b), "\xef\xbb\xbf"), "\n", 2)[0])
			if minorRe.MatchString(line) {
				return line, p
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// readHead reads at most n bytes of a regular file (a .ampls-php from an
// untrusted checkout could be huge or a device/pipe).
func readHead(p string, n int64) ([]byte, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, n))
}

func installed(minor string) bool {
	if !minorRe.MatchString(minor) {
		return false
	}
	_, err := os.Stat(filepath.Join(paths.PHPDir(minor), paths.Exe("php")))
	return err == nil
}

// Installed lists installed PHP minors under paths.PHPRoot(), newest first.
func Installed() []string {
	entries, err := os.ReadDir(paths.PHPRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && minorRe.MatchString(e.Name()) && installed(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { return compare(out[i], out[j]) > 0 })
	return out
}

func newestInstalled() string {
	if v := Installed(); len(v) > 0 {
		return v[0]
	}
	return ""
}

// compare compares dotted numeric versions ("8.10" > "8.9").
func compare(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
