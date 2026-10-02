package mysql

import (
	"strings"
	"testing"
)

func TestRenderIni(t *testing.T) {
	s := RenderIni(3307, `C:\Apnoro`)
	for _, want := range []string{
		`basedir="C:/Apnoro/mysql"`, `datadir="C:/Apnoro/data/mysql"`, "port=3307",
		"bind-address=127.0.0.1", "mysqlx=OFF", `log-error="C:/Apnoro/logs/mysql.log"`,
		`pid-file="C:/Apnoro/run/mysqld-internal.pid"`, "character-set-server=utf8mb4",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(s, "mysql_native_password") {
		t.Error("keep default auth plugin")
	}
}

func TestNames(t *testing.T) {
	for _, ok := range []string{"blog", "my_app_1", strings.Repeat("a", 64)} {
		if checkName(ok) != nil {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "a-b", "a b", "x`;DROP", strings.Repeat("a", 65), "mysql", "SYS"} {
		if checkName(bad) == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if quoteIdent("a`b") != "`a``b`" {
		t.Error("quoteIdent")
	}
	if sqlString(`it's\`) != `'it\'s\\'` {
		t.Error(sqlString(`it's\`))
	}
}
