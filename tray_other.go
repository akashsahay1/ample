//go:build !windows

package main

// macOS tray comes later (energye/systray needs cgo on darwin).
const trayAvailable = false

func startTray(*App) {}
func stopTray()      {}
