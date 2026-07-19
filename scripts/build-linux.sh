#!/usr/bin/env bash
# Gemini Voice Studio — Linux Build Script
# Usage: ./scripts/build-linux.sh [--arch amd64|arm64] [--clean]
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ARCH="amd64"
CLEAN=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) ARCH="$2"; shift 2 ;;
        --clean) CLEAN=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done
if [[ "$ARCH" != "amd64" && "$ARCH" != "arm64" ]]; then
    echo "Error: --arch must be amd64 or arm64"
    exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"
EMBED_DIR="$PROJECT_ROOT/backend/internal/embed/dist"
BINARY_NAME="gemini-voice-studio-linux-$ARCH"
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

cd "$PROJECT_ROOT/backend"
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath \
    -ldflags="-s -w \
      -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Version=$VERSION \
      -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Commit=$COMMIT_SHA \
      -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Date=$BUILD_DATE" \
    -o "$BIN_DIR/$BINARY_NAME" ./cmd/server

SIZE=$(du -h "$BIN_DIR/$BINARY_NAME" | cut -f1)
echo "Built $BIN_DIR/$BINARY_NAME ($SIZE)"
echo "Run: ./bin/$BINARY_NAME --version"
