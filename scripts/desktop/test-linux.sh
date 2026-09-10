#!/bin/bash
# This harness may provision only a disposable container, never the host.
set -euo pipefail
test -f /.dockerenv && test "$(id -u)" = 0 && test ! -e /etc/nlroom/owner.uid
package=$1
dpkg -i "$package"
useradd -m -u 10001 player
useradd -m -u 10002 stranger
install -d -m 0755 /etc/nlroom /run/nlroom
install -d -m 0700 /var/lib/nlroom
printf '10001\n' >/etc/nlroom/owner.uid
service_pid=
cleanup() {
  if [ -n "$service_pid" ]; then kill "$service_pid" 2>/dev/null || true; wait "$service_pid" || true; fi
}
trap cleanup EXIT
/usr/lib/nlroom/nlroom-service daemon --state-dir /var/lib/nlroom &
service_pid=$!
nlroom-setup --check
if runuser -u stranger -- nlroom-cli status --json >/dev/null 2>&1; then echo 'Unauthorized user accessed local service' >&2; exit 1; fi
runuser -u player -- dbus-run-session -- xvfb-run -a -s '-screen 0 1120x760x24' python3 /workspace/scripts/desktop/test_webview.py
identity_before=$(sha256sum /var/lib/nlroom/identity.bin | cut -d' ' -f1)
kill "$service_pid"
wait "$service_pid"
service_pid=
# Exercise dpkg replacement and purge on the actual payload in this container.
dpkg -i "$package"
test "$identity_before" = "$(sha256sum /var/lib/nlroom/identity.bin | cut -d' ' -f1)"
test "$(cat /etc/nlroom/owner.uid)" = 10001
/usr/lib/nlroom/nlroom-service daemon --state-dir /var/lib/nlroom &
service_pid=$!
nlroom-setup --check
runuser -u player -- nlroom-cli room leave >/dev/null
kill "$service_pid"
wait "$service_pid"
service_pid=
dpkg --purge nlroom
test -f /var/lib/nlroom/identity.bin && test -f /etc/nlroom/owner.uid
echo 'PASS deb install/reinstall/purge, identity retention and real UID isolation (container; systemd lifecycle tested separately)'
