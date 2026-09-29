package main

import _ "embed"

// Tray icons generated from assets/icon/tray-*.svg (scripts/icon-tool).
// "partial" (some services stopped) uses the orange error dot.

//go:embed assets/icons/tray-running.ico
var trayRunningICO []byte

//go:embed assets/icons/tray-stopped.ico
var trayStoppedICO []byte

//go:embed assets/icons/tray-error.ico
var trayErrorICO []byte

var (
	iconRunning = func() []byte { return trayRunningICO }
	iconStopped = func() []byte { return trayStoppedICO }
	iconPartial = func() []byte { return trayErrorICO }
)
