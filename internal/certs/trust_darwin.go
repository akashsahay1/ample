//go:build darwin

package certs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsCATrusted asks the macOS trust settings whether the CA verifies.
func IsCATrusted() bool {
	if _, err := os.Stat(CAPath()); err != nil {
		return false
	}
	return exec.Command("security", "verify-cert", "-c", CAPath(), "-p", "ssl").Run() == nil
}

// TrustCA adds the CA as a trusted root to the System keychain (machine, needs
// root) or the login keychain (user; macOS asks for confirmation).
func TrustCA(machine bool) error {
	if err := EnsureCA(); err != nil {
		return err
	}
	var args []string
	if machine {
		args = []string{"add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", CAPath()}
	} else {
		h, _ := os.UserHomeDir()
		args = []string{"add-trusted-cert", "-r", "trustRoot", "-k", filepath.Join(h, "Library", "Keychains", "login.keychain-db"), CAPath()}
	}
	out, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certs: security %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
