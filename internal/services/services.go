// Package services runs Apache and MySQL as detached background processes
// tracked by pid files in paths.RunDir().
package services

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"apnoro/internal/paths"
)

// State of a managed service. (CONTRACTS.md named this type Status, which
// collides with func Status; callers use services.Status(name).Running.)
type State struct {
	Running bool
	PID     int
}

func pidFile(name string) string { return filepath.Join(paths.RunDir(), name+".pid") }

// pid file format: "<pid>\n<exe path>\n"
func writePid(name string, pid int, exe string) error {
	if err := os.MkdirAll(paths.RunDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pidFile(name), []byte(fmt.Sprintf("%d\n%s\n", pid, exe)), 0o644)
}

func readPid(name string) (int, string) {
	b, err := os.ReadFile(pidFile(name))
	if err != nil {
		return 0, ""
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n")
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	exe := ""
	if len(lines) > 1 {
		exe = strings.TrimSpace(lines[1])
	}
	return pid, exe
}

// Status reports whether the service recorded in run/<name>.pid is alive and
// still the expected executable.
func Status(name string) State {
	pid, exe := readPid(name)
	if pid <= 0 {
		return State{}
	}
	if !processMatches(pid, exe) {
		return State{}
	}
	return State{Running: true, PID: pid}
}

// Start launches exe detached with stdout/stderr appended to logFile and
// records its pid. It returns nil immediately if the service already runs.
func Start(name string, exe string, args []string, logFile string) error {
	if st := Status(name); st.Running {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return fmt.Errorf("services: %s: %w", name, err)
	}
	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("services: %s: open log: %w", name, err)
	}
	fmt.Fprintf(lf, "\n[%s] Apnoro starting %s\n", time.Now().Format(time.RFC3339), name)
	pid, err := startDetached(exe, args, lf)
	lf.Close()
	if err != nil {
		return fmt.Errorf("services: start %s: %w", name, err)
	}
	if err := writePid(name, pid, exe); err != nil {
		return fmt.Errorf("services: %s: write pid: %w", name, err)
	}
	// Give the process a moment to fail fast (bad config, port in use...).
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if !processMatches(pid, exe) {
			os.Remove(pidFile(name))
			return fmt.Errorf("services: %s exited immediately: %s", name, tail(logFile, 8))
		}
	}
	return nil
}

// Stop runs graceful (if non-nil), waits up to 8s, then kills the whole process tree.
func Stop(name string, graceful func() error) error {
	pid, exe := readPid(name)
	if pid <= 0 || !processMatches(pid, exe) {
		os.Remove(pidFile(name))
		return nil
	}
	if graceful != nil {
		if err := graceful(); err == nil {
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) && processAlive(pid) {
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
	var err error
	if processAlive(pid) || hasChildren(pid) {
		err = killTree(pid)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && processAlive(pid) {
			time.Sleep(100 * time.Millisecond)
		}
		if processAlive(pid) {
			return fmt.Errorf("services: %s (pid %d) did not stop: %v", name, pid, err)
		}
	}
	os.Remove(pidFile(name))
	return nil
}

// WaitPort waits until something accepts TCP connections on 127.0.0.1:port.
func WaitPort(port int, timeout time.Duration) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("services: port %d not ready after %s", port, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// PortInUse reports whether port is taken on this machine, and the owning
// process name when detectable.
func PortInUse(port int) (bool, string) {
	inUse := false
	if l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err != nil {
		inUse = true
	} else {
		l.Close()
	}
	if !inUse {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond); err == nil {
			c.Close()
			inUse = true
		}
	}
	if !inUse {
		return false, ""
	}
	return true, portOwner(port)
}

func tail(file string, n int) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r", ""), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
