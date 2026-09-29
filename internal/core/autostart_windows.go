package core

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"ampls/internal/paths"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func setLaunchAtLogin(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue("AMPLS"); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe := filepath.Join(paths.InstallDir(), "AMPLS.exe")
	return k.SetStringValue("AMPLS", `"`+exe+`" --hidden`)
}
