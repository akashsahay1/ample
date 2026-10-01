package main

import (
	"os"

	"ampls/internal/api"
	"ampls/internal/api/mock"
	"ampls/internal/core"
)

// newBackend returns the real core, or the in-memory mock when AMPLS_MOCK=1
// (useful for UI work without Apache/MySQL installed).
func newBackend() api.Backend {
	if os.Getenv("AMPLS_MOCK") == "1" {
		return mock.New()
	}
	core.Version = version
	core.Build = build
	c := core.New()
	c.BackgroundSync = true // the GUI is long-running: apply new parked folders automatically
	return c
}
