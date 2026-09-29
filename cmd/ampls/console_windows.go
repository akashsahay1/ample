//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
)

const enableVirtualTerminalProcessing = 0x0004

// initConsole switches the console to UTF-8 (for ✓/✗ and the progress bar) and
// enables ANSI colors when stdout is an interactive console.
func initConsole() {
	if !isTerminal() {
		return
	}
	_, _, _ = procSetConsoleOutputCP.Call(65001)
	h := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing)); r == 0 {
		return
	}
	colorOn = os.Getenv("NO_COLOR") == ""
}
