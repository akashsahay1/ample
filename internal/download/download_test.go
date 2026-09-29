package download

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
}

func TestUnzipStrip(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	makeZip(t, z, map[string]string{"Apache24/bin/httpd.exe": "x", "Apache24/conf/httpd.conf": "y"})
	out := filepath.Join(dir, "out")
	if err := Unzip(z, out, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "bin", "httpd.exe")); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join(dir, "out2")
	if err := Unzip(z, out2, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out2, "Apache24", "bin", "httpd.exe")); err != nil {
		t.Fatal(err)
	}
}

func TestUnzipNoStripWhenMultipleTop(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	makeZip(t, z, map[string]string{"php.exe": "x", "ext/php_curl.dll": "y"})
	out := filepath.Join(dir, "out")
	if err := Unzip(z, out, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "ext", "php_curl.dll")); err != nil {
		t.Fatal(err)
	}
}

func TestUnzipSlip(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	makeZip(t, z, map[string]string{"../evil.txt": "x"})
	if err := Unzip(z, filepath.Join(dir, "out"), false); err == nil {
		t.Fatal("expected zip-slip error")
	}
}

func TestFile(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.UserAgent()
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "f.txt")
	var got int64
	if err := File(context.Background(), srv.URL, dest, func(d, _ int64) { got = d }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "hello" || got != 5 || ua != UserAgent {
		t.Fatalf("bad: %q %d %q", b, got, ua)
	}
	sum, _ := SHA256File(dest)
	if sum != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatal(sum)
	}
}

func TestUnzipSlipVariants(t *testing.T) {
	cases := []map[string]string{
		{"top/a.txt": "x", "top/../../evil.txt": "x"}, // escapes after stripTopDir
		{"top/a.txt": "x", `top/..\..\evil.txt`: "x"}, // backslash separators
	}
	for i, files := range cases {
		dir := t.TempDir()
		z := filepath.Join(dir, "a.zip")
		makeZip(t, z, files)
		if err := Unzip(z, filepath.Join(dir, "sub", "out"), true); err == nil {
			t.Errorf("case %d: expected zip-slip error", i)
		}
		if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
			t.Errorf("case %d: wrote outside dest", i)
		}
	}
	dir := t.TempDir()
	z := filepath.Join(dir, "b.zip")
	makeZip(t, z, map[string]string{"Apache24/bin/x": "x", "Apache24/../evil.txt": "x"})
	if err := UnzipSubdir(z, "Apache24", filepath.Join(dir, "out")); err == nil {
		t.Error("UnzipSubdir: expected zip-slip error")
	}
}
