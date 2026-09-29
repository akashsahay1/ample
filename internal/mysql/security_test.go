package mysql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ampls/internal/paths"
)

func TestClientDefaultsEscaping(t *testing.T) {
	s := renderClientDefaults(3306, "p\"w\\x\ninit-command=DROP DATABASE x\r\n[mysqldump]")
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) != 6 {
		t.Fatalf("password injected extra lines:\n%s", s)
	}
	if lines[2] != `password="p\"w\\x\ninit-command=DROP DATABASE x\r\n[mysqldump]"` {
		t.Fatalf("password line: %s", lines[2])
	}
}

func TestClientDefaultsNotInSharedHome(t *testing.T) {
	home := t.TempDir()
	paths.SetHome(home)
	defer paths.SetHome("")
	p, err := clientDefaults(3306, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	if strings.HasPrefix(strings.ToLower(p), strings.ToLower(filepath.Clean(home))) {
		t.Fatalf("password file %s written inside the shared data dir", p)
	}
}

func TestDatabaseNameValidation(t *testing.T) {
	for _, bad := range []string{"", "a`b", "a b", "x;DROP", "--help", "../x", "mysql", "SYS", strings.Repeat("a", 65)} {
		if checkName(bad) == nil {
			t.Errorf("checkName(%q) accepted", bad)
		}
	}
	if checkName("my_app1") != nil {
		t.Error("valid name rejected")
	}
}
