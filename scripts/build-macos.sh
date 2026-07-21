#!/usr/bin/env bash
# Gemini Voice Studio — macOS Build Script
# Usage: ./scripts/build-macos.sh [--arch amd64|arm64] [--clean] [--universal]
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ARCH="arm64"
CLEAN=false
UNIVERSAL=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) ARCH="$2"; shift 2 ;;
        --clean) CLEAN=true; shift ;;
        --universal) UNIVERSAL=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done
if [[ "$UNIVERSAL" = false && "$ARCH" != "amd64" && "$ARCH" != "arm64" ]]; then
    echo "Error: --arch must be amd64 or arm64"
    exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"
EMBED_DIR="$PROJECT_ROOT/backend/internal/embed/dist"
VERSION="${VERSION:-dev}"
COMMIT_SHA="${COMMIT_SHA:-$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

if [ "$CLEAN" = true ]; then
    rm -rf "$BIN_DIR" "$EMBED_DIR" "$PROJECT_ROOT/dist"
fi
cd "$PROJECT_ROOT"
npm ci
npm run typecheck
npm run build
rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR"
cp -R "$PROJECT_ROOT/dist/." "$EMBED_DIR/"
mkdir -p "$BIN_DIR"

build_binary() {
    local target_arch="$1"
    local output="$BIN_DIR/gemini-voice-studio-darwin-$target_arch"
    cd "$PROJECT_ROOT/backend"
    CGO_ENABLED=0 GOOS=darwin GOARCH="$target_arch" go build -trimpath \
        -ldflags="-s -w \
          -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Version=$VERSION \
          -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Commit=$COMMIT_SHA \
          -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Date=$BUILD_DATE" \
        -o "$output" ./cmd/server
}

if [ "$UNIVERSAL" = true ]; then
    build_binary amd64
    build_binary arm64
    UNIVERSAL_BINARY="$BIN_DIR/gemini-voice-studio-darwin-universal"
    lipo -create \
        "$BIN_DIR/gemini-voice-studio-darwin-amd64" \
        "$BIN_DIR/gemini-voice-studio-darwin-arm64" \
        -output "$UNIVERSAL_BINARY"
    echo "Built $UNIVERSAL_BINARY"
else
    build_binary "$ARCH"
    echo "Built $BIN_DIR/gemini-voice-studio-darwin-$ARCH"
fi
