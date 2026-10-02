// Package paths resolves every filesystem location Apnoro uses.
//
// Two roots exist:
//   - InstallDir: where the programs live (Apnoro.exe, bin\apnoro.exe, bin\php.exe shim).
//     Read-only at runtime.
//   - Home: the user-chosen data directory (default C:\Apnoro on Windows). Holds the
//     runtimes (Apache, PHP versions, MySQL), generated configs, certs, logs and data.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// HomeFileName is written by the installer next to Apnoro.exe and contains the data dir.
const HomeFileName = "data-dir.txt"

var (
	homeOnce     sync.Once
	homeOverride string
	home         string
)

// SetHome forces the data directory (used by `apnoro setup --home` and tests).
// Must be called before the first call to Home.
func SetHome(dir string) { homeOverride = dir }

// Home returns the Apnoro data directory.
// Resolution order: SetHome, $APNORO_HOME, <InstallDir>/data-dir.txt, OS default.
func Home() string {
	homeOnce.Do(func() {
		switch {
		case homeOverride != "":
			home = homeOverride
		case os.Getenv("APNORO_HOME") != "":
			home = os.Getenv("APNORO_HOME")
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
		return `C:\Apnoro`
	case "darwin":
		h, _ := os.UserHomeDir()
		return filepath.Join(h, "Library", "Application Support", "Apnoro")
	default:
		h, _ := os.UserHomeDir()
		return filepath.Join(h, ".apnoro")
	}
}

// InstallDir is the directory holding Apnoro.exe. Binaries in <InstallDir>/bin
// (apnoro.exe, php.exe shim) resolve to the parent directory.
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

// DefaultSitesDir is the folder parked on first run (~/Apnoro/Sites).
func DefaultSitesDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Apnoro", "Sites")
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
