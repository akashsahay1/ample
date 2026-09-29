//go:build !windows

package external

import (
	"errors"
	"os"
	"syscall"
)

const isWindows = false

// ErrAccessDenied is returned by Stop when a process belongs to another user.
var ErrAccessDenied = errors.New("access denied")

// Processes is not implemented outside Windows yet (macOS Herd/MAMP support is planned).
func Processes() []Proc { return nil }

func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

func killPID(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Kill(); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return ErrAccessDenied
		}
		return err
	}
	return nil
}

func candidateRoots(kind string) []string { return nil }

func herdInstallCandidates() []string { return nil }
