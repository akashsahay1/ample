package apache

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// hostRe matches a lowercase DNS host name, optionally a "*." wildcard.
var hostRe = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// safeConfPath reports whether p can be rendered with q() without breaking out
// of the quoted Apache argument or injecting directives (a folder name on
// macOS may contain quotes, backslashes or newlines).
func safeConfPath(p string) bool {
	if p == "" || strings.Contains(p, `"`) || strings.Contains(p, "${") {
		return false
	}
	// q() only escapes quotes; a trailing backslash would escape the closing
	// quote (backslashes are separators on Windows and are converted to /).
	if runtime.GOOS != "windows" && strings.HasSuffix(p, `\`) {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			return false
		}
	}
	return true
}

// checkConfigInputs rejects values that would be rendered unsafely into the
// Apache config (defence in depth; sites.Discover already skips such folders).
func checkConfigInputs(opts Options, home string, vhosts []VHost) error {
	if !safeConfPath(home) {
		return fmt.Errorf("apache: write config: unsupported characters in data directory %q", home)
	}
	if opts.DefaultPHPCGI != "" && !safeConfPath(opts.DefaultPHPCGI) {
		return fmt.Errorf("apache: write config: unsupported characters in path %q", opts.DefaultPHPCGI)
	}
	for _, v := range vhosts {
		if len(v.Domain) > 253 || !hostRe.MatchString(v.Domain) {
			return fmt.Errorf("apache: write config: invalid domain %q", v.Domain)
		}
		for _, a := range v.Aliases {
			if len(a) > 253 || !hostRe.MatchString(a) {
				return fmt.Errorf("apache: write config: invalid alias %q for %s", a, v.Domain)
			}
		}
		for _, p := range []string{v.DocRoot, v.PHPCGI} {
			if !safeConfPath(p) {
				return fmt.Errorf("apache: write config: unsupported characters in path %q for %s", p, v.Domain)
			}
		}
		if v.Secure {
			for _, p := range []string{v.CertFile, v.KeyFile} {
				if p != "" && !safeConfPath(p) {
					return fmt.Errorf("apache: write config: unsupported characters in path %q for %s", p, v.Domain)
				}
			}
		}
	}
	return nil
}
