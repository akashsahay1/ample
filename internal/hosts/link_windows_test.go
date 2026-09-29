//go:build windows

package hosts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeDirLink creates a directory junction (no privileges needed).
func makeDirLink(target, link string) error {
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}

func TestLockDirPinsDirectory(t *testing.T) {
	home, _ := setup(t)
	run := filepath.Join(home, "run")
	release, err := lockDir(run)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// the unprivileged client can still write requests while it is held...
	if err := writeFileAtomic(filepath.Join(run, requestFile), []byte("{}")); err != nil {
		t.Fatalf("client write blocked: %v", err)
	}
	// ...but the run dir (and its parent) cannot be swapped out for a junction.
	if err := os.Rename(run, run+".moved"); err == nil {
		t.Fatal("run dir renamed while locked")
	}
	if err := os.Rename(home, home+".moved"); err == nil {
		os.Rename(home+".moved", home)
		t.Fatal("home renamed while run dir locked")
	}
}
