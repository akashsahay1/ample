<#
.SYNOPSIS
  One-command Apnoro build: Go binaries -> Wails GUI -> runtime payload -> Inno Setup installer.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Version 1.0.0
  powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -SkipGui -SkipInstaller
#>
[CmdletBinding()]
param(
    [string]$Version = '',
    # build number; defaults to the git commit count (monotonic, reproducible)
    [string]$Build = '',
    # do not copy the installer into website/uploads or update the site data
    [switch]$SkipSite,
    [switch]$SkipGui,
    [switch]$SkipInstaller,
    # passed to ISCC as /DCompression=... (e.g. 'none' or 'lzma2/fast' for quick test builds)
    [string]$Compression = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Root = Split-Path -Parent $PSScriptRoot
$Dist = Join-Path $Root 'dist'
$App  = Join-Path $Dist 'app'
$Bin  = Join-Path $App 'bin'

function Write-Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Invoke-Checked([string]$Exe, [string[]]$ArgList) {
    Write-Host "    $Exe $($ArgList -join ' ')" -ForegroundColor DarkGray
    & $Exe @ArgList
    if ($LASTEXITCODE -ne 0) { throw "$Exe failed with exit code $LASTEXITCODE" }
}
function Find-Tool([string]$Name, [string[]]$Candidates) {
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($c in $Candidates) { if ($c -and (Test-Path $c)) { return $c } }
    throw "$Name not found (looked on PATH and in: $($Candidates -join '; '))"
}

# ---------------------------------------------------------------- version
if (-not $Version) {
    $wails = Get-Content (Join-Path $Root 'wails.json') -Raw | ConvertFrom-Json
    $Version = $wails.info.productVersion
}
if (-not $Version) { $Version = '0.0.0-dev' }
if (-not $Build) {
    $Build = (& git -C $Root rev-list --count HEAD 2>$null)
    if (-not $Build) { $Build = '0' }
}
$Build = "$Build".Trim()
Write-Host "Apnoro $Version build $Build" -ForegroundColor Green
$LdVars = "-X main.version=$Version -X main.build=$Build"

$Go = Find-Tool 'go' @('C:\Program Files\Go\bin\go.exe')
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'

New-Item -ItemType Directory -Force $Bin | Out-Null

# ---------------------------------------------------------------- (a) Go binaries
Write-Step 'Go binaries'
$targets = @(
    @{ Pkg = 'cmd/apnoro';        Out = 'apnoro.exe' },
    @{ Pkg = 'cmd/php-shim';     Out = 'php.exe'; Dir = 'shims' },
    @{ Pkg = 'cmd/apnoro-helper'; Out = 'apnoro-helper.exe' }
)
Push-Location $Root
try {
    foreach ($t in $targets) {
        $dir = Join-Path $Root $t.Pkg
        if (-not (Test-Path (Join-Path $dir '*.go'))) { throw "missing Go package ./$($t.Pkg) (no .go files in $dir)" }
        $outDir = if ($t.ContainsKey('Dir')) { Join-Path $App $t.Dir } else { $Bin }
        New-Item -ItemType Directory -Force $outDir | Out-Null
        Invoke-Checked $Go @('build', '-trimpath', '-ldflags', "-s -w $LdVars", '-o', (Join-Path $outDir $t.Out), "./$($t.Pkg)")
    }
} finally { Pop-Location }

# ---------------------------------------------------------------- (b) Wails GUI
if (-not $SkipGui) {
    Write-Step 'Wails GUI'
    $WailsExe = Find-Tool 'wails' @((Join-Path $env:USERPROFILE 'go\bin\wails.exe'))
    Push-Location $Root
    try {
        Invoke-Checked $WailsExe @('build', '-clean', '-platform', 'windows/amd64', '-ldflags', $LdVars)
    } finally { Pop-Location }
    Copy-Item -Force (Join-Path $Root 'build\bin\Apnoro.exe') (Join-Path $App 'Apnoro.exe')
} elseif (-not (Test-Path (Join-Path $App 'Apnoro.exe'))) {
    Write-Warning 'SkipGui: dist\app\Apnoro.exe does not exist yet (installer compile will fail)'
}

# ---------------------------------------------------------------- (d) runtime payload
if (-not (Test-Path (Join-Path $Dist 'payload\versions.json')) -or -not (Test-Path (Join-Path $Dist 'bin-extra\composer.phar'))) {
    Write-Step 'Runtime payload (fetch-runtimes.ps1)'
    & (Join-Path $PSScriptRoot 'fetch-runtimes.ps1')
}

# ---------------------------------------------------------------- (c) composer
Write-Step 'Composer'
$Shims = Join-Path $App 'shims'
New-Item -ItemType Directory -Force $Shims | Out-Null
Copy-Item -Force (Join-Path $Dist 'bin-extra\composer.phar') $Shims
Copy-Item -Force (Join-Path $Dist 'bin-extra\composer.bat') $Shims
# earlier layouts shipped these in bin\
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $Bin 'php.exe'), (Join-Path $Bin 'composer.phar'), (Join-Path $Bin 'composer.bat')

# ---------------------------------------------------------------- (e) installer
if (-not $SkipInstaller) {
    Write-Step 'Inno Setup installer'
    $Iscc = Find-Tool 'ISCC' @(
        (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'))
    $isccArgs = @("/DAppVersion=$Version", "/DAppBuild=$Build")
    if ($Compression) { $isccArgs += "/DCompression=$Compression" }
    $isccArgs += (Join-Path $Root 'installer\apnoro.iss')
    Invoke-Checked $Iscc $isccArgs
    $out = Join-Path $Dist "Apnoro-Setup-$Version.exe"
    Write-Host ("    {0} ({1:N0} MB)" -f $out, ((Get-Item $out).Length / 1MB)) -ForegroundColor Green

    # ------------------------------------------------------------ (f) website
    if (-not $SkipSite) {
        Write-Step 'Website (website/uploads + release data)'
        & (Join-Path $PSScriptRoot 'publish-site.ps1') -Version $Version -Build $Build -Installer $out
    }
}

Write-Step 'Done'
