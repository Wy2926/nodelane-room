#!/usr/bin/env bash
set -euo pipefail
umask 077
case "$1" in
  control)
    # Only the public HTTPS certificate is shared. Both private keys stay here.
    openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
      -keyout /state/control.key -out /state/control.crt \
      -subj /CN=control -addext subjectAltName=DNS:control >/dev/null 2>&1
    cp /state/control.crt /trust/control.crt
    chmod 644 /trust/control.crt
    exec nodelane-server --state-dir /state/control serve --listen :8443 --tls-cert /state/control.crt \
      --tls-key /state/control.key
    ;;
  relay)
    export NLROOM_DEPLOYMENT=container NLROOM_MAPPED_PORT=4242
    exec nlroom-node run --state-dir /state/node --server https://control:8443
    ;;
  client)
    # Independent underlays, including an explicit deny route as a second guard.
    ip route replace unreachable "$BLOCK_SUBNET"
    ip route replace 224.0.0.0/4 dev eth0
    python3 /opt/test/peer.py serve &
    peer_pid=$!
    nodelane --state-dir /state/client daemon &
    agent_pid=$!
    cleanup() {
      kill "$agent_pid" "$peer_pid" 2>/dev/null || true
      wait "$agent_pid" "$peer_pid" 2>/dev/null || true
    }
    trap cleanup EXIT
    trap 'exit 0' TERM INT
    wait -n "$agent_pid" "$peer_pid"
    ;;
  *) exit 2 ;;
esac
