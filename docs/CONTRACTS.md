# Apnoro — engineering contracts

Apnoro: Apache, PHP and MySQL for Windows. A Windows-first (macOS later) local PHP dev
environment: XAMPP-style bundled stack + Laravel Herd-style per-project PHP versions.

Go module: `apnoro` (Go 1.25+). GUI: Wails v2 + React/TS/Tailwind in `frontend/`.
Every package here is shared by the GUI (`Apnoro.exe`), the CLI (`apnoro.exe`), the php shim
(`php.exe`) and the hosts helper service (`apnoro-helper.exe`).

## Ground rules for all contributors
- **Only edit the files/packages you own** (see Ownership). Shared files (`internal/paths`,
  `internal/config`, `internal/api`) are owned by the lead; ask in your report if you need a change.
- **Cross-platform discipline**: macOS support comes later. Put OS-specific code in
  `*_windows.go` / `*_darwin.go` (plus `*_other.go` or `//go:build !windows` fallbacks) so
  `GOOS=darwin go build ./...` and `GOOS=windows go build ./...` both compile.
- No cgo. Pure Go only (pure-Go deps are fine: `golang.org/x/sys`, `go-sql-driver/mysql`,
  `spf13/cobra`, `fsnotify`).
- Never hide console windows incorrectly: child processes on Windows must be started with
  `SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}` when spawned from the GUI.
- Errors: wrap with context (`fmt.Errorf("apache: write config: %w", err)`).
- Every package ships unit tests for its pure logic (`go test ./internal/...` must pass).
- Paths always come from `internal/paths`. Config from `internal/config`.

