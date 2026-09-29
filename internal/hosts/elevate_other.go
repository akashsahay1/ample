//go:build !windows

package hosts

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"ampls/internal/paths"
)

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// elevateApply runs `ampls hosts apply` with administrator privileges via
// osascript (macOS) or sudo -n elsewhere.
func elevateApply() error {
	exe := filepath.Join(paths.BinDir(), "ampls")
	shell := shQuote(exe) + " hosts apply --home " + shQuote(paths.Home())
	var cmd *exec.Cmd
	if _, err := exec.LookPath("osascript"); err == nil {
		script := `do shell script "` + strings.ReplaceAll(strings.ReplaceAll(shell, `\`, `\\`), `"`, `\"`) + `" with administrator privileges`
		cmd = exec.Command("osascript", "-e", script)
	} else {
		cmd = exec.Command("sudo", "-n", "sh", "-c", shell)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func flushDNS() {
	_ = exec.Command("dscacheutil", "-flushcache").Run()
}
