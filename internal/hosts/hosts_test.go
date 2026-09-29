package hosts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const userHosts = "# Copyright (c) Microsoft\r\n127.0.0.1 localhost\r\n10.0.0.5 nas.lan # my nas\r\n"

func TestRenderAppendAndIdempotent(t *testing.T) {
	out := render(userHosts, []string{"blog.test", "Shop.test", "blog.test"}, "\r\n")
	want := "# Copyright (c) Microsoft\r\n127.0.0.1 localhost\r\n10.0.0.5 nas.lan # my nas\r\n\r\n" +
		BeginMarker + "\r\n# Managed by AMPLS. Do not edit this block; changes will be overwritten.\r\n" +
		"127.0.0.1 blog.test\r\n::1 blog.test\r\n127.0.0.1 shop.test\r\n::1 shop.test\r\n" + EndMarker + "\r\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
	if again := render(out, []string{"shop.test", "blog.test"}, "\r\n"); again != out {
		t.Fatalf("not idempotent:\n%q", again)
	}
	// replace
	out2 := render(out, []string{"new.test"}, "\r\n")
	if strings.Contains(out2, "blog.test") || !strings.Contains(out2, "::1 new.test\r\n") || !strings.HasPrefix(out2, userHosts) {
		t.Fatalf("replace failed: %q", out2)
	}
	if strings.Count(out2, BeginMarker) != 1 {
		t.Fatal("duplicate block")
	}
	// remove
	if got := render(out2, nil, "\r\n"); got != userHosts {
		t.Fatalf("remove failed: %q", got)
	}
	if !equalSets(parseBlock(out), []string{"blog.test", "shop.test"}) {
		t.Fatalf("parseBlock: %v", parseBlock(out))
	}
}

func TestRenderEdgeCases(t *testing.T) {
	// block in the middle of the file with user lines after it are preserved
	mid := "a\n" + BeginMarker + "\n127.0.0.1 old.test\n" + EndMarker + "\nb\n"
	got := render(mid, []string{"x.test"}, "\n")
	if !strings.HasPrefix(got, "a\nb\n\n"+BeginMarker) || strings.Contains(got, "old.test") {
		t.Fatalf("mid: %q", got)
	}
	// dangling begin marker (no end) does not swallow user lines
	dang := "a\n" + BeginMarker + "\nkeep me\n"
	got = render(dang, nil, "\n")
	if got != "a\nkeep me\n" {
		t.Fatalf("dangling: %q", got)
	}
	// empty file, LF
	if got := render("", []string{"a.test"}, "\n"); !strings.HasPrefix(got, BeginMarker+"\n") {
		t.Fatalf("empty: %q", got)
	}
	if got := render("", nil, "\n"); got != "" {
		t.Fatalf("empty/nil: %q", got)
	}
	// BOM stripped, no trailing newline in input
	if got := render("\xef\xbb\xbfx", nil, "\n"); got != "x\n" {
		t.Fatalf("bom: %q", got)
	}
	// Render uses platform line endings
	r := Render("", []string{"a.test"})
	if runtime.GOOS == "windows" && !strings.Contains(r, "\r\n") {
		t.Fatal("expected CRLF on windows")
	}
}

