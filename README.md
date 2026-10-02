# Apnoro

**Apache, PHP and MySQL for Windows: one install, every PHP version.**

Apnoro is a local PHP development environment for Windows (macOS planned). It bundles Apache, MySQL 8.4 and PHP like XAMPP does, then adds the conveniences of Laravel Herd: drop a folder into a parked directory and it is served at `http://<folder>.test`, pick a PHP version per project, turn on HTTPS with one click, and manage everything from a desktop app with a tray icon or from the `apnoro` command line.

## Features

- **Bundled stack.** Apache 2.4 (Apache Lounge build), MySQL 8.4 LTS, PHP 8.5 and phpMyAdmin ship in the installer.
- **Parked folders.** Every subfolder of a parked directory is served automatically as `<folder>.test`. The installer can create and park `%USERPROFILE%\Apnoro\Sites` for you.
- **Per-project PHP versions.** Install PHP versions side by side and pin any site to one (`apnoro isolate 8.3`). Apache runs each site through mod_fcgid with that version's `php-cgi.exe`.
- **`php` shim.** The `php` command on your PATH picks the right version for the folder you are in (`.apnoro-php` file, then site isolation, then the default). `composer` is included.
- **One-click HTTPS.** A local certificate authority signs per-site certificates (`apnoro secure`); `apnoro trust` adds the CA to your Windows trust store.
- **New project wizard.** Create Laravel, WordPress or blank PHP projects, optionally with a matching MySQL database already wired into `.env` / `wp-config.php`.
- **PHP settings UI.** Edit `memory_limit`, `upload_max_filesize`, `post_max_size`, `max_execution_time`, `display_errors`, toggle extensions, or open `php.ini`.
- **MySQL tools.** Create, drop, import and export databases from the app or CLI; phpMyAdmin is at `http://localhost/phpmyadmin`.
- **Logs viewer.** Apache, MySQL and per-PHP-version logs in the app and via `apnoro logs`.
- **Desktop app and tray.** Wails app with Dashboard, Sites, PHP Versions, MySQL, Logs and Settings screens, and a tray icon showing running / stopped / error.
- **You choose the data directory.** The installer asks where to keep runtimes, databases and configs (default `C:\Apnoro`).

## Screenshots

Screenshots are not published yet. The approved UI mockups are in [`docs/design`](docs/design) (open the `.dc.html` files in a browser): Dashboard (`Main`), `Sites`, `Php`, `Mysql`, `Installer` and `Icon`.

## Install

1. Download `Apnoro-Setup-x.y.z.exe` from the project's releases page.
2. Run it. Choose the install location and the **data directory** (default `C:\Apnoro`; no spaces in the path, and it must be separate from the program folder).
3. Pick the optional tasks: create and park the Sites folder, add `apnoro`/`php`/`composer` to PATH, trust the local HTTPS certificate, start Apnoro at Windows startup, desktop icon.
4. Launch Apnoro. Services start automatically by default.

> **SmartScreen:** unsigned builds show "Windows protected your PC". Click **More info**, then **Run anyway**.

The installer needs administrator rights (it installs the hosts helper service and, if needed, the Microsoft Visual C++ runtime).

## Quick start

```powershell
# 1. Drop a folder into the parked Sites folder
mkdir $env:USERPROFILE\Apnoro\Sites\hello
echo "<?php phpinfo();" > $env:USERPROFILE\Apnoro\Sites\hello\index.php
# -> open http://hello.test

# 2. Run that site on a specific PHP version (from inside the project folder)
cd $env:USERPROFILE\Apnoro\Sites\hello
apnoro php:install 8.3
apnoro isolate 8.3

# 3. Serve it over HTTPS
apnoro secure          # -> https://hello.test
apnoro trust           # once, if the CA is not trusted yet
```

Or create a project: `apnoro new laravel shop --db`.

## CLI reference

Run `apnoro <command> --help` for details. Global flag: `--home <dir>` overrides the data directory.

| Command | What it does |
|---|---|
| `apnoro start` / `stop` / `restart` `[apache\|mysql]` | Control services (detached; they keep running after the command exits) |
| `apnoro status` | Service state, PIDs, versions, ports, default PHP, CA trust, data dir |
| `apnoro sites` | List all sites |
| `apnoro park [dir]` / `unpark [dir]` / `parked` | Serve every folder in a directory as `<folder>.test`; stop; list |
| `apnoro link [name]` / `unlink [name]` | Serve the current directory as `<name>.test`; remove |
| `apnoro isolate <version> [--site name]` | Pin a site to a PHP version |
| `apnoro unisolate [site]` | Follow the default PHP version again |
| `apnoro secure [site]` / `unsecure [site]` | HTTPS on/off (defaults to the site for the current directory) |
| `apnoro open [site]` | Open a site in the browser |
| `apnoro new <laravel\|wordpress\|blank> <name>` | Create a project (`--dir`, `--php`, `--db`) |
| `apnoro php:list` | Installed and available PHP versions |
| `apnoro php:install <version>` / `php:remove <version>` | Download and install / uninstall a PHP version |
| `apnoro php:use <version>` | Set the default PHP version |
| `apnoro php:ini [version]` | Print the path of a version's `php.ini` |
| `apnoro which-php` | Which PHP version the `php` command uses here, and why |
| `apnoro db:list` / `db:create <name>` / `db:drop <name>` (`-f`) | Manage MySQL databases |
| `apnoro db:import <db> <file.sql>` / `db:export <db> <file.sql>` | Import / export SQL |
| `apnoro logs [name] [-n 100]` | Show a log (no name lists them) |
| `apnoro trust` | Trust the Apnoro certificate authority |
| `apnoro version` | Print version and data directory |

Also present: `apnoro setup` (run by the installer) and `apnoro hosts apply|clear|list` (manage the Apnoro block of the hosts file).

## Default credentials

MySQL: user `root`, **empty password**, host `127.0.0.1`, port `3306`. MySQL binds to loopback only, so it is not reachable from other machines. You can change the root password from the MySQL screen in the app.

## Documentation

- [User guide](docs/USER_GUIDE.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Building from source](docs/BUILDING.md)
- [Changelog](CHANGELOG.md) · [Third-party notices](THIRD_PARTY_NOTICES.md)

## Roadmap

- macOS `.dmg` (signed build)
- Node version manager
- Mail catcher
- Redis
- Nginx option
- Auto-update

## License

Apnoro is released under the [MIT License](LICENSE) © 2026 Akash Sahay. Bundled components keep their own licenses; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
