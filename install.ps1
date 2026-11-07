# vynsh installer for Windows.
#
#   irm https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.ps1 | iex
#
# Parameters:
#   -Version      release to install, e.g. 0.3.0-beta (default: latest)
#   -InstallDir   where to put the binary (default: %LOCALAPPDATA%\Programs\vynsh)
#   -NoPath       do not add the install directory to the user PATH
#   -NoVerify     skip the checksum check (not recommended)
[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$InstallDir = "",
    [string]$Repo = "jaraslaukunin/vynsh",
    [switch]$NoPath,
    [switch]$NoVerify
)

$ErrorActionPreference = "Stop"

function Write-Step($message) { Write-Host $message }
function Fail($message) { Write-Error $message; exit 1 }

# ---- detect the platform -------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    "x86" { Fail "32-bit Windows is not supported by vynsh" }
    default { "amd64" }
}

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\vynsh"
}

# ---- resolve the release -------------------------------------------------
# Archives are named after the version, so "latest" is resolved to a tag first.
# GitHub's /releases/latest skips pre-releases, hence the list fallback.
$api = "https://api.github.com/repos/$Repo/releases"

if ($Version -eq "latest") {
    $release = $null
    try { $release = Invoke-RestMethod -Uri "$api/latest" -UseBasicParsing } catch { $release = $null }
    if (-not $release -or -not $release.tag_name) {
        try {
            $all = Invoke-RestMethod -Uri "$api?per_page=20" -UseBasicParsing
            $release = $all | Where-Object { -not $_.draft } | Select-Object -First 1
        } catch {
            Fail "could not find a release in $Repo`n  * check your network, or pick a version: -Version 0.3.0-beta`n  * releases: https://github.com/$Repo/releases"
        }
    }
    if (-not $release -or -not $release.tag_name) {
        Fail "could not find a release in $Repo`n  * releases: https://github.com/$Repo/releases"
    }
    $tag = $release.tag_name
    $Version = $tag -replace '^v', ''
} else {
    $tag = "v$Version"
}
$base = "https://github.com/$Repo/releases/download/$tag"
$asset = "vynsh_${Version}_windows_${arch}.zip"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("vynsh-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Write-Step "downloading $asset"
    $zip = Join-Path $tmp $asset
    try {
        Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -UseBasicParsing
    } catch {
        Fail "could not download $base/$asset`n  * check that version $Version exists (see https://github.com/$Repo/releases)`n  * or build from source: go install github.com/$Repo/cmd/vynsh@latest"
    }

    # ---- verify the checksum --------------------------------------------
    if (-not $NoVerify) {
        try {
            $sums = (Invoke-WebRequest -Uri "$base/SHA256SUMS" -UseBasicParsing).Content
            $line = ($sums -split "`n" | Where-Object { $_ -match [regex]::Escape($asset) } | Select-Object -First 1)
            if ($line) {
                $want = ($line -split "\s+")[0].Trim().ToLower()
                $got = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLower()
                if ($want -ne $got) { Fail "checksum mismatch for $asset (expected $want, got $got)" }
                Write-Step "checksum ok"
            }
        } catch {
            Write-Step "note: SHA256SUMS is not available for this release, skipping verification"
        }
    }

    # ---- unpack ----------------------------------------------------------
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    $exe = Get-ChildItem -Path $tmp -Recurse -Filter "vynsh.exe" | Select-Object -First 1
    if (-not $exe) { Fail "the archive does not contain vynsh.exe" }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $target = Join-Path $InstallDir "vynsh.exe"
    Copy-Item -Path $exe.FullName -Destination $target -Force

    Write-Host ""
    Write-Host "v vynsh installed to $target" -ForegroundColor Green
    try { & $target version | ForEach-Object { Write-Host "  $_" } } catch { }

    # ---- PATH ------------------------------------------------------------
    if (-not $NoPath) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($userPath -notlike "*$InstallDir*") {
            $newPath = if ([string]::IsNullOrEmpty($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            $env:Path = "$env:Path;$InstallDir"
            Write-Host ""
            Write-Host "Added $InstallDir to your PATH (new terminals pick it up automatically)."
        }
    }

    Write-Host ""
    Write-Host "Try it:  vynsh report    or    vynsh tui"
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
