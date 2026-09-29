package projects

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateEnvNoNewlineInjection(t *testing.T) {
	out := UpdateEnv("APP_NAME=x\nDB_PASSWORD=\n", [][2]string{{"DB_PASSWORD", "pw\nAPP_DEBUG=true\r\nX=1"}})
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "APP_DEBUG") || strings.HasPrefix(l, "X=") {
			t.Fatalf("value injected a new line: %q", out)
		}
	}
	if !strings.Contains(out, `DB_PASSWORD="pw\nAPP_DEBUG=true\r\nX=1"`) {
		t.Fatalf("unexpected encoding: %q", out)
	}
}

func TestCreateRejectsBadDBName(t *testing.T) {
	dir := t.TempDir()
	for _, db := range []string{"a'b", "x\ny", "a b", "../x"} {
		if _, err := Create(context.Background(), Request{Kind: "blank", Name: "ok", Dir: dir, DB: db}, nil); err == nil {
			t.Errorf("accepted db %q", db)
		}
	}
}

func writeZip(t *testing.T, names ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
	}
	zw.Close()
	f.Close()
	return p
}

func TestUnzipStripZipSlip(t *testing.T) {
	for _, evil := range []string{"wordpress/../../evil.php", `wordpress/..\..\evil.php`, "wordpress/../x/../../evil.php"} {
		root := t.TempDir()
		dest := filepath.Join(root, "site")
		z := writeZip(t, "wordpress/index.php", evil)
		if err := unzipStrip(z, dest, "wordpress/"); err == nil {
			t.Errorf("%q: zip-slip accepted", evil)
		}
		if _, err := os.Stat(filepath.Join(root, "evil.php")); err == nil {
			t.Errorf("%q: file written outside dest", evil)
		}
	}
}
