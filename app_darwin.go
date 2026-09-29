//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func openFolder(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	return exec.Command("open", path).Start()
}

func openPath(path string) error { return openFolder(path) }

func openTerminal(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	return exec.Command("open", "-a", "Terminal", dir).Start()
}
