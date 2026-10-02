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
	return exec.Command("/usr/bin/security", "verify-cert", "-c", CAPath(), "-p", "ssl").Run() == nil
}

// TrustCA adds the CA as a trusted root to the System keychain (machine, needs
// root) or the login keychain (user; macOS asks for confirmation).
func TrustCA(machine bool) error {
	if err := EnsureCA(); err != nil {
		return err
	}
	// Trust a private copy of the verified certificate, not the file in the
	// (shared) data directory, which could be swapped before `security` reads it.
	c, _, err := LoadCA()
	if err != nil {
		return fmt.Errorf("certs: trust: %w", err)
	}
	if !constrained(c) {
		return fmt.Errorf("certs: trust: refusing to trust a CA without the Apnoro name constraints")
	}
	tmpDir, err := os.MkdirTemp("", "apnoro-ca-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	caFile := filepath.Join(tmpDir, "ca.crt")
	if err := writePEM(caFile, "CERTIFICATE", c.Raw); err != nil {
		return err
	}
	var args []string
	if machine {
		args = []string{"add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", caFile}
	} else {
		h, _ := os.UserHomeDir()
		args = []string{"add-trusted-cert", "-r", "trustRoot", "-k", filepath.Join(h, "Library", "Keychains", "login.keychain-db"), caFile}
	}
	out, err := exec.Command("/usr/bin/security", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certs: security %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
