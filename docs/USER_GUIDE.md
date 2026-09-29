# AMPLS user guide

## Contents
- [Concepts](#concepts)
- [The app](#the-app): Dashboard, Sites, New project, PHP Versions, MySQL, Logs, Settings, Tray
- [How the `php` command picks a version](#how-the-php-command-picks-a-version)
- [HTTPS and trust](#https-and-trust)
- [Data directory layout](#data-directory-layout)
- [Upgrading and uninstalling](#upgrading-and-uninstalling)

## Concepts

- **Services**: Apache (ports 80/443) and MySQL (port 3306). They run as background processes and keep running after you close the app or CLI, unless you enable "Stop services on quit".
- **Parked directory**: a folder whose subfolders are all served as sites. `~\AMPLS\Sites\blog` becomes `http://blog.test`. Sites with a `public\` folder (Laravel, Symfony) or `web\` folder are served from there.
- **Linked site**: a single folder outside any parked directory, served under a name you choose (`ampls link`).
- **Default PHP** vs **isolated site**: sites follow the default PHP version unless pinned to another one.
- **Data directory**: where runtimes, databases, configs, certificates and logs live (chosen at install, default `C:\AMPLS`).

## The app

Sidebar: Dashboard, Sites, PHP Versions, MySQL, Logs, Settings.

### Dashboard
Shows the state of Apache and MySQL (running/stopped, version, ports), the default PHP version, a short list of sites, and quick actions: **Open Sites folder** and **New project**. Use the start/stop/restart controls to manage services. A warning appears when the local CA is not trusted yet.

### Sites
Lists every site with its framework (Laravel, WordPress or PHP), path, PHP version and an HTTPS toggle. A search box filters the list.

- **Park directory**: add a folder; all its subfolders become `<folder>.test`. Remove a parked folder to stop serving it (your files are never touched).
- **PHP** dropdown per site: choose a version to isolate the site, or "default" to follow the default again. Apache is restarted to apply it.
- **HTTPS toggle**: issues a certificate for the site and serves it on `https://<site>.test`.
- Click a site's domain to open it in your browser. Linked sites can be removed (unlinked).

The first time a new site is created AMPLS asks the helper service to add it to the hosts file. If the helper is not running you will get a UAC prompt instead.

### New project
Started from **New project** (Sites or Dashboard).

| Type | What happens |
|---|---|
| **Laravel** | Runs `composer create-project laravel/laravel <name>` with the chosen PHP version, sets `DB_*` and `APP_URL` in `.env` |
| **WordPress** | Downloads the latest WordPress, extracts it, writes `wp-config.php` with the database and fresh salts |
| **Blank PHP** | Creates `index.php` with a phpinfo link and a README |

Options: project name (lowercase letters, digits and dashes), parent directory (normally a parked folder), PHP version, and **Create MySQL database** (a database named after the project, wired into the project's config). Progress is shown live; when it finishes the site is ready at `http://<name>.test`. CLI equivalent: `ampls new laravel shop --db`.

### PHP Versions
- **Installed** versions with full version, site count, and **Remove** (a version still in use cannot be removed safely; isolate those sites elsewhere first). One version is the default (`ampls php:use 8.3`).
- **Available** versions come from windows.php.net (NTS x64). Click **Install** to download side by side. Versions past end of life are flagged EOL.
- **Settings** for the selected version: `memory_limit`, `upload_max_filesize`, `post_max_size`, `max_execution_time`, `display_errors`. The **Extensions** list toggles every `php_*.dll` found in that version's `ext\` folder (xdebug is loaded as a `zend_extension`). Saving edits `php.ini` in place and keeps your comments; restart Apache to apply to web requests.
- **Edit php.ini** opens the file (`ampls php:ini 8.3` prints its path). php.ini is generated on first use with sane defaults: absolute `extension_dir`, common extensions enabled, errors logged to `logs\php-<minor>-error.log`, `date.timezone=UTC`.

### MySQL
- **Connection** card: host `127.0.0.1`, port `3306`, user `root`, password (empty by default), plus a copyable `.env` snippet (`DB_CONNECTION=mysql`, ...). You can set a root password here; phpMyAdmin and AMPLS use the stored value.
- **Databases** table with tables count and size. **New database**, **Import .sql**, and per-database export and drop. CLI: `ampls db:list|db:create|db:drop|db:import|db:export`.
- **phpMyAdmin** is at `http://localhost/phpmyadmin` (log in as `root`).
- MySQL data is in `<data dir>\data\mysql`.

### Logs
Pick a log (Apache error, Apache access, MySQL, `php-<minor>`) and view the most recent lines. CLI: `ampls logs apache-error -n 200`. Files are in `<data dir>\logs`.

### Settings
| Setting | Meaning |
|---|---|
| TLD | Domain suffix for sites (default `test`) |
| HTTP / HTTPS / MySQL ports | Defaults 80 / 443 / 3306; change if they conflict with other software |
| Parked directories | The list managed on the Sites screen |
| Start services on launch | Start Apache and MySQL when the app opens (default on) |
| Stop services on quit | Stop them when the app quits (default on) |
| Launch at login | Start AMPLS with Windows |
| Data directory | Read-only display |

### Tray
The tray icon shows overall state: running, stopped or error. Use it to show the window, start/stop services, and quit. Closing the window leaves the app in the tray.

## How the `php` command picks a version

The installer (optional task "Add ampls, php and composer to PATH") puts a small `php.exe` shim first in your PATH. Each time you run `php` (or `composer`, which calls it) the shim decides the version in this order:

1. The nearest `.ampls-php` file walking up from the current folder (contents like `8.3`).
2. The site that contains the current folder, if it is isolated to a version.
3. The default PHP version.
4. If none is set, the newest installed version.

Check with `ampls which-php`, which prints the version, the source of the decision and the binary. To pin a whole repo regardless of the site, create `.ampls-php` in its root:

```powershell
echo 8.2 > .ampls-php
```

## HTTPS and trust

`ampls secure` (or the HTTPS toggle) creates a certificate for `<site>.test` and `*.<site>.test`, signed by a local CA called **AMPLS Local CA**. The CA private key never leaves your data directory (`certs\ca.key`). For browsers to accept the certificates the CA must be trusted: `ampls trust` (or the dashboard prompt) adds it to your Windows trusted roots; the installer task "Trust the AMPLS local HTTPS certificate" does it machine-wide. Site certificates last 825 days, the CA 10 years. Firefox needs an extra step, see [Troubleshooting](TROUBLESHOOTING.md#https-shows-a-warning).

## Data directory layout

```
<data dir>\                 (default C:\AMPLS)
  apache\                   Apache httpd (incl. mod_fcgid)
  php\<minor>\              one folder per PHP version (php.exe, php-cgi.exe, ext\, php.ini)
  mysql\                    MySQL binaries
  data\mysql\               MySQL databases
  conf\                     generated: httpd.conf, sites\*.conf, my.ini
  certs\                    ca.crt, ca.key, sites\<domain>.crt/.key
  logs\                     apache-error.log, apache-access.log, mysql.log, php-<minor>-error.log
  run\                      pid files, hosts request/applied files
  apps\phpmyadmin\          phpMyAdmin
  www\                      default localhost page
  config.json               AMPLS settings, parked folders, per-site overrides
```

Program files (`AMPLS.exe`, `bin\ampls.exe`, `bin\php.exe`, `bin\composer.*`, `bin\ampls-helper.exe`) live in the install folder (default `C:\Program Files\AMPLS`). Files under `conf\` are regenerated, so edit settings through AMPLS, not by hand. Your projects live wherever you keep them (by default `%USERPROFILE%\AMPLS\Sites`).

## Upgrading and uninstalling

**Upgrade**: run the newer `AMPLS-Setup-x.y.z.exe`. It stops services, replaces programs and runtimes, and pre-selects your existing data directory. Databases, site config, certificates and `php.ini` files are not overwritten.

**Uninstall** (Settings > Apps): services are stopped, the AMPLS hosts entries are removed, the helper service and the AMPLS CA trust entry are removed, and PATH is cleaned. You are then asked whether to **also delete the data directory**. Choose **No** (the default) to keep databases and configuration for a reinstall; choose **Yes** to delete everything in it. Your project folders are never touched.

Silent installs can set the data directory: `AMPLS-Setup-1.0.0.exe /DATADIR=D:\AMPLS`.
