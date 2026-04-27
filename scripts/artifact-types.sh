#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

ARTIFACT_NAME=${ARTIFACT_NAME:-type-demo}
VERSIONS=${VERSIONS:-0.1.0,0.2.0}
VERSION_BASE=${VERSION_BASE:-0.1.0}
TYPES=${TYPES:-app_bundle,config_bundle,data_bundle,firmware,container_image,agent_bundle}
OUT_DIR=${OUT_DIR:-/tmp/parcel-types}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}

if [ "$SIGN_ARTIFACTS" = "1" ]; then
  source "$BASE_DIR/scripts/ensure-signing-key.sh"
fi

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

if [ -z "$VERSIONS" ]; then
  VERSIONS="$VERSION_BASE"
fi

IFS=',' read -r -a type_list <<<"$TYPES"
IFS=',' read -r -a version_list <<<"$VERSIONS"

for atype in "${type_list[@]}"; do
  atype=$(echo "$atype" | xargs)
  if [ -z "$atype" ]; then
    continue
  fi
  for base_ver in "${version_list[@]}"; do
    base_ver=$(echo "$base_ver" | xargs)
    if [ -z "$base_ver" ]; then
      continue
    fi
    version="${base_ver}-${atype}"
    input_dir="$OUT_DIR/input/$atype/$base_ver"
    mkdir -p "$input_dir"
    cat > "$input_dir/readme.txt" <<EOF
Parcel artifact demo
type=${atype}
baseVersion=${base_ver}
artifactVersion=${version}
createdAt=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
    printf "%s\n" "payload-${atype}-${base_ver}-$(date -u +%s)" > "$input_dir/payload.txt"
    cat > "$input_dir/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
cat > files/preapply.txt <<EOF_TXT
preapply ok
type=${HWOPS_ARTIFACT_TYPE}
version=${HWOPS_ARTIFACT_VERSION}
time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF_TXT
EOF
    chmod 0755 "$input_dir/preapply.sh"
    cat > "$input_dir/plan.yaml" <<'EOF'
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 120
EOF
    cat > "$input_dir/index.html" <<EOF
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Parcel Artifact Demo</title>
    <style>
      :root { color-scheme: light; }
      body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 0; background: #0a0a0a; color: #f5f5f5; }
      .wrap { padding: 32px; }
      .card { border: 1px solid #333; padding: 20px; border-radius: 12px; background: #111; max-width: 720px; }
      .tag { display: inline-block; padding: 4px 8px; border-radius: 999px; font-size: 12px; background: #b91c1c; color: #fff; }
      .kv { margin: 12px 0; }
      .kv strong { display: inline-block; width: 140px; color: #ddd; }
      .note { margin-top: 16px; color: #bbb; font-size: 13px; }
      .muted { color: #999; }
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="card">
        <div class="tag">Parcel Demo</div>
        <h1>Artifact Applied</h1>
        <div class="kv"><strong>Type</strong> ${atype}</div>
        <div class="kv"><strong>Base Version</strong> ${base_ver}</div>
        <div class="kv"><strong>Artifact Version</strong> ${version}</div>
        <div class="kv"><strong>Generated</strong> <span class="muted">$(date -u +"%Y-%m-%dT%H:%M:%SZ")</span></div>
        <div class="note">If this page changed, the agent applied a new artifact.</div>
        <div class="note">Pre-apply status: <span id="preapply" class="muted">loading...</span></div>
      </div>
    </div>
    <script>
      fetch('preapply.txt')
        .then(r => r.text())
        .then(t => { document.getElementById('preapply').textContent = t.trim() || 'ok'; })
        .catch(() => { document.getElementById('preapply').textContent = 'unavailable'; });
    </script>
  </body>
</html>
EOF

    tar_path="$OUT_DIR/${ARTIFACT_NAME}-${version}.tar.gz"
    pack_args=(--name "$ARTIFACT_NAME" --version "$version" --type "$atype" --input-dir "$input_dir" --out "$tar_path")
    if [ "$SIGN_ARTIFACTS" = "1" ]; then
      pack_args+=(--signing-key "$SIGNING_KEY" --signing-key-id "$SIGNING_KEY_ID")
    fi
    PACK_JSON=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${pack_args[@]}")
    SIG=$(python3 - <<'PY' "$PACK_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
    SIG_KEY_ID=$(python3 - <<'PY' "$PACK_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

    meta_json=$(python3 - <<'PY' "$atype" "$base_ver" "$version"
import json, sys
print(json.dumps({
  "type": sys.argv[1],
  "baseVersion": sys.argv[2],
  "artifactVersion": sys.argv[3],
  "source": "artifact-types",
}))
PY
)

    form_args=(-F "name=$ARTIFACT_NAME" -F "version=$version" -F "type=$atype" -F "metadata=$meta_json" -F "file=@$tar_path")
    if [ -n "$SIG" ]; then
      form_args+=(-F "signature=$SIG" -F "signatureKeyId=$SIG_KEY_ID")
    fi
    upload_json=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_args[@]}")

    artifact_id=$(python3 - <<'PY' "$upload_json"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId",""))
PY
)
    echo "Uploaded type=$atype version=$version id=$artifact_id"
  done
done