func TestValidate(t *testing.T) {
	good := [][]string{nil, {"blog.test"}, {"a.blog.test", "x-1.test"}, {"1.test"}}
	for _, g := range good {
		if err := Validate(g, "test"); err != nil {
			t.Errorf("Validate(%v): %v", g, err)
		}
	}
	bad := []string{
		"test", ".test", "blog.test.", "blog.com", "a.b.c.test", "*.blog.test", "Blog.test",
		"-a.test", "a-.test", "a_b.test", "a b.test", "blog.test\n127.0.0.1 evil.com",
		"blog.test evil.com", "..test", strings.Repeat("a", 64) + ".test", "",
	}
	for _, b := range bad {
		if err := Validate([]string{b}, "test"); err == nil {
			t.Errorf("Validate(%q) accepted", b)
		}
	}
	for _, tld := range []string{"", "a.b", "TEST", "1ab", "te st", "-x"} {
		if err := Validate([]string{"a." + tld}, tld); err == nil {
			t.Errorf("tld %q accepted", tld)
		}
	}
	many := make([]string, MaxDomains+1)
	for i := range many {
		many[i] = "a.test"
	}
	if Validate(many, "test") == nil {
		t.Error("too many accepted")
	}
	if validateAllowed([]string{"x.test", "y.localhost"}) != nil || validateAllowed([]string{"evil"}) == nil {
		t.Error("validateAllowed")
	}
	// public / non-allow-listed TLDs are never accepted, whatever tld is claimed
	for _, d := range []string{"windowsupdate.com", "update.microsoft.com", "a.local", "a.dev", "x.internal"} {
		if validateAllowed([]string{d}) == nil {
			t.Errorf("validateAllowed(%q) accepted", d)
		}
	}
}

func setup(t *testing.T) (home, hostsFile string) {
	home = t.TempDir()
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r // macOS: /var -> /private/var
	}
	hostsFile = filepath.Join(home, "hosts")
	os.WriteFile(hostsFile, []byte(userHosts), 0o644)
	hostsPathOverride = hostsFile
	t.Cleanup(func() { hostsPathOverride = "" })
	os.MkdirAll(filepath.Join(home, "run"), 0o755)
	return
}

