# Gemini Voice Studio — Wails Windows-native build
# Usage: .\scripts\build-windows-wails.ps1 [-Arch amd64|arm64] [-Clean]
# Requires Windows, Go, Node.js/npm, the Wails v2 CLI, and WebView2.
# SPDX-License-Identifier: Apache-2.0

param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [switch]$Clean
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$BackendRoot = Join-Path $ProjectRoot "backend"
$BinDir = Join-Path $ProjectRoot "bin"
$EmbedDir = Join-Path $BackendRoot "internal" "embed" "dist"
$FrontendDist = Join-Path $ProjectRoot "dist"
$BinaryName = "gemini-voice-studio-windows-$Arch-wails.exe"
$OutputPath = Join-Path $BinDir $BinaryName
$WailsOutputPath = Join-Path $BackendRoot "build" "bin" $BinaryName

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    throw "Wails CLI was not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"
}

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

$Version = if ($env:VERSION) { $env:VERSION } else { "dev" }
$CommitSha = if ($env:COMMIT_SHA) { $env:COMMIT_SHA } else {
    try { (git -c "safe.directory=$ProjectRoot" -C $ProjectRoot rev-parse --short HEAD).Trim() } catch { "unknown" }
}
$BuildDate = if ($env:BUILD_DATE) { $env:BUILD_DATE } else { (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") }
$LdFlags = @(
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Version=$Version",
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Commit=$CommitSha",
    "-X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Date=$BuildDate"
) -join " "

Push-Location $BackendRoot
try {
    # Wails owns the Windows GUI linker flags and WebView2 resource generation.
    # The frontend is already built and copied into the Go embed boundary above.
    wails build -clean -s -skipbindings -platform "windows/$Arch" -webview2 browser -trimpath -ldflags $LdFlags -o $BinaryName
} finally {
    Pop-Location
}

if (-not (Test-Path $WailsOutputPath)) {
    throw "Wails build output not found at $WailsOutputPath"
}
Copy-Item -Force $WailsOutputPath $OutputPath
$Size = [math]::Round((Get-Item $OutputPath).Length / 1MB, 2)
Write-Host "Built $OutputPath ($Size MB)" -ForegroundColor Cyan
Write-Host "WebView2 runtime: system browser mode"
