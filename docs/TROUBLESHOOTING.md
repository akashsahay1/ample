# Troubleshooting

Start with `apnoro status` and `apnoro logs`. Logs are in `<data dir>\logs` (default `C:\Apnoro\logs`).

## Port 80, 443 or 3306 is already in use

Apache needs 80 and 443, MySQL needs 3306. Find the owner:

```powershell
netstat -ano | findstr :80
netstat -ano | findstr :443
netstat -ano | findstr :3306
tasklist /fi "PID eq <pid>"
```

Common culprits:
- **IIS / World Wide Web Publishing Service (W3SVC)**: `net stop w3svc` and set the service to Manual or Disabled (`sc config w3svc start= disabled`). Also check "HTTP.sys" users with `netsh http show servicestate`.
- **Skype** or other apps using 80/443 (recent Skype versions no longer do).
- **XAMPP, Laragon, WAMP or another MySQL**: stop them first. A second MySQL service (`services.msc`) blocks 3306.
- Anything else in the `netstat` output: quit it, or change Apnoro ports under **Settings > Ports** (then use `http://site.test:8080`).

## `.test` sites do not resolve

1. Confirm the entry exists: `apnoro hosts list`, or open `C:\Windows\System32\drivers\etc\hosts` and look for the `# BEGIN APNORO` ... `# END APNORO` block. Do not edit inside it.
2. Check the helper service: `sc query ApnoroHelper` should say RUNNING. Start it with `sc start ApnoroHelper` (admin). Without it Apnoro falls back to a UAC prompt.
3. Apply pending entries manually (admin terminal): `apnoro hosts apply`.
4. Flush the DNS cache: `ipconfig /flushdns`.
5. **Browser secure DNS bypasses the hosts file.** Chrome/Edge: Settings > Privacy > Security > turn off "Use secure DNS" (or choose your OS resolver). Firefox: Settings > Privacy & Security > DNS over HTTPS > Off, or add `test` to its exceptions.
6. A VPN or security tool that rewrites the hosts file may remove the block; re-run `apnoro hosts apply`.
7. Test outside the browser: `ping blog.test` should answer from 127.0.0.1.

## HTTPS shows a warning

- Run `apnoro trust` and restart the browser. `apnoro status` shows "HTTPS CA: trusted" when it worked.
- **Firefox uses its own certificate store.** Open `about:config`, set `security.enterprise_roots.enabled` to `true`, restart Firefox. (Or import `<data dir>\certs\ca.crt` under Settings > Privacy & Security > Certificates > View Certificates > Authorities.)
- Make sure the site is secured (`apnoro secure <site>`) and Apache was restarted (`apnoro restart apache`).
- Certificates only cover `<site>.test` and `*.<site>.test`. Other hostnames will warn.

## Apache will not start

- Check `logs\apache-error.log`. Test the generated config: `<data dir>\apache\bin\httpd.exe -t -f <data dir>\conf\httpd.conf`.
- Port conflict: see above.
- A page shows "Service Unavailable" or 500 for PHP: check `logs\php-<minor>-error.log`; make sure that PHP version is still installed (`apnoro php:list`).

## MySQL will not start

- Read `logs\mysql.log` first.
- Make sure `<data dir>\data\mysql` is writable by your user (the installer grants Users modify rights on the data directory). Restore with `icacls "<data dir>" /grant Users:(OI)(CI)M /T` from an admin prompt.
- A leftover process holding the datadir: end `mysqld.exe` in Task Manager, delete `<data dir>\run\mysql.pid`, retry.
- Another MySQL is using port 3306: change the port in Settings or stop the other server.
- Do not copy datadirs between MySQL versions.

## "VCRUNTIME140.dll / MSVCP140.dll was not found", or programs exit immediately

The Microsoft Visual C++ Redistributable (x64) is missing. The installer normally installs it. Install it manually from https://aka.ms/vs/17/release/vc_redist.x64.exe and restart Apnoro.

## Antivirus removes or blocks `php-cgi.exe`

Some antivirus tools quarantine `php-cgi.exe` or `mysqld.exe`. Symptom: 500 errors or a missing file in `php\<minor>\`. Restore the file from quarantine, add an exclusion for the whole data directory (and the install folder), then reinstall the PHP version (`apnoro php:remove 8.5`, `apnoro php:install 8.5`) if the file is gone.

## `php` runs the wrong version, or another PHP

```powershell
where php
apnoro which-php
```

- `where php` must list `...\Apnoro\bin\php.exe` **first**. If XAMPP, Laragon, Chocolatey or another PHP appears above it, move the Apnoro `bin` folder up in **System Properties > Environment Variables > Path** (system entries come before user entries), or remove the other entry. Open a new terminal afterwards.
- `apnoro which-php` shows the chosen version and its source (`.apnoro-php` file, site isolation, default). Fix with `apnoro isolate <v>`, `apnoro unisolate`, `apnoro php:use <v>`, or edit `.apnoro-php`.
- `apnoro: command not found`: the PATH task was not selected or the terminal predates install. Open a new terminal, or add `<install dir>\bin` to PATH.

## The site shows the wrong PHP version

Web requests use the version shown for the site in **Sites** (or `apnoro sites`), not the CLI version. After changing it, Apache is restarted; if not, run `apnoro restart apache`.

## Resetting

- **Regenerate configs**: `apnoro restart` rewrites `conf\` from your settings.
- **Reset hosts entries**: `apnoro hosts clear` (admin) then `apnoro hosts apply`.
- **Start clean but keep data**: `apnoro stop`, delete the contents of `<data dir>\run`, `apnoro start`.
- **Full reset**: `apnoro stop`, then delete `<data dir>\config.json`, `conf\`, `certs\` (and `data\mysql\` **only if you want to lose all databases**), then re-run `apnoro setup --home "<data dir>"`.
- **Reinstall**: uninstall and answer **No** to keep the data directory, or **Yes** for a clean slate.

## Still stuck?

Collect `apnoro status`, `apnoro version` and the relevant log tail (`apnoro logs apache-error -n 100`) and open an issue.
