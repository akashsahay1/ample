# Changelog

All notable changes to AMPLS are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-10-01

First release (Windows x64).

### Added
- Bundled stack: Apache 2.4.68 (Apache Lounge, with mod_fcgid 2.3.10), PHP 8.5.11 (NTS x64), MySQL 8.4.11 LTS, phpMyAdmin 5.2.3, Composer 2.10.3.
- Inno Setup installer `AMPLS-Setup-x.y.z.exe` with a data-directory page (default `C:\AMPLS`), optional tasks (park Sites folder, PATH, trust HTTPS certificate, start at Windows startup, desktop icon) and a Visual C++ runtime check. Upgrades keep your data; uninstall asks before deleting the data directory.
- Parked directories: every subfolder served as `<folder>.test`; `%USERPROFILE%\AMPLS\Sites` created and parked on request. Linked single-folder sites.
- Per-site PHP versions via Apache mod_fcgid; install, remove and set the default PHP version; edit common `php.ini` values and extensions.
- `php` shim that resolves the version from a `.ampls-php` file, site isolation, or the default; bundled `composer`.
- One-click HTTPS with a local certificate authority ("AMPLS Local CA") and per-site certificates.
- `AMPLSHelper` Windows service that applies validated `.test` entries to the hosts file (UAC-elevated fallback if unavailable).
- New project wizard for Laravel, WordPress and blank PHP, with optional database creation.
- MySQL management: create, drop, import, export databases; root password change; phpMyAdmin at `http://localhost/phpmyadmin`.
- `ampls` CLI: services, sites, PHP versions, databases, logs, trust, project scaffolding.
- Wails desktop app (Dashboard, Sites, PHP Versions, MySQL, Import, Logs, Settings) with tray icon and running/stopped/error states.
- Coexistence with XAMPP, Laravel Herd, Laragon and WAMP: detects them, names the program holding a port AMPLS needs, and can stop it on request (only processes inside that tool's own folder).
- Import from XAMPP (htdocs, vhosts, MariaDB databases), Laravel Herd (parked folders, links, PHP pins, HTTPS) and any MySQL/MariaDB server. Projects are served in place; the source stack is never changed. CLI: `ampls env`, `ampls env:stop`, `ampls import`.
- `php`/`composer` shims in a separate `shims` folder; when another PHP is already on PATH the installer keeps it first by default.
- Choose the MySQL root password during install (empty = none), and a per-project database name in New project (`ampls new --db-name`).

### Security
- Data directory is owner-only; setup runs unelevated; the hosts helper only writes validated `.test`/`.localhost` entries; the local CA is name-constrained to `.test`, `localhost` and loopback; a deny-all default vhost blocks DNS-rebinding access to phpMyAdmin.

### Known limitations
- Builds are unsigned; Windows SmartScreen shows a warning on first run.
- Windows only; macOS support is planned.
