#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
mkdir -p dist
VER="1.2.0"
LDF="-s -w -X main.version=$VER"

build() {
  local goos=$1 goarch=$2 out=$3
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -ldflags "$LDF" -o "dist/$out" .
  echo "built: dist/$out"
}

# Windows 64 位
build windows amd64 datamask-win-x64.exe

# macOS（Intel + Apple Silicon）
build darwin amd64 datamask-macos-x86_64
build darwin arm64 datamask-macos-arm64

echo "=== BUILDS OK ==="
