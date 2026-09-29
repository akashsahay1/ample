//go:build windows

package external

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const isWindows = true

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const (
	afInet                  = 2
	afInet6                 = 23
	tcpTableOwnerPIDListen  = 3
	errInsufficientBuffer   = 122
	stillActive             = 259
	queryLimitedInformation = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

// listenPorts maps pid -> listening TCP ports (IPv4 + IPv6) via GetExtendedTcpTable.
func listenPorts() map[int][]int {
	out := map[int][]int{}
	for _, fam := range []uint32{afInet, afInet6} {
		buf := tcpTable(fam)
		if len(buf) < 4 {
			continue
		}
		n := int(binary.LittleEndian.Uint32(buf[:4]))
		rowSize, portOff, pidOff := 24, 8, 20 // MIB_TCPROW_OWNER_PID
		if fam == afInet6 {
			rowSize, portOff, pidOff = 56, 20, 52 // MIB_TCP6ROW_OWNER_PID
		}
		for i := 0; i < n; i++ {
			off := 4 + i*rowSize
			if off+rowSize > len(buf) {
				break
			}
			row := buf[off : off+rowSize]
			port := int(binary.BigEndian.Uint16(row[portOff : portOff+2]))
			pid := int(binary.LittleEndian.Uint32(row[pidOff : pidOff+4]))
			out[pid] = appendUnique(out[pid], port)
		}
	}
	for pid := range out {
		sort.Ints(out[pid])
	}
	return out
}

func tcpTable(family uint32) []byte {
	size := uint32(16 * 1024)
	for tries := 0; tries < 5; tries++ {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)),
			0, uintptr(family), tcpTableOwnerPIDListen, 0)
		if r == 0 {
			return buf[:size]
		}
		if r != errInsufficientBuffer {
			return nil
		}
		size += 4096
	}
	return nil
}

func appendUnique(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// Processes lists running processes with their executable path (when the
// current user may query it) and listening ports.
func Processes() []Proc {
	ports := listenPorts()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []Proc
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := Proc{PID: int(e.ProcessID), PPID: int(e.ParentProcessID), Name: windows.UTF16ToString(e.ExeFile[:])}
		p.Exe = imagePath(p.PID)
		p.Ports = ports[p.PID]
		out = append(out, p)
	}
	return out
}

func imagePath(pid int) string {
	if pid == 0 || pid == 4 {
		return ""
	}
	h, err := windows.OpenProcess(queryLimitedInformation, false, uint32(pid))
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

func pidAlive(pid int) bool {
	h, err := windows.OpenProcess(queryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == stillActive
}

// ErrAccessDenied is returned by Stop when a process belongs to another user
// or runs as a Windows service.
var ErrAccessDenied = errors.New("access denied")

func killPID(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return ErrAccessDenied
		}
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return ErrAccessDenied
		}
		return err
	}
	windows.WaitForSingleObject(h, uint32((5 * time.Second).Milliseconds()))
	return nil
}

// fixedDrives returns "C:\", "D:\" ... for local fixed disks.
func fixedDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{`C:\`}
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(p) == windows.DRIVE_FIXED {
			out = append(out, root)
		}
	}
	return out
}

func regString(path, name string) string {
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue(name)
		k.Close()
		if err == nil && v != "" {
			return v
		}
	}
	return ""
}

func candidateRoots(kind string) []string {
	var out []string
	drives := fixedDrives()
	valid := func(root string, markers ...string) {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(root, m)); err == nil {
				out = append(out, root)
				return
			}
		}
	}
	switch kind {
	case "xampp":
		for _, r := range []string{regString(`SOFTWARE\xampp`, "Install_Dir"), regString(`SOFTWARE\WOW6432Node\xampp`, "Install_Dir")} {
			if r != "" {
				valid(r, `apache\bin\httpd.exe`, "xampp-control.exe")
			}
		}
		for _, d := range drives {
			valid(filepath.Join(d, "xampp"), `apache\bin\httpd.exe`, "xampp-control.exe")
		}
	case "herd":
		if h, err := os.UserHomeDir(); err == nil {
			valid(filepath.Join(h, ".config", "herd"), `config\valet\config.json`, `bin\herd.bat`)
		}
	case "laragon":
		for _, d := range drives {
			valid(filepath.Join(d, "laragon"), "laragon.exe", `bin\apache`)
		}
	case "wamp":
		for _, d := range drives {
			valid(filepath.Join(d, "wamp64"), "wampmanager.exe", `bin\apache`)
			valid(filepath.Join(d, "wamp"), "wampmanager.exe", `bin\apache`)
		}
	}
	return out
}

func herdInstallCandidates() []string {
	var out []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs")} {
		if base != "" && isFile(filepath.Join(base, "Herd", "Herd.exe")) {
			out = append(out, filepath.Join(base, "Herd"))
		}
	}
	return out
}
