#!/bin/sh
set -eu
case "$1" in
  remove|upgrade|deconfigure)
    if [ -d /run/systemd/system ]; then
      # Stop waits for Go/Nebula cleanup before dpkg replaces any executable.
      systemctl stop nlroom-service.service
      if [ "$(systemctl show -p MainPID --value nlroom-service.service)" != 0 ]; then
        echo 'Networking process has not exited; refusing to replace its files.' >&2
        exit 1
      fi
      # On upgrade retain enablement and owner binding for postinst recovery.
      if [ "$1" = remove ]; then systemctl disable nlroom-service.service; fi
    fi
    ;;
esac
