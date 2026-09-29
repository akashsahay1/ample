//go:build !windows

package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Hide is a no-op outside Windows.
func Hide(cmd *exec.Cmd) {}

func startDetached(exe string, args []string, log *os.File) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Reap in the background so the child never lingers as a zombie while we run.
	go cmd.Wait()
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && err != syscall.EPERM {
		return false
	}
	// Treat zombies as dead where /proc exists.
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		f := strings.Fields(string(b))
		if len(f) > 2 && f[2] == "Z" {
			return false
		}
	}
	return true
}

func processMatches(pid int, exe string) bool {
	if !processAlive(pid) {
		return false
	}
	if exe == "" {
		return true
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return true // cannot verify; assume ours
	}
	comm := strings.TrimSpace(string(out))
	return comm == "" || filepath.Base(comm) == filepath.Base(exe) || strings.HasSuffix(exe, comm)
}

func hasChildren(pid int) bool {
	return syscall.Kill(-pid, 0) == nil
}

func killTree(pid int) error {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && processAlive(pid) {
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if processAlive(pid) {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func portOwner(port int) string {
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fc").Output()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "c") {
			return l[1:]
		}
	}
	return ""
}
