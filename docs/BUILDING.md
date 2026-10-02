# Building Apnoro

## Prerequisites (Windows 11 x64)

| Tool | Version | Notes |
|---|---|---|
| Go | 1.25+ | `C:\Program Files\Go\bin` on PATH |
| Wails CLI | v2 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` → `%USERPROFILE%\go\bin\wails.exe` |
| Node.js + npm | 20+ | used by Wails for `frontend/` and by the icon generator |
| Inno Setup | 6.3+ | `ISCC.exe` in `Program Files (x86)\Inno Setup 6` or `%LOCALAPPDATA%\Programs\Inno Setup 6` |
| WebView2 runtime | any | preinstalled on Windows 11 |

The build also uses Windows' built-in `curl.exe` and `tar.exe`, plus network access for the first
runtime download.

## One-command build

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Version 1.0.0
```

Output: `dist\Apnoro-Setup-1.0.0.exe`. `-Version` defaults to `info.productVersion` in `wails.json`.

| Switch | Effect |
|---|---|
| `-SkipGui` | skip `wails build` (reuse `dist\app\Apnoro.exe`) |
| `-SkipInstaller` | stop after assembling `dist\app` |
| `-Compression none` | fast test installer (default `lzma2/ultra64` takes a few minutes) |

Steps it runs:
1. `go build` (`CGO_ENABLED=0`, `-ldflags "-s -w -X main.version=<ver>"`):
   `cmd/apnoro` → `dist\app\bin\apnoro.exe`, `cmd/php-shim` → `dist\app\bin\php.exe`,
   `cmd/apnoro-helper` → `dist\app\bin\apnoro-helper.exe`
2. `wails build -clean -platform windows/amd64 -ldflags "-X main.version=<ver>"` →
   `build\bin\Apnoro.exe` → `dist\app\Apnoro.exe`
3. `scripts\fetch-runtimes.ps1` if `dist\payload\versions.json` is missing
4. copies `composer.phar` + `composer.bat` into `dist\app\bin`
5. `ISCC installer\apnoro.iss /DAppVersion=<ver>`

`dist\` is build output and git-ignored.

## Runtime payload (`scripts\fetch-runtimes.ps1`)

Downloads to `dist\cache\` (reused on later runs; `-Force` downloads again) and assembles
`dist\payload\` in the data-directory layout from `docs\CONTRACTS.md`:

```
payload\apache\          Apache Lounge httpd 2.4 (Apache24\ contents, manual\ removed) + modules\mod_fcgid.so
payload\php\<minor>\     newest stable PHP NTS x64 from windows.php.net releases.json (sha256 verified)
payload\php\cacert.pem   curl CA bundle
payload\mysql\           MySQL 8.4 LTS winx64 (trimmed: no mysql-test/include/docs/pdb/.lib/mecab dictionaries)
payload\apps\phpmyadmin\ phpMyAdmin (english) + config.inc.php (cookie auth, 127.0.0.1, random blowfish secret,
                         port read from <data dir>\config.json -> ports.mysql)
payload\LICENSES\        upstream licenses
payload\versions.json    versions, source URLs and sha256 of every download
dist\bin-extra\          composer.phar, composer.bat
dist\cache\vc_redist.x64.exe
```

Versions are discovered dynamically:
- Apache and mod_fcgid: the first `httpd-2.4.*-Win64-VS*.zip` / `mod_fcgid-*-win64-VS*.zip` link on
  https://www.apachelounge.com/download/ (currently VS18 builds).
- PHP: highest minor in https://windows.php.net/downloads/releases/releases.json (`-PhpMinor 8.4` pins one).
- MySQL: probes `https://cdn.mysql.com/Downloads/MySQL-8.4/mysql-8.4.<N>-winx64.zip` from N=40 downwards
  (`-MySqlPatch 11` pins one).
- phpMyAdmin: https://www.phpmyadmin.net/home_page/version.json.
- VC++ runtime: https://aka.ms/vs/18/release/vc_redist.x64.exe (the "v14" redistributable for VS 2017–2026,
  which covers the VS17 PHP builds and VS18 Apache builds). The installer only runs it when the installed
  runtime (`HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`) is older than the bundled one;
  the bundled version is read from the exe at compile time.

To update runtimes: delete `dist\cache\<file>` (or pass `-Force`) and run the script again.

## Icons (`scripts\icon-tool`)

The source SVGs are in `assets\icon\` (`apnoro.svg`, the simplified `apnoro-32.svg` and `apnoro-16.svg`,
and `tray-{running,stopped,error}.svg`), taken from the approved `docs\design\Icon.dc.html`.

```powershell
cd scripts\icon-tool; npm install; node generate.mjs
```

This regenerates `assets\icons\`: `app-{16..1024}.png` (16 uses the 16px drawing, 24/32 use the 32px
drawing), `app.ico` (16–256), `app.icns`, `tray-*.ico` (16/20/24/32) plus `tray-*-32.png`, and the
Inno Setup wizard BMPs `installer-wizard-{large,small}-{100,125,150,175,200}.bmp`. It also copies
`build\appicon.png` and `build\windows\icon.ico`, which Wails uses.

## Bumping the version

1. Set `info.productVersion` in `wails.json` (this sets the exe file version).
2. Run `scripts\build.ps1 -Version X.Y.Z` (this sets `main.version` in every binary, `AppVersion` in
   the installer and the output file name).

The installer `AppId` GUID in `installer\apnoro.iss` must never change, so upgrades install in place.
Upgrades keep the data folder chosen earlier, never overwrite `apps\phpmyadmin\config.inc.php`, and only
ever ship runtimes. `data\`, `conf\`, `certs\`, `config.json` and `php.ini` are created by `apnoro setup`.

## Code signing and SmartScreen

The installer and exes are unsigned for now, so Windows SmartScreen shows "Windows protected your PC"
until the file builds up reputation. Users have to click **More info → Run anyway**. For public releases,
get an OV/EV code-signing certificate (or use Azure Trusted Signing) and sign `Apnoro.exe`, `bin\*.exe`
and the setup:
- In Inno Setup, add `SignTool=...` to `[Setup]`, with `signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 ...`.
- Sign the binaries in `dist\app` before running ISCC.

## macOS (future)

- GUI: `wails build -platform darwin/universal -ldflags "-X main.version=X.Y.Z"` → `build/bin/Apnoro.app`.
  Wails uses `build/appicon.png` to make the `.icns`; `assets/icons/app.icns` is also generated.
- CLI and helper: `GOOS=darwin GOARCH=arm64` and `amd64` builds, joined with `lipo -create`, placed in
  `Apnoro.app/Contents/Resources/bin` (or installed to `/usr/local/bin` by a `.pkg`).
- Runtimes: there are no official zip builds of httpd or PHP for macOS. Options are a pinned Homebrew
  bottle set, static-php-cli builds of PHP, and MySQL's macOS tar.gz. A `fetch-runtimes-macos.sh` would
  mirror the Windows script.
- Signing with the Apple Developer ID:
  `codesign --deep --force --options runtime --timestamp --sign "Developer ID Application: <Name> (<TEAMID>)" Apnoro.app`
  (sign every bundled Mach-O binary first, with hardened runtime and entitlements).
- Notarize: `xcrun notarytool submit Apnoro.dmg --apple-id ... --team-id ... --password <app-specific> --wait`,
  then `xcrun stapler staple Apnoro.dmg`.
- DMG: `create-dmg --volname Apnoro --app-drop-link 480 170 Apnoro-X.Y.Z.dmg build/bin/Apnoro.app`.
