//go:build !windows

package core

// setLaunchAtLogin is implemented with a LaunchAgent in the macOS port.
func setLaunchAtLogin(on bool) error { return nil }

func launchAtLoginEnabled() bool { return false }
