//go:build windows

package services

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow         = 0x08000000
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
	stillActive            = 259
)

// Hide configures cmd to run without a visible console window (for short-lived
// helper commands such as php.exe -r, httpd -t, mysql.exe).
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func startDetached(exe string, args []string, log *os.File) (int, error) {
	try := func(flags uint32) (int, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = filepath.Dir(exe)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		pid := cmd.Process.Pid
		cmd.Process.Release()
		return pid, nil
	}
	base := uint32(createNoWindow | createNewProcessGroup)
	// Break away from the parent's job (terminals/IDEs may kill the job on exit).
	if pid, err := try(base | createBreakawayFromJob); err == nil {
		return pid, nil
	}
	return try(base)
}

func openProc(pid int) (windows.Handle, error) {
	return windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
}

func processAlive(pid int) bool {
	h, err := openProc(pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

func imagePath(pid int) string {
	h, err := openProc(pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// processMatches: pid alive and its image is exe (full path, or base name as fallback).
func processMatches(pid int, exe string) bool {
	if !processAlive(pid) {
		return false
	}
	if exe == "" {
		return true
	}
	img := imagePath(pid)
	if img == "" {
		return false
	}
	if strings.EqualFold(filepath.Clean(img), filepath.Clean(exe)) {
		return true
	}
	a, _ := filepath.Abs(exe)
	if strings.EqualFold(filepath.Clean(img), a) {
		return true
	}
	// Different path spellings (8.3 names, symlinks): fall back to image name.
	return strings.EqualFold(filepath.Base(img), filepath.Base(exe)) && sameFile(img, exe)
}

func sameFile(a, b string) bool {
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// children maps parent pid -> child pids from a toolhelp snapshot.
func processTable() (map[uint32][]uint32, map[uint32]string) {
	kids := map[uint32][]uint32{}
	names := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return kids, names
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID != e.ParentProcessID {
			kids[e.ParentProcessID] = append(kids[e.ParentProcessID], e.ProcessID)
		}
		names[e.ProcessID] = windows.UTF16ToString(e.ExeFile[:])
	}
	return kids, names
}

func descendants(pid int) []uint32 {
	kids, _ := processTable()
	var out []uint32
	seen := map[uint32]bool{uint32(pid): true}
	queue := []uint32{uint32(pid)}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range kids[p] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
				queue = append(queue, c)
			}
		}
	}
	return out
}

func hasChildren(pid int) bool { return len(descendants(pid)) > 0 }

func terminate(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return err
	}
	windows.WaitForSingleObject(h, 3000)
	return nil
}

// killTree terminates pid first (so it cannot respawn workers), then every
// descendant captured before the kill.
func killTree(pid int) error {
	desc := descendants(pid)
	var firstErr error
	if processAlive(pid) {
		if err := terminate(uint32(pid)); err != nil {
			firstErr = err
		}
	}
	for _, c := range desc {
		if err := terminate(c); err != nil && firstErr == nil && processAlive(int(c)) {
			firstErr = err
		}
	}
	return firstErr
}

// portOwner finds the listening process via `netstat -ano`.
func portOwner(port int) string {
	cmd := exec.Command("netstat", "-ano", "-p", "TCP")
	Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	suffix := ":" + strconv.Itoa(port)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || !strings.EqualFold(f[0], "TCP") || !strings.HasSuffix(f[1], suffix) {
			continue
		}
		if !strings.EqualFold(f[3], "LISTENING") {
			continue
		}
		pid, err := strconv.Atoi(f[4])
		if err != nil {
			continue
		}
		return processName(pid)
	}
	return ""
}

func processName(pid int) string {
	if pid == 4 {
		return "System"
	}
	if img := imagePath(pid); img != "" {
		return filepath.Base(img)
	}
	_, names := processTable()
	if n, ok := names[uint32(pid)]; ok {
		return n
	}
	return "pid " + strconv.Itoa(pid)
}
