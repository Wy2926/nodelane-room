#!/bin/sh
set -eu
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database /usr/share/applications
fi
if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi
echo 'Device identity and player binding retained in /var/lib/nlroom and /etc/nlroom.'
