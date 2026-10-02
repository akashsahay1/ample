# Architecture

Apnoro is a Go module (`apnoro`, Go 1.25+) with a Wails v2 + React/TypeScript GUI. Four executables share the same internal packages:

| Executable | Role |
|---|---|
| `Apnoro.exe` | Wails desktop app with tray (`main.go`, `app.go`, `frontend/`) |
| `bin\apnoro.exe` | Cobra CLI (`cmd/apnoro`) |
| `bin\php.exe` | `php` shim that picks a PHP version and execs the real one (`cmd/php-shim`) |
| `bin\apnoro-helper.exe` | Windows service that edits the hosts file (`cmd/apnoro-helper`) |

## Overview

```
 GUI (Wails)        CLI (apnoro)        php shim
      \                 |                 |
       \                |                 v
        +---> api.Backend <--- internal/core       internal/shim
                    |
   +----------+-----+------+----------+---------+----------+
   |          |            |          |         |          |
 sites      apache       mysql      php       certs    projects
 (discover) (vhost cfg)  (cfg,SQL)  (install, (CA,     (laravel,
                                     ini)     certs)   wp, blank)
   |          |            |
   |          v            v
   |     services (detached procs + pid files)
   v
 hosts.Request --> run/hosts.json --> apnoro-helper (LocalSystem) --> hosts file
                                           ^ fallback: UAC `apnoro hosts apply`

 Browser --> Apache :80/:443 (127.0.0.1)
               `-- mod_fcgid --> php\<minor>\php-cgi.exe  (per vhost)
 App / PHP  --> MySQL 127.0.0.1:3306
```

## Layers

- **`internal/api`**: the `Backend` interface and DTOs shared by GUI and CLI. `internal/core` implements it by orchestrating the packages below; a mock backend (`APNORO_MOCK=1`) drives frontend development.
- **`internal/paths`**: every filesystem location. **`internal/config`**: `config.json` in the data dir (ports, TLD, parked dirs, links, per-site overrides, MySQL root password, app settings).
- **`internal/sites`**: discovers sites from parked directories and links, applies overrides, detects framework and docroot (`public\`, `web\`).
- **`internal/apache`**: renders `conf\httpd.conf` and one `conf\sites\*.conf` per site; validates with `httpd -t`.
- **`internal/mysql`**: `my.ini`, initialization (`--initialize-insecure`), graceful shutdown, database operations, import/export.
- **`internal/php`**: available/installed versions, install from windows.php.net, `php.ini` generation and editing, extension toggling.
- **`internal/certs`**: local CA and per-site certificates, trust store integration.
- **`internal/hosts`**: managed hosts block rendering, validation and helper protocol.
- **`internal/projects`**: Laravel (Composer), WordPress (zip) and blank scaffolding.
- **`internal/services`**: starting, stopping and probing service processes.
- **`internal/download`**: resumable downloads, unzip, SHA-256.

## Process model

- **Detached services.** Apache and MySQL are started as hidden background processes with stdout/stderr redirected to `logs\`, and their PIDs are written to `run\<name>.pid`. They outlive the CLI and the GUI. Stopping tries a graceful path first (MySQL `SHUTDOWN` over SQL), then kills the whole process tree. Status is the pid file plus a liveness check.
- **Per-site PHP.** Each vhost's `<Directory>` gets `FcgidWrapper "<Home>/php/<minor>/php-cgi.exe" .php`. php-cgi reads the `php.ini` next to it. Changing a site's version regenerates that vhost and restarts Apache. `localhost` and phpMyAdmin use the default PHP.
- **Hosts helper.** Writing the hosts file needs admin. The installer registers `ApnoroHelper` (LocalSystem). Unprivileged callers write the desired domains to `run\hosts.json` (`hosts.Request`); the helper watches that file, validates every domain (`<label>.<tld>` or `sub.<label>.<tld>`, strict charset), rewrites only the block between `# BEGIN APNORO` and `# END APNORO` (127.0.0.1 and ::1 lines), and records the result in `run\hosts.applied.json`. If the helper does not answer within about 5 seconds, Apnoro falls back to a UAC-elevated `apnoro.exe hosts apply`.
- **`php` shim.** Resolves the version (nearest `.apnoro-php`, containing site's override, default, newest installed) using only local files, then runs `<Home>\php\<minor>\php.exe` with the same arguments, stdio and exit code.
- **Two roots.** Install dir (programs, read-only at runtime, contains `data-dir.txt`) and the user-chosen data dir (`paths.Home()`: `$APNORO_HOME`, else `data-dir.txt`, else `C:\Apnoro`).

## Security notes

- Apache and MySQL listen on `127.0.0.1` only; nothing is exposed to the network. The default MySQL root password is empty, which is acceptable only because of that loopback binding.
- The helper accepts no free-form input: it only writes lines it has validated and only inside its marked block. Requests come from a file in the data dir.
- The local CA private key (`certs\ca.key`) stays on the machine and is never uploaded. Anyone who can read it can mint certificates your machine trusts, so keep the data directory private to your user. Uninstall removes the CA from the trust store.
- Downloaded runtimes are verified by SHA-256 where a hash is published; the installer payload records URLs and hashes in `versions.json`.
- Builds are currently unsigned (SmartScreen warning).

## Cross-platform plan (macOS)

OS-specific code is isolated in `*_windows.go` / `*_darwin.go` files with `//go:build !windows` fallbacks, no cgo, so `GOOS=darwin go build ./...` compiles today. Planned macOS differences:

- Data dir `~/Library/Application Support/Apnoro`; `.dmg` packaging and signing.
- Hosts changes via a privileged helper (launchd daemon) or resolver file instead of the Windows service.
- Trust via `security add-trusted-cert` into the keychain.
- Runtimes from macOS builds of Apache/PHP/MySQL rather than Windows zips; fcgi wrapper for PHP.
- Tray as a menu bar item (Wails).

## Related docs

[CONTRACTS.md](CONTRACTS.md) (package APIs and ownership), [BUILDING.md](BUILDING.md), [design mockups](design).
