<#
.SYNOPSIS
  Build codex-client.exe on Windows (CGO + Npcap).

.DESCRIPTION
  The capture layer needs cgo + the Npcap SDK, so a plain `go build` will not do.
  This finds the Go toolchain, a mingw-w64 gcc, and the Npcap SDK, wires the cgo
  environment, and builds a versioned codex-client.exe.

.PARAMETER Version
  Version string embedded as main.version (shown by -version). Default: "dev".

.EXAMPLE
  pwsh scripts/build-codex.ps1
  pwsh scripts/build-codex.ps1 -Version 0.1.0
#>
param(
  [string]$Version = "dev",
  [string]$Notes = ""
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot

function Find-First($candidates, $what) {
  foreach ($c in $candidates) {
    if ($c -and (Test-Path $c)) { return $c }
  }
  throw "Could not find $what. Looked in:`n  $($candidates -join "`n  ")"
}

# Go
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { $go = Find-First @('C:\Program Files\Go\bin\go.exe', 'C:\Go\bin\go.exe') 'the Go toolchain' }

# mingw-w64 gcc (winget WinLibs install, or on PATH)
$gccOnPath = (Get-Command gcc -ErrorAction SilentlyContinue).Source
$mingwBin = if ($gccOnPath) { Split-Path -Parent $gccOnPath } else {
  $wingetPkgs = Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages'
  $found = Get-ChildItem $wingetPkgs -Filter 'gcc.exe' -Recurse -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -like '*mingw64*' } | Select-Object -First 1
  if ($found) { $found.DirectoryName } else { throw "Could not find mingw-w64 gcc. Install it (e.g. winget install BrechtSanders.WinLibs.POSIX.UCRT) or add gcc to PATH." }
}

# Npcap SDK
$sdk = Find-First @($env:NPCAP_SDK, "$env:USERPROFILE\dev\npcap-sdk", "$repo\..\npcap-sdk") 'the Npcap SDK (set $env:NPCAP_SDK)'

Write-Host "Go:        $go"
Write-Host "mingw bin: $mingwBin"
Write-Host "Npcap SDK: $sdk"
Write-Host "Version:   $Version"

$env:PATH = "$mingwBin;$env:PATH"
$env:CGO_ENABLED = '1'
$env:CC = 'gcc'
$env:CGO_CFLAGS = "-I$sdk\Include"
$env:CGO_LDFLAGS = "-L$sdk\Lib\x64"

Set-Location $repo
Remove-Item 'codex-client.exe~' -ErrorAction SilentlyContinue

# Exe resources: the Codex icon (Explorer, taskbar, pinned shortcuts) and the
# file properties, from winres/winres.json. `go build` links any
# rsrc_windows_*.syso in the main package directory, so this must run first.
# Without it the exe carries no icon at all and Windows shows its generic one.
# Pinned to the version the upstream build-windows.sh uses.
$winres = (Get-Command go-winres -ErrorAction SilentlyContinue).Source
if (-not $winres) {
  & $go install github.com/tc-hib/go-winres@v0.3.1
  if ($LASTEXITCODE -ne 0) { throw "go-winres install failed ($LASTEXITCODE)" }
  $winres = Join-Path (& $go env GOPATH) 'bin\go-winres.exe'
}
Remove-Item 'rsrc_windows_*.syso' -ErrorAction SilentlyContinue
$winresArgs = @('make', '--in', 'winres/winres.json', '--arch', 'amd64', '--out', 'rsrc')
# Stamp the exe properties with the release version when it is numeric
# (e.g. 0.1.17); "dev" builds keep the 0.0.0.0 in winres.json.
# Release tags look like "alpha-0.1.16"; the file properties need the numbers.
$numericVersion = $Version -replace '^alpha-', ''
if ($numericVersion -match '^\d+(\.\d+){0,3}$') {
  $winresArgs += @('--product-version', $numericVersion, '--file-version', $numericVersion)
}
& $winres @winresArgs
if ($LASTEXITCODE -ne 0) { throw "go-winres failed ($LASTEXITCODE)" }
if (-not (Test-Path "$repo\rsrc_windows_amd64.syso")) { throw "go-winres did not produce rsrc_windows_amd64.syso" }
Write-Host "Embedded exe resources (icon, version info)" -ForegroundColor Green

# Build the PACKAGE (.), not the file: `go build albiondata-client.go` compiles
# a file list and silently skips .syso files, which is why earlier builds had no
# icon even though winres.json existed. The main package is that one file, so
# the result is otherwise identical.
& $go build -ldflags "-X main.version=$Version" -o codex-client.exe .
if ($LASTEXITCODE -ne 0) { throw "build failed ($LASTEXITCODE)" }

$size = [math]::Round((Get-Item "$repo\codex-client.exe").Length / 1MB, 1)
Write-Host "Built codex-client.exe ($size MB)" -ForegroundColor Green

# Emit version.json, the auto-update manifest the client polls (codex_update.go).
# Upload BOTH version.json and codex-client.exe to the Supabase "client" bucket on
# release; clients self-update when version.json names a higher build than theirs.
$sha = (Get-FileHash "$repo\codex-client.exe" -Algorithm SHA256).Hash.ToLower()
$manifest = [ordered]@{
  version = $Version
  url     = 'https://blpjzxfbjwxrdfzeqxhl.supabase.co/storage/v1/object/public/client/codex-client.exe'
  sha256  = $sha
  notes   = $Notes
}
# Write UTF-8 WITHOUT a BOM: Go's encoding/json (the client's manifest parser)
# does not skip a BOM, and Set-Content -Encoding utf8 on Windows PowerShell adds one.
$json = $manifest | ConvertTo-Json
[System.IO.File]::WriteAllText("$repo\version.json", $json, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Wrote version.json (version $Version, sha256 $sha)" -ForegroundColor Green

# Installer (pkg/nsis/codex-client.nsi): built only when NSIS is installed and the
# version is numeric, so a plain dev build needs nothing extra.
$makensis = (Get-Command makensis -ErrorAction SilentlyContinue).Source
if (-not $makensis) {
  $makensis = @("${env:ProgramFiles(x86)}\NSIS\makensis.exe", "$env:ProgramFiles\NSIS\makensis.exe") |
    Where-Object { Test-Path $_ } | Select-Object -First 1
}
if ($makensis -and $numericVersion -match '^\d+\.\d+\.\d+$') {
  & $makensis "/DPACKAGE_VERSION=$numericVersion" "/DTOP_SRCDIR=$repo" "/DOUTFILE=$repo\codex-client-setup.exe" "$repo\pkg\nsis\codex-client.nsi"
  if ($LASTEXITCODE -ne 0) { throw "makensis failed ($LASTEXITCODE)" }
  Write-Host "Built codex-client-setup.exe" -ForegroundColor Green
} else {
  Write-Host "Skipped the installer (needs NSIS on this machine and a numeric -Version such as 0.1.19)."
}
