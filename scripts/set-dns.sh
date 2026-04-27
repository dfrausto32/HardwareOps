#!/usr/bin/env bash
set -euo pipefail

DNS_SERVER=${DNS_SERVER:-}
DOMAIN=${DOMAIN:-parcel.internal}
IFACE=${IFACE:-}
MODE=${MODE:-auto}

if [ -z "$DNS_SERVER" ]; then
  echo "DNS_SERVER is required (example: DNS_SERVER=192.168.1.10 ./scripts/set-dns.sh)" >&2
  exit 1
fi

if [ "$MODE" = "auto" ]; then
  if command -v resolvectl >/dev/null 2>&1; then
    MODE="resolvectl"
  else
    MODE="manual"
  fi
fi

if [ "$MODE" = "resolvectl" ]; then
  if ! command -v resolvectl >/dev/null 2>&1; then
    echo "resolvectl not found. Use MODE=manual." >&2
    exit 1
  fi
  if [ -z "$IFACE" ]; then
    IFACE=$(ip route show default 0.0.0.0/0 | awk '{print $5; exit}')
  fi
  if [ -z "$IFACE" ]; then
    echo "Could not determine network interface. Set IFACE=eth0 (or similar)." >&2
    exit 1
  fi
  if sudo resolvectl dns "$IFACE" "$DNS_SERVER" && sudo resolvectl domain "$IFACE" "$DOMAIN"; then
    echo "Set DNS on $IFACE to $DNS_SERVER for domain $DOMAIN"
    exit 0
  fi
  echo "resolvectl failed; falling back to MODE=manual." >&2
  MODE="manual"
fi

if [ "$MODE" = "manual" ]; then
  RESOLV_CONF=/etc/resolv.conf
  if [ ! -w "$RESOLV_CONF" ]; then
    echo "Cannot write $RESOLV_CONF. Run with sudo or use MODE=resolvectl." >&2
    exit 1
  fi
  sudo cp "$RESOLV_CONF" "$RESOLV_CONF.bak.$(date +%s)"
  echo "nameserver $DNS_SERVER" | sudo tee "$RESOLV_CONF" >/dev/null
  echo "search $DOMAIN" | sudo tee -a "$RESOLV_CONF" >/dev/null
  echo "Updated $RESOLV_CONF (backup created)."
  exit 0
fi

echo "Unknown MODE: $MODE (use resolvectl or manual)" >&2
exit 1
