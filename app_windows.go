//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func openFolder(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	// explorer.exe returns exit code 1 even on success; ignore its result.
	cmd := exec.Command("explorer.exe", path)
	return cmd.Start()
}

func openPath(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path)
	return cmd.Start()
}

func openTerminal(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		return exec.Command(wt, "-d", dir).Start()
	}
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: fmt.Sprintf(`cmd.exe /K cd /d "%s"`, dir), CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	cmd.Dir = dir
	return cmd.Start()
}
