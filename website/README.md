# AMPLS website

A static one-page site: no build step, no server code. Upload this whole folder to any web host.

```
website/
  index.html            the page
  assets/               CSS, JS and the logo (logos come from scripts/icon-tool)
  favicon.ico
  releases.json         release list: version, build, date, file, size, sha256, components
  releases.js           the same data for the page (works from file:// and any host)
  uploads/<version>/AMPLS-Setup-<version>.exe
```

## Publishing a release

`scripts\build.ps1 -Version X.Y.Z` builds the installer and then runs
`scripts\publish-site.ps1`, which:

1. copies the installer to `uploads/X.Y.Z/`,
2. records version, **build number** (git commit count), date, size, SHA-256 and the
   bundled component versions in `releases.json` / `releases.js` (newest first).

Then upload `website/` to the host. The page shows the latest release at the top and
every release in the table.

Installers are **not committed** (`website/uploads/**/*.exe` is git-ignored); only
`releases.json` / `releases.js` are, so the repository stays small.

To register an installer built elsewhere:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\publish-site.ps1 -Version 1.0.0 -Build 19 -Installer dist\AMPLS-Setup-1.0.0.exe
```
