#!/bin/sh
set -eu
if [ "$(id -u)" != 0 ]; then echo 'Run setup as root with --owner <player-user>.' >&2; exit 1; fi
check_ready() {
  attempts=0
  expected=$(cat /usr/share/nlroom/VERSION)
  while [ "$attempts" -lt 30 ]; do
    if status=$(timeout 3 nlroom-cli status --json 2>/dev/null); then
      version=$(printf '%s\n' "$status" | sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/p')
      if [ "$version" = "$expected" ] && printf '%s\n' "$status" | grep -Eq '"protocol_version":[[:space:]]*3([,}]|$)'; then return; fi
    fi
    attempts=$((attempts + 1))
    sleep 1
  done
  echo 'The local service is not ready; inspect systemctl status nlroom-service and /var/lib/nlroom/agent.log.' >&2
  exit 1
}
if [ "$#" = 1 ] && [ "$1" = --check ]; then check_ready; exit; fi
if [ "$#" != 2 ] || [ "$1" != '--owner' ]; then echo 'Usage: sudo nlroom-setup --owner <player-user>' >&2; exit 1; fi
owner_uid=$(id -u -- "$2")
if [ "$owner_uid" = 0 ]; then echo 'Choose a non-root player account.' >&2; exit 1; fi
check_path() {
  if [ -L "$1" ] || [ ! -d "$1" ] || [ "$(stat -c %u -- "$1")" != 0 ] || [ -n "$(find "$1" -maxdepth 0 -perm /022 -print)" ]; then
    echo "Unsafe root-owned directory: $1" >&2; exit 1
  fi
}
for path in / /etc /var /var/lib /run; do check_path "$path"; done
for path in /etc/nlroom /var/lib/nlroom /run/nlroom; do
  if [ -e "$path" ] || [ -L "$path" ]; then check_path "$path"; fi
done
install -d -m 0755 /etc/nlroom /run/nlroom
install -d -m 0700 /var/lib/nlroom
owner_file=/etc/nlroom/owner.uid
if [ -e "$owner_file" ] || [ -L "$owner_file" ]; then
  if [ -L "$owner_file" ] || [ ! -f "$owner_file" ] || [ "$(stat -c %u -- "$owner_file")" != 0 ] || [ -n "$(find "$owner_file" -maxdepth 0 -perm /022 -print)" ]; then
    echo 'Unsafe owner configuration.' >&2; exit 1
  fi
  if [ "$(cat "$owner_file")" != "$owner_uid" ]; then echo 'This installation is bound to a different user; no identity or permissions were changed.' >&2; exit 1; fi
fi
temp=$(mktemp /etc/nlroom/.owner.XXXXXX)
trap 'rm -f -- "$temp"' EXIT HUP INT TERM
printf '%s\n' "$owner_uid" > "$temp"
chmod 0644 "$temp"
mv -f -- "$temp" "$owner_file"
systemctl daemon-reload
systemctl enable --now nlroom-service.service
systemctl enable --now nlroom-update.timer
check_ready
echo 'NodeLane Room is ready. Open the application as the selected player account.'
