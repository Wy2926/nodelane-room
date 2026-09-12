#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go mod verify
npm --prefix internal/control/adminweb ci
npm --prefix internal/control/adminweb run build
mkdir -p dist/linux/licenses
cp LICENSE dist/linux/licenses/NodeLaneRoom-LICENSE
cp THIRD_PARTY_NOTICES.md dist/linux/THIRD_PARTY_NOTICES.txt
build_flags='-s -w'
if [ -n "${NODELANE_UPDATE_ROOT:-}" ]; then
  trust=$(base64 < "$NODELANE_UPDATE_ROOT" | tr -d '\n')
  build_flags="$build_flags -X github.com/nodelane/nodelane-room/internal/update.TrustedRootBase64=$trust"
fi
for command in nlroom-cli nlroom-service nlroom-update nlroom-node nodelane-server; do
  CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$build_flags" -o "dist/linux/$command" "./cmd/$command"
done
printf '%s\n' 'Linux executables: dist/linux (use scripts/build.ps1 for all release archives)'
