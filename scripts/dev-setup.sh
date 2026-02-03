#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Start dependencies
( cd "$BASE_DIR" && make dev-up )

# Apply migrations
"$BASE_DIR/scripts/migrate.sh"

echo "Dev setup complete."
