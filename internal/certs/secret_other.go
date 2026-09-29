//go:build !windows

package certs

import "os"

// createExclusive creates a new file (O_EXCL: fails if the name exists, a
// symlink included); secret files are 0600.
func createExclusive(name string, secret bool) (*os.File, error) {
	perm := os.FileMode(0o644)
	if secret {
		perm = 0o600
	}
	return os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
}
