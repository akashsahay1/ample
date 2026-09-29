package external

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ampls/internal/api"
	"ampls/internal/paths"
)

func TestMain(m *testing.M) {
	paths.SetHome(filepath.Join(os.TempDir(), "ampls-external-test-home"))
	os.Exit(m.Run())
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// herdFixture copies testdata/herd to a temp dir, substituting {{ROOT}} in
// valet/config.json with that dir (JSON-escaped).
func herdFixture(t *testing.T) string {
	t.Helper()
	src := fixture(t, "herd")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if d.Name() == "config.json" {
			root := strings.ReplaceAll(dst, `\`, `\\`)
			b = []byte(strings.ReplaceAll(string(b), "{{ROOT}}", root))
			if filepath.Separator == '/' {
				b = []byte(strings.ReplaceAll(string(b), `\\`, `/`))
			}
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestUnder(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{`C:\xampp\apache\bin\httpd.exe`, `C:\xampp`, true},
		{`c:\XAMPP\mysql\bin\mysqld.exe`, `C:\xampp\`, true},
		{`C:\xampp2\apache\bin\httpd.exe`, `C:\xampp`, false},
		{`C:\xampp`, `C:\xampp`, true},
		{`D:\AMPLS\apache\bin\httpd.exe`, `C:\xampp`, false},
		{``, `C:\xampp`, false},
	}
	for _, c := range cases {
		if got := Under(c.path, c.dir); got != c.want {
			t.Errorf("Under(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}

func TestClassifyPath(t *testing.T) {
	roots := map[string][]string{
		api.EnvXAMPP: {`E:\dev\stack`},
		api.EnvHerd:  {`C:\Users\u\.config\herd`, `C:\Program Files\Herd`},
	}
	cases := []struct{ exe, want string }{
		{`E:\dev\stack\apache\bin\httpd.exe`, api.EnvXAMPP},
		{`C:\xampp\mysql\bin\mysqld.exe`, api.EnvXAMPP},
		{`C:\Program Files\Herd\resources\app.asar.unpacked\resources\bin\nginx\nginx.exe`, api.EnvHerd},
		{`C:\Users\u\.config\herd\bin\php85\php-cgi.exe`, api.EnvHerd},
		{`C:\laragon\bin\apache\httpd-2.4\bin\httpd.exe`, api.EnvLaragon},
		{`C:\wamp64\bin\mysql\mysql8.3.0\bin\mysqld.exe`, api.EnvWAMP},
		{`C:\Windows\System32\svchost.exe`, ""},
		{`C:\tools\xampp.exe`, ""}, // file name, not a directory segment
		{filepath.Join(paths.Home(), "apache", "bin", "httpd.exe"), EnvAMPLS},
	}
	for _, c := range cases {
		if got := classifyPath(c.exe, roots); got != c.want {
			t.Errorf("classifyPath(%q) = %q, want %q", c.exe, got, c.want)
		}
	}
}

func TestPortOwnerIn(t *testing.T) {
	procs := []Proc{
		{PID: 10, Name: "httpd.exe", Exe: `C:\xampp\apache\bin\httpd.exe`, Ports: []int{80, 443}},
		{PID: 11, Name: "mysqld.exe", Exe: filepath.Join(paths.Home(), "mysql", "bin", "mysqld.exe"), Ports: []int{3306}},
		{PID: 4, Name: "System", Ports: []int{8080}},
	}
	c, ok := portOwnerIn(443, procs, nil)
	if !ok || c.Env != api.EnvXAMPP || c.Process != "httpd.exe" || c.Port != 443 {
		t.Errorf("443: %+v %v", c, ok)
	}
	c, ok = portOwnerIn(3306, procs, nil)
	if !ok || c.Env != EnvAMPLS {
		t.Errorf("3306: %+v %v", c, ok)
	}
	c, ok = portOwnerIn(8080, procs, nil)
	if !ok || !strings.HasPrefix(c.Process, "System") {
		t.Errorf("8080: %+v %v", c, ok)
	}
}

func TestEnvProcessesAndIsServer(t *testing.T) {
	procs := []Proc{
		{PID: 1, Name: "xampp-control.exe", Exe: `C:\xampp\xampp-control.exe`},
		{PID: 2, Name: "httpd.exe", Exe: `C:\xampp\apache\bin\httpd.exe`},
		{PID: 3, Name: "httpd.exe", Exe: `C:\AMPLS\apache\bin\httpd.exe`},
		{PID: 4, Name: "mysqld.exe"}, // exe unknown: never matched
	}
	got := EnvProcesses(procs, []string{`C:\xampp`})
	if len(got) != 2 {
		t.Fatalf("EnvProcesses = %+v", got)
	}
	if IsServer(got[0]) || !IsServer(got[1]) {
		t.Errorf("IsServer wrong for %+v", got)
	}
}

func TestReadXAMPP(t *testing.T) {
	root := fixture(t, "xampp")
	x := ReadXAMPP(root)
	if x.Version != "8.2.12" || x.PHPVersion != "8.2.12" || x.MariaDBVersion != "10.4.32" {
		t.Errorf("versions: %+v", x)
	}
	if x.MySQLPort != 3307 {
		t.Errorf("MySQLPort = %d, want 3307 (from [mysqld], not [client])", x.MySQLPort)
	}
	if x.MyIni == "" || x.Htdocs != filepath.Join(root, "htdocs") {
		t.Errorf("paths: %+v", x)
	}
}

func TestIniValue(t *testing.T) {
	ini := "[client]\nport=3306\n[mysqld]\n# port=1\nport = 3310 ; c\ndatadir=\"C:/x y/data\"\nkey-buffer=16M\n"
	cases := []struct{ sec, key, want string }{
		{"mysqld", "port", "3310"},
		{"client", "port", "3306"},
		{"mysqld", "datadir", "C:/x y/data"},
		{"mysqld", "key_buffer", "16M"},
		{"mysqld", "socket", ""},
	}
	for _, c := range cases {
		if got := IniValue(ini, c.sec, c.key); got != c.want {
			t.Errorf("IniValue(%s,%s) = %q, want %q", c.sec, c.key, got, c.want)
		}
	}
}

func TestParseVHosts(t *testing.T) {
	conf := `
Define PROJ "D:/work"
##<VirtualHost *:80>
##    ServerName dummy-host.example.com
##    DocumentRoot "C:/xampp/htdocs/dummy-host.example.com"
##</VirtualHost>
<VirtualHost *:80>
    DocumentRoot "C:/xampp/htdocs/"
    ServerName localhost
</VirtualHost>
<VirtualHost *:80>
	ServerName blog.local
	DocumentRoot "${PROJ}/blog/public"
</VirtualHost>
<VirtualHost *:443>
	ServerName blog.local:443
	DocumentRoot "${PROJ}/blog/public"
	SSLEngine on
</VirtualHost>
<VirtualHost 127.0.0.1:80>
	ServerName   shop.test
	DocumentRoot C:/sites/shop
</VirtualHost>
<VirtualHost *:80>
	ServerName nodocroot.test
</VirtualHost>
`
	got := ParseVHosts(conf, nil, `C:\xampp\htdocs`)
	want := []VHost{
		{ServerName: "blog.local", DocumentRoot: filepath.FromSlash("D:/work/blog/public"), SSL: true},
		{ServerName: "shop.test", DocumentRoot: filepath.FromSlash("C:/sites/shop")},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestVHostSite(t *testing.T) {
	s := VHostSite(VHost{ServerName: "www.my-blog.local", DocumentRoot: filepath.FromSlash("/x/blog/public")})
	if s.Name != "my-blog" || s.Path != filepath.FromSlash("/x/blog") || s.DocRoot != filepath.FromSlash("/x/blog/public") || s.Source != "vhost" {
		t.Errorf("%+v", s)
	}
	if s.Conflict != "folder not found" {
		t.Errorf("missing folder not flagged: %+v", s)
	}
}

func TestSourceSitesXAMPP(t *testing.T) {
	root := fixture(t, "xampp")
	list, parked, notes, err := SourceSites(api.EnvXAMPP, root)
	if err != nil {
		t.Fatal(err)
	}
	htdocs := filepath.Join(root, "htdocs")
	if len(parked) != 1 || parked[0] != htdocs {
		t.Errorf("parked = %v", parked)
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "PHP 8.2.12") {
		t.Errorf("notes = %v", notes)
	}
	by := map[string]api.ImportSite{}
	for _, s := range list {
		by[s.Name] = s
	}
	if len(list) != 3 {
		t.Fatalf("sites = %+v", list)
	}
	blog := by["blog"]
	if blog.Source != "htdocs" || blog.Domain != "localhost/blog" || blog.Path != filepath.Join(htdocs, "blog") || blog.DocRoot != blog.Path {
		t.Errorf("blog = %+v", blog)
	}
	// "My Shop" folder is replaced by its vhost (shop.local, https, public docroot).
	shop := by["shop"]
	if shop.Source != "vhost" || shop.Domain != "shop.local" || !shop.Secure ||
		shop.Path != filepath.Join(htdocs, "My Shop") || shop.DocRoot != filepath.Join(htdocs, "My Shop", "public") || shop.Conflict != "" {
		t.Errorf("shop = %+v", shop)
	}
	if _, ok := by["my-shop"]; ok {
		t.Errorf("htdocs duplicate of vhost kept")
	}
	if api := by["api"]; api.Conflict != "folder not found" || api.Domain != "www.api.example.test" {
		t.Errorf("api = %+v", api)
	}
	for _, skip := range []string{"dashboard", "img"} {
		if _, ok := by[skip]; ok {
			t.Errorf("%s should be skipped", skip)
		}
	}
}

func TestDataDirDatabases(t *testing.T) {
	dbs := DataDirDatabases(filepath.Join(fixture(t, "xampp"), "mysql", "data"))
	if len(dbs) != 2 {
		t.Fatalf("dbs = %+v", dbs)
	}
	if dbs[0].Name != "blogdb" || dbs[0].Tables != 2 {
		t.Errorf("blogdb = %+v", dbs[0])
	}
	if dbs[1].Name != "shop-db" || dbs[1].Tables != 1 || dbs[1].SizeBytes == 0 {
		t.Errorf("shop-db = %+v", dbs[1])
	}
}

func TestDecodeDirName(t *testing.T) {
	for in, want := range map[string]string{"my@002dapp": "my-app", "plain": "plain", "a@0020b@002ec": "a b.c", "bad@zz": "bad@zz"} {
		if got := DecodeDirName(in); got != want {
			t.Errorf("DecodeDirName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadHerd(t *testing.T) {
	root := herdFixture(t)
	h, err := ReadHerd(root)
	if err != nil {
		t.Fatal(err)
	}
	if h.TLD != "test" || h.ActivePHP != "8.5" {
		t.Errorf("tld/active: %+v", h)
	}
	if len(h.Paths) != 2 || h.Paths[0] != filepath.Join(root, "Herd") {
		t.Errorf("paths (links dir must be excluded) = %v", h.Paths)
	}
	if len(h.Links) != 1 || h.Links[0].Name != "backend" {
		t.Errorf("links = %+v", h.Links)
	}
	if !h.Secured["backend"] || h.Secured["blog"] {
		t.Errorf("secured = %v", h.Secured)
	}
	if h.Isolated["backend"] != "8.3" || h.Isolated["blog"] != "8.4" {
		t.Errorf("isolated = %v", h.Isolated)
	}
	if len(h.PHP) != 2 || h.PHP[0].Minor != "8.5" || h.PHP[0].Full != "8.5.11" || h.PHP[1].Full != "8.4.25" {
		t.Errorf("php = %+v", h.PHP)
	}
	if len(h.Services) != 0 {
		t.Errorf("services = %v", h.Services)
	}
}

func TestReadHerdLinkSymlink(t *testing.T) {
	root := herdFixture(t)
	target := t.TempDir()
	link := filepath.Join(root, "config", "valet", "Sites", "api")
	if err := os.Symlink(target, link); err != nil {
		// Unprivileged Windows: Herd creates junctions, which need no privilege.
		out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if jerr != nil {
			t.Skipf("symlinks/junctions unavailable: %v; %v %s", err, jerr, out)
		}
	}
	h, _ := ReadHerd(root)
	for _, l := range h.Links {
		if l.Name == "api" {
			if !strings.EqualFold(filepath.Clean(l.Path), filepath.Clean(target)) {
				t.Errorf("api -> %s, want %s", l.Path, target)
			}
			return
		}
	}
	t.Errorf("symlinked site not found: %+v", h.Links)
}

func TestSourceSitesHerd(t *testing.T) {
	root := herdFixture(t)
	list, parked, notes, err := SourceSites(api.EnvHerd, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(parked) != 1 || parked[0] != filepath.Join(root, "Herd") {
		t.Errorf("parked = %v", parked)
	}
	by := map[string]api.ImportSite{}
	for _, s := range list {
		by[s.Name] = s
	}
	if len(list) != 3 {
		t.Fatalf("sites = %+v", list)
	}
	be := by["backend"]
	if be.Source != "link" || be.Domain != "backend.test" || be.PHP != "8.3" || !be.Secure ||
		be.DocRoot != filepath.Join(be.Path, "public") {
		t.Errorf("backend = %+v", be)
	}
	blog := by["blog"]
	if blog.Source != "parked" || blog.PHP != "8.4" || blog.Secure || blog.Domain != "blog.test" {
		t.Errorf("blog = %+v", blog)
	}
	if la := by["legacy-app"]; la.Domain != "legacy-app.test" {
		t.Errorf("legacy = %+v", la)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "Missing") || !strings.Contains(joined, "8.5.11, 8.4.25") {
		t.Errorf("notes = %v", notes)
	}
}

func TestDescribeFixture(t *testing.T) {
	e := describe(api.EnvXAMPP, fixture(t, "xampp"), []Proc{
		{PID: 5, Name: "mysqld.exe", Exe: filepath.Join(fixture(t, "xampp"), "mysql", "bin", "mysqld.exe"), Ports: []int{3307}},
		{PID: 6, Name: "xampp-control.exe", Exe: filepath.Join(fixture(t, "xampp"), "xampp-control.exe")},
	})
	if e.Name != "XAMPP 8.2.12" || !e.Running || len(e.Ports) != 1 || e.Ports[0] != 3307 || !e.Databases || e.Sites != 3 || !e.CanStop {
		t.Errorf("%+v", e)
	}
}
