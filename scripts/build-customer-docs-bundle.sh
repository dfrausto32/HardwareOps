#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION=${VERSION:-$(date +%Y%m%d%H%M%S)}
DIST_DIR=${DIST_DIR:-$BASE_DIR/dist/customer-docs}
STAGE_DIR="$DIST_DIR/hardwareops-customer-docs-$VERSION"
ARCHIVE_PATH="$DIST_DIR/hardwareops-customer-docs-$VERSION.zip"

mkdir -p "$DIST_DIR"
"$BASE_DIR/scripts/assemble-customer-docs.sh" "$STAGE_DIR"

python3 - <<'PY' "$STAGE_DIR" "$ARCHIVE_PATH"
import os
import sys
import zipfile

stage_dir, archive_path = sys.argv[1], sys.argv[2]
root_name = os.path.basename(stage_dir)
with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    for root, _, files in os.walk(stage_dir):
        for name in files:
            path = os.path.join(root, name)
            rel = os.path.relpath(path, stage_dir)
            zf.write(path, os.path.join(root_name, rel))
PY

echo "built $ARCHIVE_PATH"