func writeReq(t *testing.T, home string, r PendingRequest) {
	b, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(home, "run", requestFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyAndCurrent(t *testing.T) {
	setup(t)
	if err := Apply([]string{"b.test", "a.test"}); err != nil {
		t.Fatal(err)
	}
	cur, err := Current()
	if err != nil || !equalSets(cur, []string{"a.test", "b.test"}) {
		t.Fatalf("Current = %v %v", cur, err)
	}
	if Apply([]string{"evil"}) == nil {
		t.Fatal("Apply accepted invalid domain")
	}
}

func TestApplyPending(t *testing.T) {
	home, hostsFile := setup(t)
	run := filepath.Join(home, "run")
	now := time.Now().UTC()
	writeReq(t, home, PendingRequest{Domains: []string{"blog.test"}, TLD: "test", RequestedAt: now})
	if err := applyPending(run); err != nil {
		t.Fatal(err)
	}
	a, err := readApplied(run)
	if err != nil || a.Error != "" || !a.RequestedAt.Equal(now) {
		t.Fatalf("applied = %+v %v", a, err)
	}
	b, _ := os.ReadFile(hostsFile)
	if !strings.Contains(string(b), "127.0.0.1 blog.test") {
		t.Fatalf("hosts: %q", b)
	}
	// public tld rejected (even when the request claims it), hosts untouched, error recorded
	writeReq(t, home, PendingRequest{Domains: []string{"google.com"}, TLD: "com", RequestedAt: now.Add(time.Second)})
	if applyPending(run) == nil {
		t.Fatal("accepted public tld")
	}
	a, _ = readApplied(run)
	if a.Error == "" {
		t.Fatal("error not recorded")
	}
	b2, _ := os.ReadFile(hostsFile)
	if string(b2) != string(b) {
		t.Fatal("hosts modified on rejected request")
	}
	// injection attempt
	writeReq(t, home, PendingRequest{Domains: []string{"a.test\r\n1.2.3.4 bank.com"}, TLD: "test", RequestedAt: now.Add(2 * time.Second)})
	if applyPending(run) == nil {
		t.Fatal("accepted injection")
	}
	// request claims tld "test" but smuggles a public name
	writeReq(t, home, PendingRequest{Domains: []string{"a.test", "windowsupdate.com"}, TLD: "test", RequestedAt: now.Add(3 * time.Second)})
	if applyPending(run) == nil {
		t.Fatal("accepted public domain in a .test request")
	}
	b3, _ := os.ReadFile(hostsFile)
	if string(b3) != string(b) {
		t.Fatal("hosts modified on rejected request")
	}
	// oversize request file refused
	big := make([]byte, maxReqBytes+10)
	for i := range big {
		big[i] = ' '
	}
	os.WriteFile(filepath.Join(run, requestFile), big, 0o644)
	if applyPending(run) == nil {
		t.Fatal("accepted oversize request")
	}
}

func TestRequestRejectsUnsupportedTLD(t *testing.T) {
	setup(t)
	if err := Request([]string{"google.com"}, "com"); err == nil {
		t.Fatal("Request accepted tld com")
	}
}

func TestLockDirRejectsLinkedRunDir(t *testing.T) {
	home, _ := setup(t)
	if release, err := lockDir(filepath.Join(home, "run")); err != nil {
		t.Fatalf("lockDir(real dir): %v", err)
	} else {
		release()
	}
	target := t.TempDir()
	link := filepath.Join(home, "linked")
	if err := makeDirLink(target, link); err != nil {
		t.Skipf("cannot create directory link: %v", err)
	}
	if release, err := lockDir(link); err == nil {
		release()
		t.Fatal("lockDir accepted a junction/symlink")
	}
	// a real dir below a linked ancestor is refused as well
	os.MkdirAll(filepath.Join(target, "run"), 0o755)
	if release, err := lockDir(filepath.Join(link, "run")); err == nil {
		release()
		t.Fatal("lockDir accepted a path through a junction")
	}
}

func TestHelperIgnoresJunctionedRunDir(t *testing.T) {
	home, hostsFile := setup(t)
	os.RemoveAll(filepath.Join(home, "run"))
	elsewhere := t.TempDir()
	if err := makeDirLink(elsewhere, filepath.Join(home, "run")); err != nil {
		t.Skipf("cannot create directory link: %v", err)
	}
	b, _ := json.Marshal(PendingRequest{Domains: []string{"x.test"}, TLD: "test", RequestedAt: time.Now().UTC()})
	os.WriteFile(filepath.Join(elsewhere, requestFile), b, 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	RunHelperContext(ctx, home)
	if _, err := os.Stat(filepath.Join(elsewhere, appliedFile)); err == nil {
		t.Fatal("helper wrote through a junction")
	}
	h, _ := os.ReadFile(hostsFile)
	if string(h) != userHosts {
		t.Fatal("hosts modified via junctioned run dir")
	}
}

func TestRunHelper(t *testing.T) {
	home, hostsFile := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunHelperContext(ctx, home) }()
	time.Sleep(100 * time.Millisecond)
	req := PendingRequest{Domains: []string{"x.test", "api.x.test"}, TLD: "test", RequestedAt: time.Now().UTC()}
	b, _ := json.Marshal(req)
	if err := writeFileAtomic(filepath.Join(home, "run", requestFile), b); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		a, err := readApplied(filepath.Join(home, "run"))
		if err == nil && !a.RequestedAt.Before(req.RequestedAt) {
			if a.Error != "" {
				t.Fatal(a.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not respond")
		}
		time.Sleep(50 * time.Millisecond)
	}
	h, _ := os.ReadFile(hostsFile)
	if !strings.Contains(string(h), "::1 api.x.test") {
		t.Fatalf("hosts: %q", h)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("helper did not stop")
	}
}

func TestHelperRejectsPublicTLD(t *testing.T) {
	home, hostsFile := setup(t)
	// config.json is user-writable: a tld set there must not widen what the helper writes
	os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"tld":"com"}`), 0o644)
	writeReq(t, home, PendingRequest{Domains: []string{"google.com"}, TLD: "com", RequestedAt: time.Now().UTC()})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	RunHelperContext(ctx, home)
	a, err := readApplied(filepath.Join(home, "run"))
	if err != nil || a.Error == "" {
		t.Fatalf("expected rejection, got %+v %v", a, err)
	}
	h, _ := os.ReadFile(hostsFile)
	if string(h) != userHosts {
		t.Fatal("hosts modified")
	}
}
