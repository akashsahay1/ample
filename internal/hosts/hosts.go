// Package hosts manages the AMPLS block in the system hosts file.
//
// Writing the hosts file needs administrator rights. On Windows the
// ampls-helper service (LocalSystem) watches <Home>/run/hosts.json and applies
// a strictly validated block (see RunHelper). When the helper is not running,
// Request falls back to a UAC-elevated `ampls.exe hosts apply`, which calls
// ApplyPending.
package hosts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"ampls/internal/paths"
)

const (
	BeginMarker = "# BEGIN AMPLS"
	EndMarker   = "# END AMPLS"
)

// MaxDomains caps the number of domains accepted in a single request.
const MaxDomains = 2000

const (
	requestFile = "hosts.json"
	appliedFile = "hosts.applied.json"
	maxReqBytes = 1 << 20
)

// PendingRequest is the content of run/hosts.json.
type PendingRequest struct {
	Domains     []string  `json:"domains"`
	TLD         string    `json:"tld"`
	RequestedAt time.Time `json:"requestedAt"`
}

// AppliedResult is the content of run/hosts.applied.json.
type AppliedResult struct {
	Domains     []string  `json:"domains"`
	RequestedAt time.Time `json:"requestedAt"`
	AppliedAt   time.Time `json:"appliedAt"`
	Error       string    `json:"error,omitempty"`
}

// hostsPathOverride redirects HostsPath in tests.
var hostsPathOverride string

func eol() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// HostsPath returns the system hosts file location.
func HostsPath() string {
	if hostsPathOverride != "" {
		return hostsPathOverride
	}
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

// normalize lowercases, dedupes and sorts domains.
func normalize(domains []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// Render replaces (or appends) the managed block in existing with entries for
// domains. Lines outside the block are preserved. With no domains the block is
// removed. The result uses CRLF line endings on Windows and LF elsewhere.
func Render(existing string, domains []string) string {
	return render(existing, domains, eol())
}

func render(existing string, domains []string, nl string) string {
	existing = strings.TrimPrefix(existing, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(existing, "\r\n", "\n"), "\n")
	var kept []string
	in := false
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], "\r")
		t := strings.TrimSpace(l)
		if !in && t == BeginMarker {
			// only treat it as a block if a matching end marker follows
			hasEnd := false
			for _, r := range lines[i+1:] {
				if strings.TrimSpace(r) == EndMarker {
					hasEnd = true
					break
				}
			}
			if hasEnd {
				in = true
			}
			continue // a dangling begin marker is dropped
		}
		if in {
			if t == EndMarker {
				in = false
			}
			continue
		}
		if t == EndMarker {
			continue // stray end marker
		}
		kept = append(kept, l)
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	var b strings.Builder
	for _, l := range kept {
		b.WriteString(l)
		b.WriteString(nl)
	}
	ds := normalize(domains)
	if len(ds) > 0 {
		if len(kept) > 0 {
			b.WriteString(nl)
		}
		b.WriteString(BeginMarker + nl)
		b.WriteString("# Managed by AMPLS. Do not edit this block; changes will be overwritten." + nl)
		for _, d := range ds {
			b.WriteString("127.0.0.1 " + d + nl)
			b.WriteString("::1 " + d + nl)
		}
		b.WriteString(EndMarker + nl)
	}
	return b.String()
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validLabel(s string) bool { return labelRe.MatchString(s) }

// ValidTLD reports whether tld is a single strict DNS label (e.g. "test").
func ValidTLD(tld string) bool {
	return validLabel(tld) && !strings.ContainsAny(tld[:1], "0123456789")
}

// Validate checks that each domain is <label>.<tld> or <sub>.<label>.<tld>
// with strict lowercase LDH labels, and that there are at most MaxDomains.
func Validate(domains []string, tld string) error {
	if !ValidTLD(tld) {
		return fmt.Errorf("hosts: invalid tld %q", tld)
	}
	if len(domains) > MaxDomains {
		return fmt.Errorf("hosts: too many domains (%d > %d)", len(domains), MaxDomains)
	}
	suffix := "." + tld
	for _, d := range domains {
		if len(d) > 253 || !strings.HasSuffix(d, suffix) {
			return fmt.Errorf("hosts: invalid domain %q", d)
		}
		labels := strings.Split(strings.TrimSuffix(d, suffix), ".")
		if len(labels) < 1 || len(labels) > 2 {
			return fmt.Errorf("hosts: invalid domain %q", d)
		}
		for _, l := range labels {
			if !validLabel(l) {
				return fmt.Errorf("hosts: invalid domain %q", d)
			}
		}
	}
	return nil
}

// validateAny validates domains whatever their TLD (used by Apply as a last line
// of defence so nothing but strict host names ever reaches the hosts file).
func validateAny(domains []string) error {
	if len(domains) > MaxDomains {
		return fmt.Errorf("hosts: too many domains (%d > %d)", len(domains), MaxDomains)
	}
	for _, d := range domains {
		i := strings.LastIndexByte(d, '.')
		if i < 0 {
			return fmt.Errorf("hosts: invalid domain %q", d)
		}
		if err := Validate([]string{d}, d[i+1:]); err != nil {
			return err
		}
	}
	return nil
}

// parseBlock returns the domains inside the managed block of content.
func parseBlock(content string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == BeginMarker:
			in = true
		case t == EndMarker:
			in = false
		case in && t != "" && !strings.HasPrefix(t, "#"):
			f := strings.Fields(t)
			for _, h := range f[1:] {
				if strings.HasPrefix(h, "#") {
					break
				}
				out = append(out, h)
			}
		}
	}
	return normalize(out)
}

