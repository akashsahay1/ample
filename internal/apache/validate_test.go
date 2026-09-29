package apache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ampls/internal/paths"
)

func TestWriteConfigRejectsInjection(t *testing.T) {
	home := t.TempDir()
	paths.SetHome(home)
	cgi := filepath.Join(home, "php", "8.4", "php-cgi.exe")
	bad := []VHost{
		{Domain: "a.test", DocRoot: home + "/x\"\nLoadModule evil_module evil.so\n#", PHPCGI: cgi},
		{Domain: "a.test", DocRoot: home + "/x\nInclude /etc/passwd", PHPCGI: cgi},
		{Domain: "a.test", DocRoot: home + "/${AMPLS_HOME}", PHPCGI: cgi},
		{Domain: "a.test\nLoadModule x y", DocRoot: home, PHPCGI: cgi},
		{Domain: "a.test", Aliases: []string{"*.a.test b.test"}, DocRoot: home, PHPCGI: cgi},
		{Domain: "a.test", DocRoot: home, PHPCGI: cgi + "\"x"},
		{Domain: "a.test", DocRoot: home, PHPCGI: cgi, Secure: true, CertFile: "c\n.crt", KeyFile: "k.key"},
	}
	for i, v := range bad {
		if err := WriteConfig(Options{}, []VHost{v}); err == nil {
			t.Errorf("case %d accepted: %+v", i, v)
		}
		if b, err := os.ReadFile(filepath.Join(paths.SitesConfDir(), "a.test.conf")); err == nil && strings.Contains(string(b), "LoadModule") {
			t.Fatalf("case %d: injected directive rendered", i)
		}
	}
	ok := VHost{Domain: "a.test", Aliases: []string{"*.a.test"}, DocRoot: filepath.Join(home, "My Site (1)"), PHPCGI: cgi}
	if err := WriteConfig(Options{}, []VHost{ok}); err != nil {
		t.Fatalf("valid vhost rejected: %v", err)
	}
}
