#requires -Version 5.1
# mcrcon — single-command installer for Windows.
#
# Downloads the release build for your architecture from GitHub, verifies the
# SHA-256 checksum, and installs it into a bin directory (added to PATH).
#
# Usage (PowerShell):
#   ./install.ps1
#   ./install.ps1 -Version v1.3.0 -InstallDir "$HOME\bin"
param(
    [string]$Version = "",
    [string]$InstallDir = "",
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$Repo = "mcrcon/mcrcon"

function Write-Step([string]$Msg) { Write-Host "mcrcon: $Msg" }

function Get-LatestVersion {
    $resp = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "mcrcon-installer" }
    return [string]$resp.tag_name
}

if ($Help) {
    Write-Host @"
mcrcon installer (PowerShell)

Usage:
  .\install.ps1 [-Version vX.Y.Z] [-InstallDir <dir>]

Defaults:
  -Version    latest GitHub release
  -InstallDir `$HOME\bin  (added to user PATH)
"@
    exit 0
}

# --- resolve version ---
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = Get-LatestVersion
    Write-Step "using latest release $Version"
}
if (-not $Version.StartsWith("v")) { $Version = "v" + $Version }
$VerNoV = $Version.Substring(1)

# --- detect platform ---
$bits = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString()
$Arch = switch ($bits) {
    "X64"    { "amd64" }
    "Arm64"  { "arm64" }
    default  { throw "unsupported architecture: $bits (supported: amd64, arm64)" }
}

# --- pick install location ---
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $HOME "bin"
}
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# --- download + verify + install ---
$Archive = "mcrcon_${VerNoV}_windows_${Arch}.zip"
$Base = "https://github.com/$Repo/releases/download/$Version"
$Zip = Join-Path $env:TEMP $Archive

Write-Step "downloading $Base/$Archive"
Invoke-WebRequest -Uri "$Base/$Archive" -OutFile $Zip -UseBasicParsing

Write-Step "verifying SHA-256 checksum"
$Checksums = Invoke-WebRequest -Uri "$Base/checksums.txt" -UseBasicParsing | Select-Object -ExpandProperty Content
$Expected = ($Checksums -split "`n" | Where-Object { $_ -match "  $Archive$" } | ForEach-Object { ($_ -split "\s+")[0] }).Trim()
if (-not $Expected) { throw "no checksum entry for $Archive" }
$Actual = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw "checksum mismatch: got $Actual, want $Expected" }

Write-Step "extracting and installing to $InstallDir"
$ExtractDir = Join-Path $env:TEMP "mcrcon-install-$([guid]::NewGuid())"
Expand-Archive -Path $Zip -DestinationPath $ExtractDir
Copy-Item -Path (Join-Path $ExtractDir "mcrcon.exe") -Destination $InstallDir -Force
Remove-Item -Recurse -Force $ExtractDir, $Zip

# --- add to user PATH if missing ---
$Bin = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ((";" + $Bin + ";") -notlike "*;$InstallDir;*") {
    [System.Environment]::SetEnvironmentVariable("PATH", ($Bin.TrimEnd(";") + ";" + $InstallDir), "User")
    Write-Step "added $InstallDir to your user PATH (restart terminals to pick it up)"
}

$VerOut = & (Join-Path $InstallDir "mcrcon.exe") -v
Write-Step "installed $VerOut"