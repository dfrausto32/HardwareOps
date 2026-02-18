#!/usr/bin/env bash
set -euo pipefail

DOMAIN=${DOMAIN:-hardwareops.internal}
DNS_IP=${DNS_IP:-}
CONTAINER_NAME=${CONTAINER_NAME:-hardwareops-coredns}
DATA_DIR=${DATA_DIR:-/tmp/hardwareops-coredns}
FORCE=${FORCE:-0}
HOST_NET=${HOST_NET:-0}

if [ -z "$DNS_IP" ]; then
  echo "DNS_IP is required (the IP that hardwareops.internal should resolve to)." >&2
  echo "Example: DNS_IP=192.168.1.10 ./scripts/run-coredns.sh" >&2
  exit 1
fi

if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  if [ "$FORCE" = "1" ]; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  else
    echo "Container $CONTAINER_NAME already exists. Set FORCE=1 to replace." >&2
    exit 1
  fi
fi

mkdir -p "$DATA_DIR"

CORE_FILE="$DATA_DIR/Corefile"
ZONE_FILE="$DATA_DIR/db.$DOMAIN"
SERIAL=$(date +%Y%m%d%H)

cat > "$CORE_FILE" <<EOF
.:53 {
  log
  errors
  file /etc/coredns/db.$DOMAIN $DOMAIN
  forward . /etc/resolv.conf
  cache 30
}
EOF

cat > "$ZONE_FILE" <<EOF
\$ORIGIN $DOMAIN.
\$TTL 60

@   IN SOA ns1.$DOMAIN. admin.$DOMAIN. (
        $SERIAL ; serial
        7200       ; refresh
        3600       ; retry
        1209600    ; expire
        60         ; minimum
)

@   IN NS ns1.$DOMAIN.
ns1 IN A $DNS_IP
@   IN A $DNS_IP
agent IN A $DNS_IP
EOF

if [ "$HOST_NET" = "1" ]; then
  docker run -d --name "$CONTAINER_NAME" \
    --network host \
    -v "$CORE_FILE":/etc/coredns/Corefile:ro \
    -v "$ZONE_FILE":/etc/coredns/db.$DOMAIN:ro \
    coredns/coredns:1.11.1 -conf /etc/coredns/Corefile
else
  docker run -d --name "$CONTAINER_NAME" \
    -p 53:53/udp -p 53:53/tcp \
    -v "$CORE_FILE":/etc/coredns/Corefile:ro \
    -v "$ZONE_FILE":/etc/coredns/db.$DOMAIN:ro \
    coredns/coredns:1.11.1 -conf /etc/coredns/Corefile
fi

echo "CoreDNS running as $CONTAINER_NAME for $DOMAIN -> $DNS_IP"
if [ "$HOST_NET" = "1" ]; then
  echo "Using host network (binds directly to port 53 on the host)."
fi
