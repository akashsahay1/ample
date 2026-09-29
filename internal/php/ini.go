package php

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ampls/internal/paths"
)

// Settings is the editable subset of php.ini.
type Settings struct {
	Values     map[string]string
	Extensions []Ext
}

// Ext is a PHP extension found in ext/.
type Ext struct {
	Name    string
	Enabled bool
}

// SettingKeys are the php.ini values exposed in Settings.Values.
var SettingKeys = []string{"memory_limit", "upload_max_filesize", "post_max_size", "max_execution_time", "display_errors"}

// DefaultExtensions are enabled by EnsureIni when their DLL exists.
var DefaultExtensions = []string{
	"bcmath", "curl", "exif", "fileinfo", "gd", "gd2", "intl", "mbstring", "mysqli",
	"openssl", "pdo_mysql", "pdo_sqlite", "sodium", "sqlite3", "zip",
}

// zendExts are loaded with zend_extension=.
var zendExts = map[string]bool{"opcache": true, "xdebug": true}

func extDir(minor string) string { return filepath.Join(paths.PHPDir(minor), "ext") }

func slash(p string) string { return filepath.ToSlash(p) }

// EnsureIni creates php.ini from php.ini-development with AMPLS defaults if it
// does not exist yet.
func EnsureIni(minor string) error {
	ini := IniPath(minor)
	if _, err := os.Stat(ini); err == nil {
		return nil
	}
	dir := paths.PHPDir(minor)
	var base string
	for _, n := range []string{"php.ini-development", "php.ini-production"} {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			base = string(b)
			break
		}
	}
	if base == "" {
		base = "[PHP]\n"
	}
	values := map[string]string{
		"extension_dir":       `"` + slash(extDir(minor)) + `"`,
		"memory_limit":        "512M",
		"upload_max_filesize": "128M",
		"post_max_size":       "128M",
		"max_execution_time":  "120",
		"date.timezone":       "UTC",
		"error_log":           `"` + slash(filepath.Join(paths.LogsDir(), "php-"+minor+"-error.log")) + `"`,
	}
	cacert := filepath.Join(paths.PHPRoot(), "cacert.pem")
	if _, err := os.Stat(cacert); err == nil {
		values["curl.cainfo"] = `"` + slash(cacert) + `"`
		values["openssl.cafile"] = `"` + slash(cacert) + `"`
	}
	available := availableExts(minor)
	exts := map[string]bool{}
	for _, e := range DefaultExtensions {
		if available[e] {
			exts[e] = true
		}
	}
	if available["opcache"] {
		exts["opcache"] = true
	}
	out := EditIni(base, values, exts)
	if err := os.WriteFile(ini, []byte(out), 0o644); err != nil {
		return fmt.Errorf("php: write php.ini: %w", err)
	}
	return nil
}

// availableExts lists extension names from ext/php_*.dll (or *.so).
func availableExts(minor string) map[string]bool {
	out := map[string]bool{}
	entries, _ := os.ReadDir(extDir(minor))
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		if e.IsDir() {
			continue
		}
		if n == "php_zend_test.dll" || n == "php_dl_test.dll" {
			continue
		}
		if strings.HasPrefix(n, "php_") && strings.HasSuffix(n, ".dll") {
			out[strings.TrimSuffix(strings.TrimPrefix(n, "php_"), ".dll")] = true
		} else if strings.HasSuffix(n, ".so") {
			out[strings.TrimSuffix(n, ".so")] = true
		}
	}
	return out
}

// ReadSettings returns the common ini values and the state of every extension in ext/.
func ReadSettings(minor string) (Settings, error) {
	b, err := os.ReadFile(IniPath(minor))
	if err != nil {
		return Settings{}, fmt.Errorf("php: read php.ini: %w", err)
	}
	vals, enabled := ParseIni(string(b))
	s := Settings{Values: map[string]string{}}
	for _, k := range SettingKeys {
		if v, ok := vals[k]; ok {
			s.Values[k] = v
		}
	}
	names := []string{}
	for n := range availableExts(minor) {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s.Extensions = append(s.Extensions, Ext{Name: n, Enabled: enabled[n]})
	}
	return s, nil
}

