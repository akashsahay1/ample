<#
.SYNOPSIS
  Downloads the Apnoro runtime payload (Apache, mod_fcgid, PHP, MySQL, phpMyAdmin, cacert,
  Composer, VC++ redist) into dist\cache and assembles dist\payload with the data-dir layout
  from docs\CONTRACTS.md.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\fetch-runtimes.ps1
  powershell -ExecutionPolicy Bypass -File scripts\fetch-runtimes.ps1 -Force   # re-download everything
#>
[CmdletBinding()]
param(
    [switch]$Force,
    # Override the PHP minor to bundle (default: newest stable minor from releases.json)
    [string]$PhpMinor = '',
    # Override the MySQL 8.4 patch release (default: probe cdn.mysql.com downwards)
    [int]$MySqlPatch = 0
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Root    = Split-Path -Parent $PSScriptRoot
$Dist    = Join-Path $Root 'dist'
$Cache   = Join-Path $Dist 'cache'
$Payload = Join-Path $Dist 'payload'
$BinX    = Join-Path $Dist 'bin-extra'
$Work    = Join-Path $Dist 'work'
$UA      = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36'
$Curl    = Join-Path $env:SystemRoot 'System32\curl.exe'
$Tar     = Join-Path $env:SystemRoot 'System32\tar.exe'

foreach ($d in $Dist, $Cache, $BinX) { New-Item -ItemType Directory -Force $d | Out-Null }

function Write-Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

function Get-Text([string]$Url) {
    $out = & $Curl -sSL --fail --retry 3 -A $UA $Url
    if ($LASTEXITCODE -ne 0) { throw "GET $Url failed (curl exit $LASTEXITCODE)" }
    return ($out -join "`n")
}

function Test-Url([string]$Url) {
    $code = & $Curl -sIL -o NUL -w '%{http_code}' -A $UA $Url
    return ($code -eq '200')
}

# Downloads $Url into dist\cache\$Name (skipped when cached unless -Force). Returns the path.
function Get-Cached([string]$Url, [string]$Name) {
    $dest = Join-Path $Cache $Name
    if ((Test-Path $dest) -and -not $Force) {
        Write-Host "    cached: $Name"
        return $dest
    }
    Write-Host "    downloading $Url"
    $tmp = "$dest.part"
    & $Curl -L --fail --retry 3 --retry-delay 2 -A $UA -o $tmp $Url
    if ($LASTEXITCODE -ne 0) { throw "download failed: $Url (curl exit $LASTEXITCODE)" }
    Move-Item -Force $tmp $dest
    return $dest
}

function Get-Sha256([string]$Path) { (Get-FileHash -Algorithm SHA256 $Path).Hash.ToLowerInvariant() }

function Expand-Zip([string]$Zip, [string]$Dest) {
    New-Item -ItemType Directory -Force $Dest | Out-Null
    & $Tar -xf $Zip -C $Dest
    if ($LASTEXITCODE -ne 0) { throw "extract failed: $Zip" }
}

# Moves the *contents* of $From into $To (created if needed).
function Move-Contents([string]$From, [string]$To) {
    New-Item -ItemType Directory -Force $To | Out-Null
    Get-ChildItem -Force $From | ForEach-Object { Move-Item -Force $_.FullName $To }
}

$versions = [ordered]@{ generated = (Get-Date).ToUniversalTime().ToString('o') }
function Add-Version([string]$Key, [string]$Version, [string]$Url, [string]$File) {
    $versions[$Key] = [ordered]@{ version = $Version; url = $Url; file = (Split-Path -Leaf $File); sha256 = (Get-Sha256 $File) }
}

# ---------------------------------------------------------------- resolve + download
Write-Step 'Apache Lounge (httpd + mod_fcgid)'
$al = Get-Text 'https://www.apachelounge.com/download/'
$httpdRel = [regex]::Matches($al, 'href="(/download/VS\d+/binaries/httpd-2\.4\.\d+-\d+-[Ww]in64-VS\d+\.zip)"') | ForEach-Object { $_.Groups[1].Value } | Select-Object -First 1
$fcgidRel = [regex]::Matches($al, 'href="(/download/VS\d+/modules/mod_fcgid-[\d.]+-win64-VS\d+\.zip)"') | ForEach-Object { $_.Groups[1].Value } | Select-Object -First 1
if (-not $httpdRel) { throw 'could not find httpd win64 zip on apachelounge.com/download' }
if (-not $fcgidRel) { throw 'could not find mod_fcgid win64 zip on apachelounge.com/download' }
$httpdUrl = "https://www.apachelounge.com$httpdRel"
$fcgidUrl = "https://www.apachelounge.com$fcgidRel"
$httpdZip = Get-Cached $httpdUrl (Split-Path -Leaf $httpdRel)
$fcgidZip = Get-Cached $fcgidUrl (Split-Path -Leaf $fcgidRel)
$httpdVer = ([regex]::Match($httpdRel, 'httpd-(2\.4\.\d+)')).Groups[1].Value
$fcgidVer = ([regex]::Match($fcgidRel, 'mod_fcgid-([\d.]+)-')).Groups[1].Value
Add-Version 'apache' $httpdVer $httpdUrl $httpdZip
Add-Version 'mod_fcgid' $fcgidVer $fcgidUrl $fcgidZip

Write-Step 'PHP (NTS x64)'
$relJson = Get-Text 'https://windows.php.net/downloads/releases/releases.json' | ConvertFrom-Json
$minors = $relJson.PSObject.Properties.Name | Sort-Object { [version]$_ } -Descending
if ($PhpMinor) { $minor = $PhpMinor } else { $minor = $minors[0] }
$rel = $relJson.$minor
if (-not $rel) { throw "PHP $minor not in releases.json" }
$ntsKey = $rel.PSObject.Properties.Name | Where-Object { $_ -like 'nts-v*-x64' } | Select-Object -First 1
$phpZipInfo = $rel.$ntsKey.zip
$phpUrl = "https://windows.php.net/downloads/releases/$($phpZipInfo.path)"
$phpZip = Get-Cached $phpUrl $phpZipInfo.path
if ((Get-Sha256 $phpZip) -ne $phpZipInfo.sha256.ToLowerInvariant()) { Remove-Item $phpZip; throw "PHP sha256 mismatch for $($phpZipInfo.path) (deleted; re-run)" }
Add-Version 'php' $rel.version $phpUrl $phpZip
$versions.php['minor'] = $minor

Write-Step 'MySQL 8.4 LTS'
if ($MySqlPatch -gt 0) { $cands = @($MySqlPatch) } else { $cands = 40..0 }
$mysqlUrl = $null
# Reuse a cached zip if present (saves the probe round-trips)
if (-not $Force -and $MySqlPatch -eq 0) {
    $cachedMy = Get-ChildItem $Cache -Filter 'mysql-8.4.*-winx64.zip' -ErrorAction SilentlyContinue | Sort-Object { [version]($_.Name -replace '^mysql-(8\.4\.\d+)-winx64\.zip$', '$1') } -Descending | Select-Object -First 1
    if ($cachedMy) { $mysqlUrl = "https://cdn.mysql.com/Downloads/MySQL-8.4/$($cachedMy.Name)" }
}
if (-not $mysqlUrl) {
    foreach ($n in $cands) {
        $u = "https://cdn.mysql.com/Downloads/MySQL-8.4/mysql-8.4.$n-winx64.zip"
        if (Test-Url $u) { $mysqlUrl = $u; break }
    }
}
if (-not $mysqlUrl) { throw 'could not find a MySQL 8.4.x winx64 zip on cdn.mysql.com' }
$mysqlZip = Get-Cached $mysqlUrl (Split-Path -Leaf $mysqlUrl)
Add-Version 'mysql' (([regex]::Match($mysqlUrl, 'mysql-(8\.4\.\d+)')).Groups[1].Value) $mysqlUrl $mysqlZip

Write-Step 'phpMyAdmin'
$pmaVer = (Get-Text 'https://www.phpmyadmin.net/home_page/version.json' | ConvertFrom-Json).version
$pmaUrl = "https://files.phpmyadmin.net/phpMyAdmin/$pmaVer/phpMyAdmin-$pmaVer-english.zip"
$pmaZip = Get-Cached $pmaUrl "phpMyAdmin-$pmaVer-english.zip"
Add-Version 'phpmyadmin' $pmaVer $pmaUrl $pmaZip

Write-Step 'cacert.pem, Composer, VC++ redistributable'
$cacertUrl = 'https://curl.se/ca/cacert.pem'
$cacert = Get-Cached $cacertUrl 'cacert.pem'
Add-Version 'cacert' ((Get-Item $cacert).LastWriteTimeUtc.ToString('yyyy-MM-dd')) $cacertUrl $cacert
$composerUrl = 'https://getcomposer.org/download/latest-stable/composer.phar'
$composer = Get-Cached $composerUrl 'composer.phar'
$composerVer = ''
try { $composerVer = (Get-Text 'https://getcomposer.org/versions' | ConvertFrom-Json).stable[0].version } catch { }
Add-Version 'composer' $composerVer $composerUrl $composer
# Apache Lounge now builds with VS18 (VS2026) -> ship the matching (backwards-compatible) v14 redist
$vcUrl = 'https://aka.ms/vs/18/release/vc_redist.x64.exe'
$vc = Get-Cached $vcUrl 'vc_redist.x64.exe'
Add-Version 'vc_redist' ((Get-Item $vc).VersionInfo.FileVersion) $vcUrl $vc

# ---------------------------------------------------------------- assemble payload
Write-Step "Assembling $Payload"
if (Test-Path $Payload) { Remove-Item -Recurse -Force $Payload }
if (Test-Path $Work) { Remove-Item -Recurse -Force $Work }
New-Item -ItemType Directory -Force $Payload, $Work | Out-Null
$Lic = Join-Path $Payload 'LICENSES'
New-Item -ItemType Directory -Force $Lic | Out-Null

# apache/
$w = Join-Path $Work 'httpd'; Expand-Zip $httpdZip $w
$a24 = Get-ChildItem $w -Directory -Recurse -Filter 'Apache24' | Select-Object -First 1
if (-not $a24) { throw 'Apache24 dir not found in httpd zip' }
$apache = Join-Path $Payload 'apache'
Move-Contents $a24.FullName $apache
Remove-Item -Recurse -Force (Join-Path $apache 'manual') -ErrorAction SilentlyContinue   # ~20 MB of HTML docs
Copy-Item (Join-Path $apache 'LICENSE.txt') (Join-Path $Lic 'Apache-httpd-LICENSE.txt')
$w = Join-Path $Work 'fcgid'; Expand-Zip $fcgidZip $w
$so = Get-ChildItem $w -Recurse -Filter 'mod_fcgid.so' | Select-Object -First 1
if (-not $so) { throw 'mod_fcgid.so not found in mod_fcgid zip' }
Copy-Item $so.FullName (Join-Path $apache 'modules\mod_fcgid.so')

# php/<minor>/ (zip has no top dir) + php/cacert.pem
$phpDir = Join-Path $Payload "php\$minor"
Expand-Zip $phpZip $phpDir
Copy-Item $cacert (Join-Path $Payload 'php\cacert.pem')
Copy-Item (Join-Path $phpDir 'license.txt') (Join-Path $Lic 'PHP-LICENSE.txt')

# mysql/ (top dir stripped, trimmed)
$w = Join-Path $Work 'mysql'; Expand-Zip $mysqlZip $w
$top = Get-ChildItem $w -Directory | Select-Object -First 1
$mysql = Join-Path $Payload 'mysql'
Move-Contents $top.FullName $mysql
foreach ($p in 'mysql-test', 'include', 'docs') { Remove-Item -Recurse -Force (Join-Path $mysql $p) -ErrorAction SilentlyContinue }
Get-ChildItem $mysql -Recurse -Include '*.pdb' -File | Remove-Item -Force
Get-ChildItem (Join-Path $mysql 'bin') -Filter '*debug*' -File -ErrorAction SilentlyContinue | Remove-Item -Force
Get-ChildItem (Join-Path $mysql 'lib') -Filter '*.lib' -File -ErrorAction SilentlyContinue | Remove-Item -Force
Remove-Item -Recurse -Force (Join-Path $mysql 'lib\plugin\debug') -ErrorAction SilentlyContinue
# Not needed for a local dev server: MeCab Japanese full-text dictionaries (~130 MB), import libs,
# the GUI configurator, benchmarking/keyring-migration/MyISAM maintenance tools, Perl scripts,
# and enterprise client auth plugins (OCI, Kerberos, LDAP).
Remove-Item -Recurse -Force (Join-Path $mysql 'lib\mecab') -ErrorAction SilentlyContinue
Get-ChildItem (Join-Path $mysql 'bin') -Include '*.lib', '*.pl' -File -Recurse | Remove-Item -Force
foreach ($f in 'mysql_configurator.exe', 'mysqlslap.exe', 'mysql_migrate_keyring.exe', 'myisam_ftdump.exe', 'myisamlog.exe', 'myisampack.exe') {
    Remove-Item -Force (Join-Path $mysql "bin\$f") -ErrorAction SilentlyContinue
}
foreach ($f in 'authentication_oci_client.dll', 'authentication_kerberos_client.dll', 'authentication_ldap_sasl_client.dll') {
    Remove-Item -Force (Join-Path $mysql "lib\plugin\$f") -ErrorAction SilentlyContinue
}
foreach ($req in 'bin\mysqld.exe', 'bin\mysql.exe', 'bin\mysqldump.exe', 'bin\mysqladmin.exe', 'share', 'lib\plugin') {
    if (-not (Test-Path (Join-Path $mysql $req))) { throw "MySQL payload is missing $req" }
}
Copy-Item (Join-Path $mysql 'LICENSE') (Join-Path $Lic 'MySQL-LICENSE.txt')

# apps/phpmyadmin/
$w = Join-Path $Work 'pma'; Expand-Zip $pmaZip $w
$top = Get-ChildItem $w -Directory | Select-Object -First 1
$pma = Join-Path $Payload 'apps\phpmyadmin'
Move-Contents $top.FullName $pma
Remove-Item -Recurse -Force (Join-Path $pma 'setup') -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force (Join-Path $pma 'tmp') | Out-Null
Copy-Item (Join-Path $pma 'LICENSE') (Join-Path $Lic 'phpMyAdmin-LICENSE.txt')
$chars = [char[]]'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$bytes = New-Object byte[] 32; $rng.GetBytes($bytes)
$secret = -join ($bytes | ForEach-Object { $chars[$_ % $chars.Length] })
$pmaCfg = @"
<?php
/**
 * Apnoro phpMyAdmin configuration (shipped once; never overwritten on upgrade).
 * MySQL port is read from the Apnoro config.json (<data dir>/config.json -> ports.mysql).
 */
`$apnoroPort = 3306;
`$apnoroCfgFile = dirname(__DIR__, 2) . DIRECTORY_SEPARATOR . 'config.json';
if (is_readable(`$apnoroCfgFile)) {
    `$apnoroCfg = json_decode((string) file_get_contents(`$apnoroCfgFile), true);
    if (is_array(`$apnoroCfg) && !empty(`$apnoroCfg['ports']['mysql'])) {
        `$apnoroPort = (int) `$apnoroCfg['ports']['mysql'];
    }
}

`$cfg['blowfish_secret'] = '$secret';

`$i = 0;
`$i++;
`$cfg['Servers'][`$i]['auth_type'] = 'cookie';
`$cfg['Servers'][`$i]['host'] = '127.0.0.1';
`$cfg['Servers'][`$i]['port'] = (string) `$apnoroPort;
`$cfg['Servers'][`$i]['compress'] = false;
`$cfg['Servers'][`$i]['AllowNoPassword'] = true;

`$cfg['UploadDir'] = '';
`$cfg['SaveDir'] = '';
`$cfg['TempDir'] = __DIR__ . DIRECTORY_SEPARATOR . 'tmp';
`$cfg['VersionCheck'] = false;
"@
[System.IO.File]::WriteAllText((Join-Path $pma 'config.inc.php'), $pmaCfg, (New-Object System.Text.UTF8Encoding($false)))

# bin extras (installed to {app}\bin)
Copy-Item -Force $composer (Join-Path $BinX 'composer.phar')
[System.IO.File]::WriteAllText((Join-Path $BinX 'composer.bat'), "@php `"%~dp0composer.phar`" %*`r`n", (New-Object System.Text.ASCIIEncoding))

Remove-Item -Recurse -Force $Work

# ---------------------------------------------------------------- manifest
$sizeBytes = (Get-ChildItem $Payload -Recurse -File | Measure-Object -Property Length -Sum).Sum
$versions['payloadBytes'] = $sizeBytes
[System.IO.File]::WriteAllText((Join-Path $Payload 'versions.json'), ($versions | ConvertTo-Json -Depth 5), (New-Object System.Text.UTF8Encoding($false)))

Write-Step 'Done'
Write-Host ("    payload: {0} ({1:N0} MB)" -f $Payload, ($sizeBytes / 1MB))
foreach ($k in 'apache', 'mod_fcgid', 'php', 'mysql', 'phpmyadmin', 'composer', 'vc_redist') {
    Write-Host ("    {0,-11} {1}" -f $k, $versions[$k].version)
}
