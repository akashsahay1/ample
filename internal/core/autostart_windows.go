package core

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"apnoro/internal/paths"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// launchAtLoginEnabled reports whether the Run entry exists (the installer's
// "Start Apnoro when Windows starts" task creates it without touching config.json).
func launchAtLoginEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("Apnoro")
	return err == nil
}

func setLaunchAtLogin(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue("Apnoro"); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe := filepath.Join(paths.InstallDir(), "Apnoro.exe")
	return k.SetStringValue("Apnoro", `"`+exe+`" --hidden`)
}
