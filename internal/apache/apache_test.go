package apache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apnoro/internal/paths"
)

func mustContain(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q in:\n%s", sub, s)
		}
	}
}

func TestRenderMain(t *testing.T) {
	home := `C:\Apnoro Data`
	cgi := `C:\Apnoro Data\php\8.5\php-cgi.exe`
	s := RenderMain(Options{HTTPPort: 8080, HTTPSPort: 8443, DefaultPHPCGI: cgi}, home, false)
	mustContain(t, s,
		`ServerRoot "C:/Apnoro Data/apache"`,
		"Listen 127.0.0.1:8080",
		"LoadModule fcgid_module modules/mod_fcgid.so",
		"LoadModule socache_shmcb_module modules/mod_socache_shmcb.so",
		`PidFile "C:/Apnoro Data/run/httpd-internal.pid"`,
		`ErrorLog "C:/Apnoro Data/logs/apache-error.log"`,
		`CustomLog "C:/Apnoro Data/logs/apache-access.log" combined`,
		"FcgidMaxRequestLen 268435456",
		"FcgidInitialEnv SystemRoot",
		"FcgidInitialEnv PHP_FCGI_MAX_REQUESTS 1000",
		`FcgidInitialEnv TEMP "C:/Apnoro Data/tmp"`,
		"<VirtualHost *:8080>",
		`DocumentRoot "C:/Apnoro Data/www"`,
		`Alias /phpmyadmin "C:/Apnoro Data/apps/phpmyadmin"`,
		`FcgidWrapper "C:/Apnoro Data/php/8.5/php-cgi.exe" .php`,
		`FcgidInitialEnv PHPRC "C:/Apnoro Data/php/8.5"`,
		`IncludeOptional "C:/Apnoro Data/conf/sites/*.conf"`,
		`"shmcb:C:/Apnoro Data/run/ssl_scache(512000)"`,
	)
	if strings.Contains(s, "Listen 127.0.0.1:8443") {
		t.Error("https listen without secure vhosts")
	}
	if strings.Contains(s, `\`+"Apnoro") {
		t.Error("backslash path in config")
	}
	s2 := RenderMain(Options{DefaultPHPCGI: cgi}, home, true)
	mustContain(t, s2, "Listen 127.0.0.1:80\n", "Listen 127.0.0.1:443\n")
	// default vhost must precede the sites include
	if strings.Index(s, "ServerName localhost\n    ServerAlias") > strings.Index(s, "IncludeOptional") {
		t.Error("localhost vhost must come first")
	}
}

func TestRenderVHost(t *testing.T) {
	v := VHost{
		Domain: "blog.test", Aliases: []string{"*.blog.test"},
		DocRoot: `C:\Users\me\Sites\blog\public`, PHPCGI: `C:\Apnoro\php\8.3\php-cgi.exe`,
	}
	s := RenderVHost(v, Options{})
	mustContain(t, s,
		"<VirtualHost *:80>", "ServerName blog.test", "ServerAlias *.blog.test",
		`DocumentRoot "C:/Users/me/Sites/blog/public"`,
		`<Directory "C:/Users/me/Sites/blog/public">`,
		"Options Indexes FollowSymLinks ExecCGI", "AllowOverride All", "Require all granted",
		"SetHandler fcgid-script", `FcgidWrapper "C:/Apnoro/php/8.3/php-cgi.exe" .php`,
		"DirectoryIndex index.php index.html",
	)
	if strings.Contains(s, "SSLEngine") {
		t.Error("not secure")
	}
	v.Secure, v.CertFile, v.KeyFile = true, `C:\Apnoro\certs\sites\blog.test.crt`, `C:\Apnoro\certs\sites\blog.test.key`
	s = RenderVHost(v, Options{HTTPSPort: 8443})
	mustContain(t, s, "<VirtualHost *:80>", "<VirtualHost *:8443>", "SSLEngine on",
		`SSLCertificateFile "C:/Apnoro/certs/sites/blog.test.crt"`,
		`SSLCertificateKeyFile "C:/Apnoro/certs/sites/blog.test.key"`)
	if strings.Contains(s, "Redirect") {
		t.Error("no forced redirect")
	}
}

func TestConfFileName(t *testing.T) {
	for in, want := range map[string]string{
		"blog.test": "blog.test.conf", "*.Blog.test": "_.blog.test.conf",
		"../../evil": "_.._evil.conf", "": "site.conf",
	} {
		if got := ConfFileName(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestWriteConfig(t *testing.T) {
	home := t.TempDir()
	paths.SetHome(home)
	os.MkdirAll(paths.SitesConfDir(), 0o755)
	os.WriteFile(filepath.Join(paths.SitesConfDir(), "stale.test.conf"), []byte("x"), 0o644)
	err := WriteConfig(Options{DefaultPHPCGI: `C:\x\php-cgi.exe`}, []VHost{
		{Domain: "a.test", DocRoot: home, PHPCGI: `C:\x\php-cgi.exe`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.SitesConfDir(), "stale.test.conf")); !os.IsNotExist(err) {
		t.Error("stale conf not removed")
	}
	for _, p := range []string{MainConfPath(), filepath.Join(paths.SitesConfDir(), "a.test.conf"), filepath.Join(paths.WWWDir(), "index.php")} {
		if _, err := os.Stat(p); err != nil {
			t.Error(err)
		}
	}
	args := StartArgs()
	if len(args) != 4 || args[0] != "-f" || args[2] != "-d" || strings.Contains(args[1], `\`) {
		t.Errorf("args %v", args)
	}
}
