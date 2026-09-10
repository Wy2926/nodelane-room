#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go mod verify
mkdir -p dist/linux
for command in nodelane nlroom-node nodelane-server; do
  CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags '-s -w' -o "dist/linux/$command" "./cmd/$command"
done
printf '%s\n' 'Linux executables: dist/linux (use scripts/build.ps1 for all release archives)'
