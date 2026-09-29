package main

import (
	"ampls/internal/api"
	"ampls/internal/api/mock"
)

// newBackend selects the Backend implementation. Owned by the lead: switch to
// core.New() (keeping mock.New() when AMPLS_MOCK=1).
func newBackend() api.Backend {
	return mock.New()
}
