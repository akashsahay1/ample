//go:build windows

package hosts

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"ampls/internal/paths"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
)

// quoteArg quotes a command-line argument per CommandLineToArgvW rules.
func quoteArg(s string) string { return syscall.EscapeArg(s) }

// runElevated starts exe with args via the "runas" verb (UAC prompt) and
// waits for it to exit, returning its exit code.
func runElevated(exe string, args []string) (uint32, error) {
	params := ""
	for i, a := range args {
		if i > 0 {
			params += " "
		}
		params += quoteArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	par, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: par,
		nShow:        windows.SW_HIDE,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, errors.New("administrator permission was declined")
		}
		return 0, fmt.Errorf("ShellExecuteEx: %w", callErr)
	}
	if info.hProcess == 0 {
		return 0, errors.New("no process handle")
	}
	defer windows.CloseHandle(info.hProcess)
	ev, err := windows.WaitForSingleObject(info.hProcess, 120*1000)
	if err != nil {
		return 0, err
	}
	if ev != windows.WAIT_OBJECT_0 {
		return 0, errors.New("timed out waiting for elevated process")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, err
	}
	return code, nil
}

func elevateApply() error {
	exe := paths.BinDir() + `\ampls.exe`
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("%s not found: %w", exe, err)
	}
	code, err := runElevated(exe, []string{"hosts", "apply", "--home", paths.Home()})
	if err != nil {
		return err
	}
	if code != 0 {
		if a, err := readApplied(paths.RunDir()); err == nil && a.Error != "" {
			return errors.New(a.Error)
		}
		return fmt.Errorf("ampls hosts apply exited with code %d", code)
	}
	return nil
}

func flushDNS() {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	_ = cmd.Run()
}
