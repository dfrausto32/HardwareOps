#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-https://localhost:8080}
INSECURE=${INSECURE:-0}

if [[ "$BASE_URL" == https:* ]]; then
  WS_URL=${BASE_URL/https:/wss:}
else
  WS_URL=${BASE_URL/http:/ws:}
fi
WS_URL="${WS_URL%/}/api/v1/events"

if command -v npx >/dev/null 2>&1; then
  args=(-c "$WS_URL")
  if [ "$INSECURE" = "1" ]; then
    args+=(--no-check)
  fi
  echo "Connecting to $WS_URL"
  npx wscat "${args[@]}"
  exit 0
fi

if command -v websocat >/dev/null 2>&1; then
  echo "Connecting to $WS_URL"
  websocat "$WS_URL"
  exit 0
fi

echo "Neither npx nor websocat found. Install Node.js or websocat to watch events." >&2
exit 1
