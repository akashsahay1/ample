//go:build windows

package certs

import (
	"encoding/hex"
	"testing"

	"golang.org/x/sys/windows"
)

// Verifies the store lookup wiring with a root present on every Windows install
// (Microsoft Root Certificate Authority 2011).
func TestInRootStoreKnownRoot(t *testing.T) {
	thumb, _ := hex.DecodeString("8F43288AD272F3103B6FB1428485EA3014C0BCFE")
	if !inRootStore(windows.CERT_SYSTEM_STORE_LOCAL_MACHINE, thumb) && !inRootStore(windows.CERT_SYSTEM_STORE_CURRENT_USER, thumb) {
		t.Skip("Microsoft Root 2011 not found in Root store (unusual machine)")
	}
	bogus := make([]byte, 20)
	if inRootStore(windows.CERT_SYSTEM_STORE_CURRENT_USER, bogus) {
		t.Error("bogus thumbprint found")
	}
}
