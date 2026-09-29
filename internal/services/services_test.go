package services

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ampls/internal/paths"
)

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "ampls-svc")
	paths.SetHome(dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestPidFileRoundTrip(t *testing.T) {
	if err := writePid("x", 1234, `C:\a\b.exe`); err != nil {
		t.Fatal(err)
	}
	pid, exe := readPid("x")
	if pid != 1234 || exe != `C:\a\b.exe` {
		t.Fatal(pid, exe)
	}
	os.Remove(pidFile("x"))
	if st := Status("x"); st.Running {
		t.Fatal("should not run")
	}
}

func TestStaleOwnPid(t *testing.T) {
	// Our own pid but a different exe must not count as running.
	writePid("y", os.Getpid(), filepath.Join(t.TempDir(), "nope.exe"))
	if Status("y").Running {
		t.Fatal("pid reuse detected as running")
	}
	exe, _ := os.Executable()
	writePid("y", os.Getpid(), exe)
	if !Status("y").Running {
		t.Fatal("own process should be running")
	}
	os.Remove(pidFile("y"))
}

func TestPortInUseAndWait(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if in, _ := PortInUse(port); !in {
		t.Fatal("expected in use")
	}
	if err := WaitPort(port, time.Second); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if in, _ := PortInUse(port); in {
		t.Fatal("expected free")
	}
}
