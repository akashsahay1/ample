//go:build windows

package certs

import (
	"crypto/sha1"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsCATrusted reports whether the CA is in the CurrentUser or LocalMachine
// Root store (looked up by SHA-1 thumbprint).
func IsCATrusted() bool {
	c, err := readCert(CAPath())
	if err != nil {
		return false
	}
	sum := sha1.Sum(c.Raw)
	for _, loc := range []uint32{windows.CERT_SYSTEM_STORE_CURRENT_USER, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE} {
		if inRootStore(loc, sum[:]) {
			return true
		}
	}
	return false
}

func inRootStore(location uint32, thumb []byte) bool {
	name, _ := windows.UTF16PtrFromString("Root")
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0,
		location|windows.CERT_STORE_READONLY_FLAG|windows.CERT_STORE_OPEN_EXISTING_FLAG,
		uintptr(unsafe.Pointer(name)))
	if err != nil {
		return false
	}
	defer windows.CertCloseStore(store, 0)
	blob := windows.CryptHashBlob{Size: uint32(len(thumb)), Data: &thumb[0]}
	ctx, err := windows.CertFindCertificateInStore(store,
		windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0,
		windows.CERT_FIND_SHA1_HASH, unsafe.Pointer(&blob), nil)
	if err != nil || ctx == nil {
		return false
	}
	windows.CertFreeCertificateContext(ctx)
	return true
}

// TrustCA adds the CA to the Root store: machine-wide (requires admin; used by
// the installer) or for the current user (Windows shows a confirmation dialog).
func TrustCA(machine bool) error {
	if err := EnsureCA(); err != nil {
		return err
	}
	args := []string{"-addstore", "-f", "Root", CAPath()}
	if !machine {
		args = append([]string{"-user"}, args...)
	}
	cmd := exec.Command("certutil", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certs: certutil %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	if !IsCATrusted() {
		return fmt.Errorf("certs: CA was not added to the trust store (dialog declined?)")
	}
	return nil
}
