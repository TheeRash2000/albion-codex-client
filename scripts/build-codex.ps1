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
& $go build -ldflags "-X main.version=$Version" -o codex-client.exe albiondata-client.go
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
