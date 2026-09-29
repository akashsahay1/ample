//go:build windows

package hosts

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// lockDir opens dir and pins it for the privileged helper: the directory must
// not be a reparse point (junction / symlink / mount point), its resolved path
// must equal the expected path (so no ancestor is a junction either), and the
// handle is opened without FILE_SHARE_DELETE, so while it is held nobody can
// rename or delete dir (or rename any ancestor) to swap in a junction that
// would redirect LocalSystem's reads and writes elsewhere. Call release when
// done.
func lockDir(dir string) (release func(), err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, // deliberately no FILE_SHARE_DELETE
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("hosts: open %s: %w", abs, err)
	}
	fail := func(e error) (func(), error) {
		windows.CloseHandle(h)
		return nil, e
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return fail(fmt.Errorf("hosts: stat %s: %w", abs, err))
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fail(fmt.Errorf("hosts: %s is not a directory", abs))
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fail(fmt.Errorf("hosts: %s is a junction or symbolic link; refusing to use it", abs))
	}
	final, err := finalPath(h)
	if err != nil {
		return fail(fmt.Errorf("hosts: resolve %s: %w", abs, err))
	}
	if !samePath(final, abs) {
		return fail(fmt.Errorf("hosts: %s resolves to %s (junction in path?); refusing to use it", abs, final))
	}
	return func() { windows.CloseHandle(h) }, nil
}

func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0 /* FILE_NAME_NORMALIZED|VOLUME_NAME_DOS */)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return stripExtended(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n+1)
	}
}

func stripExtended(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		return p[len(`\\?\`):]
	}
	return p
}

func longPath(p string) string {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetLongPathName(u, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) >= len(buf) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}

func samePath(final, want string) bool {
	clean := func(s string) string { return strings.TrimRight(filepath.Clean(s), `\`) }
	if strings.EqualFold(clean(final), clean(want)) {
		return true
	}
	return strings.EqualFold(clean(final), clean(longPath(want)))
}
