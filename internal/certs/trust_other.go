//go:build !windows && !darwin

package certs

import "errors"

// IsCATrusted is not implemented on this OS.
func IsCATrusted() bool { return false }

// TrustCA is not implemented on this OS.
func TrustCA(machine bool) error {
	return errors.New("certs: trusting the CA is not supported on this OS; import " + CAPath() + " manually")
}
