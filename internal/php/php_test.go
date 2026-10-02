package php

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apnoro/internal/paths"
)

//go:embed testdata/releases.json
var sampleReleases []byte

//go:embed testdata/php.ini-development
var sampleIni string

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.10", "8.9", 1}, {"8.3", "8.3.0", 0}, {"7.4.33", "8.0.0", -1},
		{"8.5.11", "8.5.2", 1}, {"8.5.0RC1", "8.5", 0}, {"8.2", "8.2", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
	if MinorOf("8.3.12") != "8.3" || !IsEOL("8.1") || !IsEOL("7.4") || IsEOL("8.2") || IsEOL("8.10") {
		t.Fatal("MinorOf/IsEOL")
	}
	if versionFromZipName("php-8.5.11-nts-Win32-vs17-x64.zip") != "8.5.11" {
		t.Fatal("versionFromZipName")
	}
}

func TestParseReleases(t *testing.T) {
	rels, err := parseReleases(sampleReleases)
	if err != nil {
		t.Fatal(err)
	}
	var minors []string
	for _, r := range rels {
		minors = append(minors, r.Minor)
	}
	if strings.Join(minors, ",") != "8.10,8.9,8.5,7.4" {
		t.Fatalf("order: %v", minors)
	}
	r := rels[2]
	if r.Full != "8.5.11" || r.URL != ReleasesBase+"php-8.5.11-nts-Win32-vs17-x64.zip" || r.SHA256 != "ff" || r.Size < 34<<20 {
		t.Fatalf("8.5: %+v", r)
	}
	if rels[3].SHA256 != "14ae3250d4447c8ccfc4c45a70d90adfbcd61e728d85f0be56a7ddf8f9c8aace" {
		t.Fatal("sha lowercased")
	}
	merged := mergeArchived(rels)
	var got []string
	for _, r := range merged {
		got = append(got, r.Minor)
	}
	if strings.Join(got, ",") != "8.10,8.9,8.5,8.1,8.0,7.4" {
		t.Fatalf("merged: %v", got)
	}
	if parseSize("900KB") != 900*1024 || parseSize("1.5MB") != 3<<19 {
		t.Fatal("parseSize")
	}
	if archiveURL(ReleasesBase+"a.zip") != ArchivesBase+"a.zip" || archiveURL(ArchivesBase+"a.zip") != "" {
		t.Fatal("archiveURL")
	}
}

func TestEditIni(t *testing.T) {
	out := EditIni(sampleIni, map[string]string{
		"memory_limit":  "512M",
		"extension_dir": `"C:/Apnoro/php/8.5/ext"`,
		"date.timezone": "UTC",
		"brand.new":     "1",
	}, map[string]bool{"curl": true, "mbstring": true, "opcache": true, "bz2": false})
	vals, exts := ParseIni(out)
	if vals["memory_limit"] != "512M" || vals["extension_dir"] != "C:/Apnoro/php/8.5/ext" || vals["date.timezone"] != "UTC" || vals["brand.new"] != "1" {
		t.Fatalf("values: %v", vals)
	}
	if !exts["curl"] || !exts["mbstring"] || !exts["opcache"] || exts["bz2"] || exts["gd"] {
		t.Fatalf("exts: %v", exts)
	}
	if !strings.Contains(out, "zend_extension=opcache") {
		t.Fatal("opcache must be zend_extension")
	}
	if strings.Count(out, "\nextension=curl") != 1 {
		t.Fatal("curl enabled once")
	}
	if !strings.Contains(out, "\r\n") && strings.Contains(sampleIni, "\r\n") {
		t.Fatal("line endings")
	}
	// disabling toggles back to commented, other lines untouched
	out2 := EditIni(out, nil, map[string]bool{"curl": false})
	_, exts2 := ParseIni(out2)
	if exts2["curl"] || !exts2["mbstring"] {
		t.Fatalf("disable: %v", exts2)
	}
	if len(strings.Split(out2, "\n")) != len(strings.Split(out, "\n")) {
		t.Fatal("line count changed on toggle")
	}
}

func TestEditIniDllStyle(t *testing.T) {
	in := "[PHP]\nextension=php_gd2.dll\n;extension=php_xdebug.dll\nzend_extension=\"C:/php/ext/php_xdebug.dll\"\n"
	out := EditIni(in, nil, map[string]bool{"gd2": false, "xdebug": true})
	_, exts := ParseIni(out)
	if exts["gd2"] || !exts["xdebug"] {
		t.Fatalf("%v\n%s", exts, out)
	}
	if strings.Count(out, "\nzend_extension=") != 1 {
		t.Fatalf("xdebug should be enabled once:\n%s", out)
	}
}

func TestEnsureIniAndSettings(t *testing.T) {
	home := t.TempDir()
	paths.SetHome(home)
	dir := paths.PHPDir("8.5")
	os.MkdirAll(filepath.Join(dir, "ext"), 0o755)
	for _, n := range []string{"php_curl.dll", "php_gd.dll", "php_mbstring.dll", "php_xsl.dll", "php_zend_test.dll"} {
		os.WriteFile(filepath.Join(dir, "ext", n), nil, 0o644)
	}
	os.WriteFile(filepath.Join(dir, "php.ini-development"), []byte(sampleIni), 0o644)
	os.WriteFile(filepath.Join(paths.PHPRoot(), "cacert.pem"), []byte("x"), 0o644)
	if err := EnsureIni("8.5"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(IniPath("8.5"))
	vals, exts := ParseIni(string(b))
	if !exts["curl"] || !exts["gd"] || !exts["mbstring"] || exts["xsl"] || exts["intl"] || exts["opcache"] {
		t.Fatalf("exts %v", exts)
	}
	if !strings.HasSuffix(vals["extension_dir"], "/php/8.5/ext") || !strings.HasSuffix(vals["curl.cainfo"], "/php/cacert.pem") ||
		!strings.HasSuffix(vals["error_log"], "/logs/php-8.5-error.log") {
		t.Fatalf("vals %v", vals)
	}
	s, err := ReadSettings("8.5")
	if err != nil {
		t.Fatal(err)
	}
	if s.Values["memory_limit"] != "512M" || len(s.Extensions) != 4 {
		t.Fatalf("settings %+v", s)
	}
	s.Values["memory_limit"] = "1G"
	s.Extensions = []Ext{{"xsl", true}, {"curl", false}}
	if err := WriteSettings("8.5", s); err != nil {
		t.Fatal(err)
	}
	s2, _ := ReadSettings("8.5")
	m := map[string]bool{}
	for _, e := range s2.Extensions {
		m[e.Name] = e.Enabled
	}
	if s2.Values["memory_limit"] != "1G" || !m["xsl"] || m["curl"] || !m["gd"] {
		t.Fatalf("after write %+v", s2)
	}
	if err := WriteSettings("8.5", Settings{Extensions: []Ext{{"nope", true}}}); err == nil {
		t.Fatal("unknown ext should fail")
	}
}
