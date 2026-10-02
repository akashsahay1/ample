package main

import (
	"os"

	"apnoro/internal/api"
	"apnoro/internal/api/mock"
	"apnoro/internal/core"
)

// newBackend returns the real core, or the in-memory mock when APNORO_MOCK=1
// (useful for UI work without Apache/MySQL installed).
func newBackend() api.Backend {
	if os.Getenv("APNORO_MOCK") == "1" {
		return mock.New()
	}
	core.Version = version
	core.Build = build
	c := core.New()
	c.BackgroundSync = true // the GUI is long-running: apply new parked folders automatically
	return c
}