// WriteSettings applies s to php.ini in place, preserving everything else.
func WriteSettings(minor string, s Settings) error {
	ini := IniPath(minor)
	b, err := os.ReadFile(ini)
	if err != nil {
		return fmt.Errorf("php: read php.ini: %w", err)
	}
	vals := map[string]string{}
	for k, v := range s.Values {
		k = strings.TrimSpace(k)
		if k == "" || strings.ContainsAny(k, "\r\n=;[]") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("php: invalid setting %q", k)
		}
		vals[k] = strings.TrimSpace(v)
	}
	avail := availableExts(minor)
	exts := map[string]bool{}
	for _, e := range s.Extensions {
		n := normExt(e.Name)
		if e.Enabled && !avail[n] {
			return fmt.Errorf("php: extension %s is not available for PHP %s", n, minor)
		}
		exts[n] = e.Enabled
	}
	out := EditIni(string(b), vals, exts)
	if err := os.WriteFile(ini, []byte(out), 0o644); err != nil {
		return fmt.Errorf("php: write php.ini: %w", err)
	}
	return nil
}

var (
	// directive: optional ';' (no space after it, to skip prose comments), key = value
	directiveRe = regexp.MustCompile(`^(\s*)(;?)([A-Za-z0-9_.]+)\s*=\s*(.*)$`)
	extLineRe   = regexp.MustCompile(`^\s*(;?)\s*(zend_extension|extension)\s*=\s*"?([^";\s]+)"?\s*(;.*)?$`)
)

// normExt turns "php_gd.dll", "gd.so", "C:/x/ext/php_xdebug.dll" into "gd"/"xdebug".
func normExt(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, `\`, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".dll"), ".so")
	return strings.TrimPrefix(s, "php_")
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' {
		if j := strings.Index(v[1:], `"`); j >= 0 {
			return v[1 : j+1]
		}
	}
	if i := strings.Index(v, ";"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// ParseIni returns active (uncommented) values and enabled extensions.
func ParseIni(content string) (map[string]string, map[string]bool) {
	vals := map[string]string{}
	exts := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := extLineRe.FindStringSubmatch(line); m != nil {
			if m[1] == "" {
				exts[normExt(m[3])] = true
			}
			continue
		}
		if m := directiveRe.FindStringSubmatch(line); m != nil && m[2] == "" {
			vals[m[3]] = unquote(m[4])
		}
	}
	return vals, exts
}

// EditIni sets values and extension states in php.ini content. Existing lines
// (active or commented-out) are edited in place; missing ones are appended.
func EditIni(content string, values map[string]string, exts map[string]bool) string {
	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	// values
	for key, val := range values {
		active, commented := -1, -1
		for i, l := range lines {
			m := directiveRe.FindStringSubmatch(l)
			if m == nil || !strings.EqualFold(m[3], key) {
				continue
			}
			if m[2] == "" {
				if active == -1 {
					active = i
				} else {
					lines[i] = ";" + l // drop duplicates so ours wins
				}
			} else if commented == -1 {
				commented = i
			}
		}
		newLine := key + " = " + val
		switch {
		case active >= 0:
			lines[active] = newLine
		case commented >= 0:
			lines[commented] = newLine
		default:
			lines = insertBeforeTrailing(lines, newLine)
		}
	}

	// extensions
	names := make([]string, 0, len(exts))
	for n := range exts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		want := exts[name]
		directive := "extension"
		if zendExts[name] {
			directive = "zend_extension"
		}
		found := false
		lastExt := -1
		for i, l := range lines {
			m := extLineRe.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			// only treat ";extension=x" (no space) as a toggleable commented line
			commentedOK := m[1] == "" || strings.HasPrefix(strings.TrimSpace(l), ";"+m[2])
			if !commentedOK {
				continue
			}
			lastExt = i
			if normExt(m[3]) != name {
				continue
			}
			if want && !found {
				lines[i] = directive + "=" + name
				found = true
			} else if m[1] == "" {
				lines[i] = ";" + strings.TrimLeft(l, " \t")
			}
		}
		if want && !found {
			line := directive + "=" + name
			if lastExt >= 0 {
				lines = append(lines[:lastExt+1], append([]string{line}, lines[lastExt+1:]...)...)
			} else {
				lines = insertBeforeTrailing(lines, line)
			}
		}
	}
	return strings.Join(lines, nl)
}

func insertBeforeTrailing(lines []string, line string) []string {
	if n := len(lines); n > 0 && lines[n-1] == "" {
		return append(lines[:n-1], line, "")
	}
	return append(lines, line)
}
