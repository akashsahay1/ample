//go:build windows

package certs

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// secretSDDL grants full control to SYSTEM, the built-in Administrators and
// the given user SID only, and is protected (P) so the Users-modify ACL of the
// data directory is not inherited.
func secretSDDL(userSID string) string {
	return "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + userSID + ")"
}

func currentUserSID() (string, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return tu.User.Sid.String(), nil
}

// createExclusive creates a new file (failing with os.ErrExist if the name
// exists, links included). A secret file gets a restrictive DACL at creation
// time, so there is no window in which other users could open it.
func createExclusive(name string, secret bool) (*os.File, error) {
	var sa *windows.SecurityAttributes
	if secret {
		sid, err := currentUserSID()
		if err != nil {
			return nil, fmt.Errorf("certs: current user: %w", err)
		}
		sd, err := windows.SecurityDescriptorFromString(secretSDDL(sid))
		if err != nil {
			return nil, fmt.Errorf("certs: security descriptor: %w", err)
		}
		sa = &windows.SecurityAttributes{SecurityDescriptor: sd}
		sa.Length = uint32(unsafe.Sizeof(*sa))
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, sa, windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil, &os.PathError{Op: "create", Path: name, Err: os.ErrExist}
		}
		return nil, &os.PathError{Op: "create", Path: name, Err: err}
	}
	return os.NewFile(uintptr(h), name), nil
}
