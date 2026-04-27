#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

export DATABASE_URL=${DATABASE_URL:-postgres://parcel:parcel@localhost:5432/parcel?sslmode=disable}

( cd "$BASE_DIR/control-plane" && go run ./cmd/migrate -dir ./migrations )
