package sites

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ampls/internal/config"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"My Blog":               "my-blog",
		"blog":                  "blog",
		"Blog_v2":               "blog-v2",
		"  spaced  ":            "spaced",
		"a--b":                  "a-b",
		"--x--":                 "x",
		"Ünïcode":               "n-code",
		"...":                   "",
		"123":                   "123",
		"Foo.Bar":               "foo-bar",
		strings.Repeat("a", 70): strings.Repeat("a", 63),
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidName(t *testing.T) {
	ok := []string{"a", "blog", "my-blog", "a1", "1a", "x-y-z"}
	bad := []string{"", "-a", "a-", "A", "a_b", "a.b", "a b", strings.Repeat("a", 64)}
	for _, s := range ok {
		if !ValidName(s) {
			t.Errorf("ValidName(%q) = false", s)
		}
	}
	for _, s := range bad {
		if ValidName(s) {
			t.Errorf("ValidName(%q) = true", s)
		}
	}
}

func mk(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func touch(t *testing.T, parts ...string) {
	t.Helper()
	p := filepath.Join(parts...)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDocRootAndFramework(t *testing.T) {
	root := t.TempDir()
	lara := mk(t, root, "lara")
	mk(t, lara, "public")
	touch(t, lara, "artisan")
	wp := mk(t, root, "wp")
	touch(t, wp, "wp-load.php")
	wp2 := mk(t, root, "wp2")
	touch(t, wp2, "wp-config.php")
	craft := mk(t, root, "craft")
	mk(t, craft, "web")
	plain := mk(t, root, "plain")

	tests := []struct {
		path, doc, fw string
	}{
		{lara, filepath.Join(lara, "public"), "laravel"},
		{wp, wp, "wordpress"},
		{wp2, wp2, "wordpress"},
		{craft, filepath.Join(craft, "web"), "php"},
		{plain, plain, "php"},
	}
	for _, tc := range tests {
		if got := DocRoot(tc.path); got != tc.doc {
			t.Errorf("DocRoot(%s) = %s, want %s", tc.path, got, tc.doc)
		}
		if got := DetectFramework(tc.path); got != tc.fw {
			t.Errorf("DetectFramework(%s) = %s, want %s", tc.path, got, tc.fw)
		}
	}
	// "artisan" as directory must not count
	odd := mk(t, root, "odd")
	mk(t, odd, "artisan")
	if DetectFramework(odd) != "php" {
		t.Error("artisan dir detected as laravel")
	}
}

func testCfg(t *testing.T) (*config.Config, string, string) {
	parked := t.TempDir()
	parked2 := t.TempDir()
	linkDir := t.TempDir()
	mk(t, parked, "My Blog")
	mk(t, parked, "shop", "public")
	touch(t, parked, "shop", "artisan")
	mk(t, parked, ".hidden")
	mk(t, parked, "___") // slugs to "" -> skipped
	touch(t, parked, "file.txt")
	mk(t, parked, "api")
	mk(t, parked2, "api")   // duplicate: first parked wins
	mk(t, parked2, "other") // unique
	mk(t, linkDir, "sub", "deeper")

	cfg := config.Default()
	cfg.DefaultPHP = "8.4"
	cfg.Parked = []string{parked, parked2, filepath.Join(parked, "missing")}
	cfg.Links = []config.Link{{Name: "api", Path: linkDir}, {Name: "Bad Name!", Path: linkDir}}
	cfg.Sites = map[string]config.SiteSettings{
		"my-blog": {PHP: "8.2", Secure: true},
		"other":   {DocRoot: "dist"},
	}
	return cfg, parked, linkDir
}

func TestDiscover(t *testing.T) {
	cfg, parked, linkDir := testCfg(t)
	all, err := Discover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range all {
		names = append(names, s.Name)
	}
	want := "api,bad-name,my-blog,other,shop"
	if strings.Join(names, ",") != want {
		t.Fatalf("names = %v, want %s", names, want)
	}
	s, ok := Find(cfg, "api")
	if !ok || !s.Linked || s.Path != linkDir {
		t.Errorf("api should be the link: %+v", s)
	}
	s, _ = Find(cfg, "my-blog")
	if s.Domain != "my-blog.test" || s.URL != "https://my-blog.test" || s.PHP != "8.2" || !s.Isolated || !s.Secure || s.Linked {
		t.Errorf("my-blog wrong: %+v", s)
	}
	if s.Path != filepath.Join(parked, "My Blog") {
		t.Errorf("my-blog path %s", s.Path)
	}
	s, _ = Find(cfg, "shop")
	if s.Framework != "laravel" || s.DocRoot != filepath.Join(parked, "shop", "public") || s.PHP != "8.4" || s.Isolated || s.URL != "http://shop.test" {
		t.Errorf("shop wrong: %+v", s)
	}
	s, _ = Find(cfg, "other")
	if !strings.HasSuffix(s.DocRoot, filepath.Join("other", "dist")) {
		t.Errorf("other docroot override: %s", s.DocRoot)
	}
	if _, ok := Find(cfg, "nope"); ok {
		t.Error("found nonexistent site")
	}

	cfg.TLD = "localhost"
	s, _ = Find(cfg, "shop")
	if s.Domain != "shop.localhost" {
		t.Errorf("tld not applied: %s", s.Domain)
	}
}

func TestFindByPath(t *testing.T) {
	cfg, parked, linkDir := testCfg(t)
	tests := []struct {
		dir, want string
		ok        bool
	}{
		{filepath.Join(parked, "shop"), "shop", true},
		{filepath.Join(parked, "shop", "public", "css"), "shop", true},
		{filepath.Join(linkDir, "sub", "deeper"), "api", true},
		{parked, "", false},
		{filepath.Join(parked, "shopping"), "", false},
		{filepath.Join(parked, "My Blog", "x"), "my-blog", true},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			dir, want string
			ok        bool
		}{strings.ToUpper(filepath.Join(parked, "shop", "app")), "shop", true})
	}
	for _, tc := range tests {
		s, ok := FindByPath(cfg, tc.dir)
		if ok != tc.ok || s.Name != tc.want {
			t.Errorf("FindByPath(%s) = %q,%v want %q,%v", tc.dir, s.Name, ok, tc.want, tc.ok)
		}
	}
}

func TestNestedSiteDeepestWins(t *testing.T) {
	parked := t.TempDir()
	outer := mk(t, parked, "outer")
	inner := mk(t, outer, "inner")
	cfg := config.Default()
	cfg.Parked = []string{parked}
	cfg.Links = []config.Link{{Name: "inner", Path: inner}}
	s, ok := FindByPath(cfg, filepath.Join(inner, "x"))
	if !ok || s.Name != "inner" {
		t.Errorf("got %q", s.Name)
	}
}
