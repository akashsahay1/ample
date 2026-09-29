// Package paths resolves every filesystem location AMPLS uses.
//
// Two roots exist:
//   - InstallDir: where the programs live (AMPLS.exe, bin\ampls.exe, bin\php.exe shim).
//     Read-only at runtime.
//   - Home: the user-chosen data directory (default C:\AMPLS on Windows). Holds the
//     runtimes (Apache, PHP versions, MySQL), generated configs, certs, logs and data.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// HomeFileName is written by the installer next to AMPLS.exe and contains the data dir.
const HomeFileName = "data-dir.txt"

var (
	homeOnce     sync.Once
	homeOverride string
	home         string
)

// SetHome forces the data directory (used by `ampls setup --home` and tests).
// Must be called before the first call to Home.
func SetHome(dir string) { homeOverride = dir }

// Home returns the AMPLS data directory.
// Resolution order: SetHome, $AMPLS_HOME, <InstallDir>/data-dir.txt, OS default.
func Home() string {
	homeOnce.Do(func() {
		switch {
		case homeOverride != "":
			home = homeOverride
		case os.Getenv("AMPLS_HOME") != "":
			home = os.Getenv("AMPLS_HOME")
		default:
			if b, err := os.ReadFile(filepath.Join(InstallDir(), HomeFileName)); err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					home = s
				}
			}
			if home == "" {
				home = defaultHome()
			}
		}
		home = filepath.Clean(home)
	})
	return home
}

func defaultHome() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\AMPLS`
	case "darwin":
		h, _ := os.UserHomeDir()
		return filepath.Join(h, "Library", "Application Support", "AMPLS")
	default:
		h, _ := os.UserHomeDir()
		return filepath.Join(h, ".ampls")
	}
}

// InstallDir is the directory holding AMPLS.exe. Binaries in <InstallDir>/bin
// (ampls.exe, php.exe shim) resolve to the parent directory.
func InstallDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	dir := filepath.Dir(exe)
	if b := filepath.Base(dir); strings.EqualFold(b, "bin") || strings.EqualFold(b, "shims") {
		return filepath.Dir(dir)
	}
	return dir
}

// BinDir holds the CLI and the hosts helper; it is added to PATH.
func BinDir() string { return filepath.Join(InstallDir(), "bin") }

// ShimsDir holds the php shim, composer.phar and composer.bat. It is on PATH only
// when the user chose so in the installer (it may shadow Herd/XAMPP php).
func ShimsDir() string { return filepath.Join(InstallDir(), "shims") }

func ApacheDir() string          { return filepath.Join(Home(), "apache") }
func PHPRoot() string            { return filepath.Join(Home(), "php") }
func PHPDir(minor string) string { return filepath.Join(PHPRoot(), minor) }
func MySQLDir() string           { return filepath.Join(Home(), "mysql") }
func DataDir() string            { return filepath.Join(Home(), "data") }
func MySQLDataDir() string       { return filepath.Join(DataDir(), "mysql") }
func ConfDir() string            { return filepath.Join(Home(), "conf") }
func SitesConfDir() string       { return filepath.Join(ConfDir(), "sites") }
func CertsDir() string           { return filepath.Join(Home(), "certs") }
func LogsDir() string            { return filepath.Join(Home(), "logs") }
func RunDir() string             { return filepath.Join(Home(), "run") }
func TmpDir() string             { return filepath.Join(Home(), "tmp") }
func DownloadsDir() string       { return filepath.Join(Home(), "downloads") }
func AppsDir() string            { return filepath.Join(Home(), "apps") }
func PHPMyAdminDir() string      { return filepath.Join(AppsDir(), "phpmyadmin") }
func WWWDir() string             { return filepath.Join(Home(), "www") }
func ConfigFile() string         { return filepath.Join(Home(), "config.json") }

// ComposerPhar is bundled next to the php shim.
func ComposerPhar() string { return filepath.Join(ShimsDir(), "composer.phar") }

// DefaultSitesDir is the folder parked on first run (~/AMPLS/Sites).
func DefaultSitesDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "AMPLS", "Sites")
}

// EnsureDirs creates the writable directory tree under Home.
func EnsureDirs() error {
	for _, d := range []string{
		Home(), PHPRoot(), DataDir(), ConfDir(), SitesConfDir(), CertsDir(),
		LogsDir(), RunDir(), TmpDir(), DownloadsDir(), AppsDir(), WWWDir(),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Exe appends ".exe" on Windows.
func Exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
