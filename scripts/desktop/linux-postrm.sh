#!/bin/sh
set -eu
if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi
echo 'Device identity and player binding retained in /var/lib/nlroom and /etc/nlroom.'