// Current returns the domains inside the managed block of the hosts file.
func Current() ([]string, error) {
	b, err := os.ReadFile(HostsPath())
	if err != nil {
		return nil, fmt.Errorf("hosts: read: %w", err)
	}
	return parseBlock(string(b)), nil
}

// Apply writes the managed block directly (requires admin rights).
func Apply(domains []string) error {
	domains = normalize(domains)
	if err := validateAny(domains); err != nil {
		return err
	}
	p := HostsPath()
	old, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("hosts: read: %w", err)
	}
	out := Render(string(old), domains)
	if out == string(old) {
		return nil
	}
	// Write in place (keeps the file's ACLs); retry briefly because antivirus
	// and the DNS client sometimes hold the file open.
	var werr error
	for i := 0; i < 10; i++ {
		if werr = os.WriteFile(p, []byte(out), 0o644); werr == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("hosts: write %s: %w", p, werr)
}

func equalSets(a, b []string) bool {
	a, b = normalize(a), normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeFileAtomic writes via a fresh exclusive temp file + rename, so a
// pre-planted symlink at the destination is replaced rather than followed.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// readRegular reads a small regular file, refusing symlinks and oversize files.
func readRegular(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("hosts: %s is not a regular file", path)
	}
	if st.Size() > maxReqBytes {
		return nil, fmt.Errorf("hosts: %s too large", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxReqBytes))
}

func readPending(runDir string) (PendingRequest, error) {
	var r PendingRequest
	b, err := readRegular(filepath.Join(runDir, requestFile))
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("hosts: parse %s: %w", requestFile, err)
	}
	return r, nil
}

func readApplied(runDir string) (AppliedResult, error) {
	var r AppliedResult
	b, err := readRegular(filepath.Join(runDir, appliedFile))
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}

func writeApplied(runDir string, r AppliedResult) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(runDir, appliedFile), b)
}

// Request is the non-admin entry point. It is a no-op when the hosts file
// already contains exactly domains; otherwise it asks the helper service via
// run/hosts.json (waiting up to 5s) and falls back to an elevated
// `ampls.exe hosts apply`.
func Request(domains []string, tld string) error {
	domains = normalize(domains)
	if err := Validate(domains, tld); err != nil {
		return err
	}
	if cur, err := Current(); err == nil && equalSets(cur, domains) {
		return nil
	}
	runDir := paths.RunDir()
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("hosts: %w", err)
	}
	req := PendingRequest{Domains: domains, TLD: tld, RequestedAt: time.Now().UTC()}
	b, _ := json.MarshalIndent(req, "", "  ")
	if err := writeFileAtomic(filepath.Join(runDir, requestFile), b); err != nil {
		return fmt.Errorf("hosts: write request: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		a, err := readApplied(runDir)
		if err != nil || a.RequestedAt.Before(req.RequestedAt) {
			continue
		}
		if a.Error != "" {
			return fmt.Errorf("hosts: helper: %s", a.Error)
		}
		flushDNS()
		return nil
	}

	// Helper did not answer: elevate.
	if err := elevateApply(); err != nil {
		return fmt.Errorf("hosts: elevated apply: %w", err)
	}
	cur, err := Current()
	if err != nil {
		return err
	}
	if !equalSets(cur, domains) {
		return errors.New("hosts: hosts file was not updated")
	}
	flushDNS()
	return nil
}

// ApplyPending reads run/hosts.json, validates it and applies it, recording
// the outcome in run/hosts.applied.json. It is what `ampls hosts apply` (run
// elevated) calls.
func ApplyPending() error {
	return applyPending(paths.RunDir(), "")
}

// applyPending: when requiredTLD != "" the request's TLD must match it.
func applyPending(runDir, requiredTLD string) error {
	req, err := readPending(runDir)
	if err != nil {
		return err
	}
	res := AppliedResult{Domains: normalize(req.Domains), RequestedAt: req.RequestedAt}
	err = func() error {
		if requiredTLD != "" && req.TLD != requiredTLD {
			return fmt.Errorf("hosts: tld %q does not match configured tld %q", req.TLD, requiredTLD)
		}
		if err := Validate(res.Domains, req.TLD); err != nil {
			return err
		}
		return Apply(res.Domains)
	}()
	res.AppliedAt = time.Now().UTC()
	if err != nil {
		res.Error = err.Error()
	}
	if werr := writeApplied(runDir, res); werr != nil && err == nil {
		err = fmt.Errorf("hosts: write result: %w", werr)
	}
	if err == nil {
		flushDNS()
	}
	return err
}
