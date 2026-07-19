# Gemini Voice Studio — Windows Build Script
# Usage: .\scripts\build-windows.ps1 [-Arch amd64|arm64] [-Clean]
# SPDX-License-Identifier: Apache-2.0

param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [switch]$Clean
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$BinDir = Join-Path $ProjectRoot "bin"
$EmbedDir = Join-Path $ProjectRoot "backend" "internal" "embed" "dist"
$FrontendDist = Join-Path $ProjectRoot "dist"
$BinaryName = "gemini-voice-studio-windows-$Arch.exe"
$Version = if ($env:VERSION) { $env:VERSION } else { "dev" }
$CommitSha = if ($env:COMMIT_SHA) { $env:COMMIT_SHA } else {
    try { (git -C $ProjectRoot rev-parse --short HEAD).Trim() } catch { "unknown" }
}
$BuildDate = if ($env:BUILD_DATE) { $env:BUILD_DATE } else { (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") }

if ($Clean) {
    if (Test-Path $BinDir) { Remove-Item -Recurse -Force $BinDir }
    if (Test-Path $EmbedDir) { Remove-Item -Recurse -Force $EmbedDir }
    if (Test-Path $FrontendDist) { Remove-Item -Recurse -Force $FrontendDist }
}

Push-Location $ProjectRoot
try {
    npm ci
    npm run typecheck
    npm run build
} finally {
    Pop-Location
}

if (-not (Test-Path $FrontendDist)) {
    throw "Frontend build output not found at $FrontendDist"
}
if (Test-Path $EmbedDir) { Remove-Item -Recurse -Force $EmbedDir }
New-Item -ItemType Directory -Force -Path $EmbedDir | Out-Null
Copy-Item -Recurse -Force (Join-Path $FrontendDist "*") $EmbedDir
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = $Arch
$LdFlags = @(
    "-s -w",
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Version=$Version",
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Commit=$CommitSha",
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Date=$BuildDate"
) -join " "

Push-Location (Join-Path $ProjectRoot "backend")
try {
    go build -trimpath -ldflags $LdFlags -o (Join-Path $BinDir $BinaryName) ./cmd/server
} finally {
    Pop-Location
}

$OutputPath = Join-Path $BinDir $BinaryName
$Size = [math]::Round((Get-Item $OutputPath).Length / 1MB, 2)
Write-Host "Built $OutputPath ($Size MB)" -ForegroundColor Cyan
Write-Host "Run: .\bin\$BinaryName --version"