## Data directory layout (`paths.Home()`, user-chosen, default `C:\Apnoro`)
```
apache/                 Apache Lounge httpd (bin/httpd.exe, modules/, incl. mod_fcgid.so)
php/<minor>/            e.g. php/8.5/  php.exe, php-cgi.exe, ext/, php.ini (generated)
mysql/                  MySQL 8.4 zip distribution (bin/mysqld.exe ...)
data/mysql/             MySQL datadir
conf/httpd.conf         generated main Apache config
conf/sites/*.conf       generated vhosts (one per site)
conf/my.ini             generated MySQL config
certs/ca.crt ca.key     local root CA;  certs/sites/<domain>.crt/.key
logs/                   apache-error.log apache-access.log mysql.log php-<minor>-error.log
run/                    *.pid, hosts.json (desired hosts), hosts.applied.json
apps/phpmyadmin/        phpMyAdmin (served at http://localhost/phpmyadmin)
www/                    default localhost page
downloads/ tmp/         scratch
config.json             internal/config
```
Install dir (`paths.InstallDir()`, e.g. `C:\Program Files\Apnoro`):
`Apnoro.exe`, `data-dir.txt`, `bin\apnoro.exe`, `bin\php.exe` (shim), `bin\composer.phar`,
`bin\composer.bat`, `bin\apnoro-helper.exe`. `bin\` is on PATH.

## How per-site PHP works
Apache + **mod_fcgid**. Every vhost's `<Directory>` gets
`FcgidWrapper "<Home>/php/<minor>/php-cgi.exe" .php`. php-cgi reads the `php.ini` next to it.
Changing a site's version = regenerate that vhost + restart Apache.
The `php.exe` shim picks the version for CLI use (see `internal/shim`).

---
## Package APIs (exact exported signatures to implement)

### `internal/download` — owner: Agent A
```go
type ProgressFunc func(done, total int64)
func File(ctx context.Context, url, dest string, progress ProgressFunc) error // resumable-safe temp file + rename, sets a browser-like User-Agent
func Unzip(src, dest string, stripTopDir bool) error // stripTopDir: if zip has a single top-level dir, extract its contents into dest
func SHA256File(path string) (string, error)
```

### `internal/php` — owner: Agent A
```go
type Release struct { Minor, Full, URL, SHA256 string; Size int64 }
type Installed struct { Minor, Full, Dir string }
func Available(ctx context.Context) ([]Release, error) // NTS x64 builds; windows.php.net releases.json + archives for 7.4/8.0/8.1; newest first
func List() ([]Installed, error)                       // scans paths.PHPRoot(); newest first
func Install(ctx context.Context, minor string, progress func(msg string, pct float64)) error
func InstallFromZip(zipPath string) (Installed, error) // used by setup for the bundled PHP
func Remove(minor string) error
func IsEOL(minor string) bool
func CLIPath(minor string) string  // .../php.exe
func CGIPath(minor string) string  // .../php-cgi.exe
func IniPath(minor string) string
func EnsureIni(minor string) error // create php.ini from php.ini-development with sane defaults (extension_dir absolute, common exts enabled, error_log -> logs/php-<minor>-error.log, cacert via curl.cainfo if bundled, date.timezone=UTC)
type Settings struct { Values map[string]string; Extensions []Ext }
type Ext struct { Name string; Enabled bool }
func ReadSettings(minor string) (Settings, error)  // Values keys: memory_limit upload_max_filesize post_max_size max_execution_time display_errors; Extensions: every ext/php_*.dll found
func WriteSettings(minor string, s Settings) error // edits php.ini in place preserving comments; xdebug = zend_extension
func Compare(a, b string) int                      // version compare, "8.10" > "8.9"
```

### `internal/apache` — owner: Agent A
```go
type VHost struct {
    Domain   string   // blog.test
    Aliases  []string // *.blog.test
    DocRoot  string
    PHPCGI   string   // absolute php-cgi.exe
    Secure   bool
    CertFile, KeyFile string
}
type Options struct {
    HTTPPort, HTTPSPort int
    DefaultPHPCGI       string // for localhost + phpMyAdmin
}
func WriteConfig(opts Options, vhosts []VHost) error // writes conf/httpd.conf + conf/sites/*.conf (removes stale); forward slashes in Apache paths
func ConfigTest() error                              // httpd -t
func Version() string                                // "2.4.x" or ""
func HttpdPath() string
func StartArgs() []string                            // args to run httpd in foreground with our config
```

### `internal/mysql` — owner: Agent A
```go
func Initialized() bool
func Initialize() error                    // mysqld --initialize-insecure (root, empty password)
func WriteConfig(port int) error           // conf/my.ini
func MysqldPath() string
func StartArgs() []string
func Version() string
func Shutdown(port int, password string) error  // graceful via SQL SHUTDOWN
func ListDatabases(port int, password string) ([]api.Database, error) // exclude system schemas
func CreateDatabase(port int, password, name string) error
func DropDatabase(port int, password, name string) error
func Import(port int, password, database, file string) error  // mysql.exe < file
func Export(port int, password, database, file string) error  // mysqldump.exe
func SetRootPassword(port int, oldPassword, newPassword string) error
func Ping(port int, password string) error
```

### `internal/services` — owner: Agent A
Services are **detached** background processes (they keep running after the CLI exits and
when the GUI closes unless the user opts in to stop). PIDs in `run/<name>.pid`.
```go
type Status struct { Running bool; PID int }
func Start(name string, exe string, args []string, logFile string) error // detached, hidden window, stdout/stderr -> logFile; waits until process is alive
func Stop(name string, graceful func() error) error                      // graceful first (may be nil), then kill whole process tree
func Status(name string) Status                                          // pid file + process alive check
func WaitPort(port int, timeout time.Duration) error
func PortInUse(port int) (bool, string) // second value: owning process name if detectable
```

### `internal/sites` — owner: Agent B
```go
func Discover(cfg *config.Config) ([]api.Site, error) // parked dirs' subfolders + links; applies cfg.Sites overrides; effective PHP = override or cfg.DefaultPHP; sorted by name
func Find(cfg *config.Config, name string) (api.Site, bool)
func FindByPath(cfg *config.Config, dir string) (api.Site, bool) // dir anywhere inside a site's folder
func Slug(folder string) string          // "My Blog" -> "my-blog"
func ValidName(name string) bool         // ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$
func DocRoot(path string) string         // public/ (Laravel/Symfony), web/, else path
func DetectFramework(path string) string // "laravel" | "wordpress" | "php"
```

### `internal/hosts` — owner: Agent B
Hosts file edits need admin. Apnoro installs `apnoro-helper.exe` as a LocalSystem Windows
service that applies a validated managed block. Fallback: UAC-elevated `apnoro.exe hosts apply`.
```go
const BeginMarker = "# BEGIN APNORO"; const EndMarker = "# END APNORO"
func Render(existing string, domains []string) string // pure: replaces/appends managed block, 127.0.0.1 + ::1 lines
func Validate(domains []string, tld string) error     // each is <label>.<tld> or sub.<label>.<tld>, strict charset
func Current() ([]string, error)                      // domains inside managed block
func Apply(domains []string) error                    // direct write (requires admin)
func Request(domains []string, tld string) error      // non-admin entry point: no-op if already applied; else write run/hosts.json and wait (<=5s) for helper; else elevate `apnoro.exe hosts apply`
func HostsPath() string
// service side (Windows): cmd/apnoro-helper uses these
func RunHelper(home string) error                     // loop: watch run/hosts.json, Validate, Apply, write run/hosts.applied.json
```

### `internal/certs` — owner: Agent B
```go
func EnsureCA() error                                   // certs/ca.crt + ca.key, RSA 2048/ECDSA P-256, 10y, CN "Apnoro Local CA"
func CAPath() string
func IsCATrusted() bool                                 // Windows: certutil -user -verifystore Root / cert store lookup
func TrustCA(machine bool) error                        // certutil [-user] -addstore Root
func EnsureSiteCert(domain string) (certFile, keyFile string, err error) // SAN: domain, *.domain; 825 days
func RemoveSiteCert(domain string) error
```

### `internal/projects` — owner: Agent B
```go
type Request struct {
    Kind, Name, Dir string        // Dir = parent directory
    PHP          string          // php.exe to use (absolute)
    ComposerPhar string
    DB           string          // database name to wire into .env / wp-config ("" = none)
    DBPort       int
    DBPassword   string
}
func Create(ctx context.Context, r Request, progress func(msg string, pct float64)) (projectPath string, err error)
// laravel: php composer.phar create-project laravel/laravel <name> ; set .env DB_* + APP_URL
// wordpress: download https://wordpress.org/latest.zip, extract, write wp-config.php with DB + salts
// blank: index.php with phpinfo link + README
```

### `internal/shim` + `cmd/php-shim` — owner: Agent B
```go
func ResolveVersion(cwd string) (minor string, source string, err error)
// order: nearest ".apnoro-php" file walking up (content "8.3"); site containing cwd (config override); cfg.DefaultPHP; newest installed
```
`cmd/php-shim/main.go` builds to `bin/php.exe`: resolves, then runs `<Home>/php/<minor>/php.exe`
with all args, stdio passthrough, same exit code. Must be fast (no network, no heavy init).

### `internal/core` + `cmd/apnoro` — owner: lead (after A & B)
`core.New()` implements `api.Backend` by orchestrating the packages above.
CLI: `apnoro start|stop|restart|status|sites|park|unpark|link|unlink|isolate|unisolate|secure|unsecure|php:list|php:install|php:use|php:remove|db:list|db:create|logs|open|setup|hosts apply|trust`.

### GUI — owner: Agent C
`main.go`, `app.go`, `tray*.go`, `internal/api/mock/`, `frontend/`. The Wails-bound `App`
wraps an `api.Backend` (mock when `APNORO_MOCK=1` or build tag `mock`). Design mockups:
`docs/design/*.dc.html` (open in a browser to view; also https://claude.ai/artifact/DZ57JdMKQPbB4rway2TBjW).

### Installer & build — owner: Agent D
`installer/`, `scripts/`, `build/appicon.png`, `build/windows/icon.ico`, `assets/`.
