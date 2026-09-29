package shim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ampls/internal/paths"
)

var home string

func TestMain(m *testing.M) {
	var err error
	home, err = os.MkdirTemp("", "ampls-shim-test")
	if err != nil {
		panic(err)
	}
	paths.SetHome(home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func installPHP(t *testing.T, minor string) {
	t.Helper()
	d := paths.PHPDir(minor)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, paths.Exe("php")), []byte("x"), 0o755)
}

func writeConfig(t *testing.T, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := os.WriteFile(paths.ConfigFile(), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompare(t *testing.T) {
	if compare("8.10", "8.9") != 1 || compare("7.4", "8.0") != -1 || compare("8.3", "8.3") != 0 {
		t.Fatal("compare")
	}
}

func TestResolveVersion(t *testing.T) {
	os.RemoveAll(paths.PHPRoot())
	os.Remove(paths.ConfigFile())
	work := t.TempDir()

	// nothing installed
	if _, _, err := ResolveVersion(work); err != ErrNoPHP {
		t.Fatalf("want ErrNoPHP, got %v", err)
	}

	installPHP(t, "8.2")
	installPHP(t, "8.10")
	installPHP(t, "8.9")
	os.MkdirAll(filepath.Join(paths.PHPRoot(), "junk"), 0o755)
	os.MkdirAll(filepath.Join(paths.PHPRoot(), "7.4"), 0o755) // dir without php.exe
	v, src, err := ResolveVersion(work)
	if err != nil || v != "8.10" || src != "newest installed" {
		t.Fatalf("newest: %s %s %v", v, src, err)
	}

	// default from config
	parked := t.TempDir()
	site := filepath.Join(parked, "blog")
	os.MkdirAll(filepath.Join(site, "app", "Http"), 0o755)
	cfg := map[string]any{"defaultPhp": "8.2", "tld": "test", "parked": []string{parked}}
	writeConfig(t, cfg)
	if v, src, _ = ResolveVersion(site); v != "8.2" || src != "default" {
		t.Fatalf("default: %s %s", v, src)
	}
	// default not installed -> newest
	cfg["defaultPhp"] = "8.1"
	writeConfig(t, cfg)
	if v, _, _ = ResolveVersion(site); v != "8.10" {
		t.Fatalf("missing default: %s", v)
	}
	cfg["defaultPhp"] = "8.2"

	// isolated site
	cfg["sites"] = map[string]any{"blog": map[string]any{"php": "8.9"}}
	writeConfig(t, cfg)
	if v, src, _ = ResolveVersion(filepath.Join(site, "app", "Http")); v != "8.9" || src != "site blog.test" {
		t.Fatalf("site: %s %s", v, src)
	}
	if v, _, _ = ResolveVersion(parked); v != "8.2" {
		t.Fatalf("outside site: %s", v)
	}

	// .ampls-php wins, found walking up
	os.WriteFile(filepath.Join(site, VersionFile), []byte("\xef\xbb\xbf8.3\r\n"), 0o644)
	v, src, _ = ResolveVersion(filepath.Join(site, "app", "Http"))
	if v != "8.3" || !strings.HasPrefix(src, VersionFile) {
		t.Fatalf("file: %s %s", v, src)
	}
	// garbage file ignored
	os.WriteFile(filepath.Join(site, VersionFile), []byte("latest"), 0o644)
	if v, _, _ = ResolveVersion(site); v != "8.9" {
		t.Fatalf("garbage file: %s", v)
	}
}
