#!/bin/sh
set -eu
case "$1" in
  configure|abort-upgrade|abort-remove|abort-deconfigure)
    if [ -d /run/systemd/system ]; then
      systemctl daemon-reload
      systemctl enable --now nlroom-update.timer
      # First install stays stopped until a player explicitly binds their UID.
      if [ -f /etc/nlroom/owner.uid ] && systemctl is-enabled --quiet nlroom-service.service; then
        systemctl start nlroom-service.service
        nlroom-setup --check
      fi
    fi
    ;;
esac
echo 'Open NodeLane Room as the installation user. First install: sudo nlroom-setup --owner <player-user>'
