//go:build !windows

package hosts

import (
	"fmt"
	"os"
	"path/filepath"
)

// lockDir checks that dir is a real directory reached without symlinks (see
// the Windows implementation, which additionally pins it while held).
func lockDir(dir string) (release func(), err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return nil, fmt.Errorf("hosts: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("hosts: %s is not a directory", abs)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("hosts: %w", err)
	}
	if filepath.Clean(real) != filepath.Clean(abs) {
		return nil, fmt.Errorf("hosts: %s resolves to %s (symlink in path?); refusing to use it", abs, real)
	}
	return func() {}, nil
}
