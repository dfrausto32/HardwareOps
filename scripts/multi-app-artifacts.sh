#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

APP_NAMES=${APP_NAMES:-customer,integrations}
VERSIONS=${VERSIONS:-0.1.0,0.2.0}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
COMPONENT_PREFIX=${COMPONENT_PREFIX:-app:}
OUT_DIR=${OUT_DIR:-/tmp/hardwareops-multi-apps}
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

IFS=',' read -r -a app_list <<<"$APP_NAMES"
IFS=',' read -r -a version_list <<<"$VERSIONS"

for app in "${app_list[@]}"; do
  app=$(echo "$app" | xargs)
  if [ -z "$app" ]; then
    continue
  fi
  for version in "${version_list[@]}"; do
    version=$(echo "$version" | xargs)
    if [ -z "$version" ]; then
      continue
    fi

    input_dir="$OUT_DIR/input/$app/$version"
    mkdir -p "$input_dir"

    cat > "$input_dir/readme.txt" <<EOF
HardwareOps multi-app demo
app=${app}
version=${version}
type=${ARTIFACT_TYPE}
componentKey=${COMPONENT_PREFIX}${app}
createdAt=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

    printf "%s\n" "payload-${app}-${version}-$(date -u +%s)" > "$input_dir/payload.txt"

    cat > "$input_dir/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
cat > files/preapply.txt <<EOF_TXT
preapply ok
type=${HWOPS_ARTIFACT_TYPE}
version=${HWOPS_ARTIFACT_VERSION}
time=$ts
EOF_TXT
echo "$ts" > files/last_applied.txt
echo "$ts artifact=${HWOPS_ARTIFACT_ID:-} version=${HWOPS_ARTIFACT_VERSION:-}" >> files/apply.log
pid_file="${HWOPS_ARTIFACT_ROOT}/heartbeat.pid"
log_file="${HWOPS_ARTIFACT_DIR}/files/heartbeat.log"
if [ -f "$pid_file" ]; then
  old_pid=$(cat "$pid_file" 2>/dev/null || true)
  if [ -n "${old_pid:-}" ] && kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
fi
( while true; do
    beat_ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "$beat_ts" >> "$log_file"
    sleep 10
  done ) >/dev/null 2>&1 &
echo $! > "$pid_file"
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
    <title>HardwareOps ${app}</title>
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
      .row { display: flex; gap: 12px; flex-wrap: wrap; margin-top: 12px; }
      .metric { background: #0f0f0f; border: 1px solid #333; border-radius: 8px; padding: 10px 14px; min-width: 140px; }
      .metric h4 { margin: 0 0 6px; font-size: 12px; color: #aaa; text-transform: uppercase; letter-spacing: 0.08em; }
      .metric .value { font-size: 20px; font-weight: 600; }
      .btn { border: 1px solid #b91c1c; background: transparent; color: #fff; padding: 8px 12px; border-radius: 8px; cursor: pointer; }
      .btn:active { transform: translateY(1px); }
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="card">
        <div class="tag">HardwareOps Demo</div>
        <h1>${app} App</h1>
        <div class="kv"><strong>Type</strong> ${ARTIFACT_TYPE}</div>
        <div class="kv"><strong>Version</strong> ${version}</div>
        <div class="kv"><strong>Component</strong> ${COMPONENT_PREFIX}${app}</div>
        <div class="kv"><strong>Generated</strong> <span class="muted">$(date -u +"%Y-%m-%dT%H:%M:%SZ")</span></div>
        <div class="note">If this page changed, the agent applied a new artifact for this component.</div>
        <div class="row">
          <div class="metric">
            <h4>Heartbeat</h4>
            <div class="value" id="heartbeat">0</div>
          </div>
          <div class="metric">
            <h4>Uptime (s)</h4>
            <div class="value" id="uptime">0</div>
          </div>
          <div class="metric">
            <h4>Clicks</h4>
            <div class="value" id="clicks">0</div>
          </div>
        </div>
        <div class="row">
          <button class="btn" id="clicker">Increment</button>
          <div class="note">Pre-apply status: <span id="preapply" class="muted">loading...</span></div>
        </div>
      </div>
    </div>
    <script>
      const start = Date.now();
      const clickKey = 'hwops:${app}:${version}:clicks';
      const clicksEl = document.getElementById('clicks');
      const heartbeatEl = document.getElementById('heartbeat');
      const uptimeEl = document.getElementById('uptime');
      const clicker = document.getElementById('clicker');
      const loadClicks = () => Number(localStorage.getItem(clickKey) || '0');
      const setClicks = (val) => localStorage.setItem(clickKey, String(val));
      clicksEl.textContent = loadClicks();
      clicker.addEventListener('click', () => {
        const next = loadClicks() + 1;
        setClicks(next);
        clicksEl.textContent = next;
      });
      let beat = 0;
      setInterval(() => {
        beat += 1;
        heartbeatEl.textContent = String(beat);
        uptimeEl.textContent = String(Math.floor((Date.now() - start) / 1000));
      }, 1000);
      fetch('preapply.txt')
        .then(r => r.text())
        .then(t => { document.getElementById('preapply').textContent = t.trim() || 'ok'; })
        .catch(() => { document.getElementById('preapply').textContent = 'unavailable'; });
    </script>
  </body>
</html>
EOF

    tar_path="$OUT_DIR/${app}-${version}.tar.gz"
    pack_args=(--name "$app" --version "$version" --type "$ARTIFACT_TYPE" --input-dir "$input_dir" --out "$tar_path")
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

    meta_json=$(python3 - <<'PY' "$app" "$version" "$ARTIFACT_TYPE" "$COMPONENT_PREFIX"
import json, sys
print(json.dumps({
  "app": sys.argv[1],
  "version": sys.argv[2],
  "type": sys.argv[3],
  "componentKey": f"{sys.argv[4]}{sys.argv[1]}",
  "source": "multi-app-artifacts",
}))
PY
)

    form_args=(-F "name=$app" -F "version=$version" -F "type=$ARTIFACT_TYPE" -F "metadata=$meta_json" -F "file=@$tar_path")
    if [ -n "$SIG" ]; then
      form_args+=(-F "signature=$SIG" -F "signatureKeyId=$SIG_KEY_ID")
    fi
    upload_json=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_args[@]}")

    artifact_id=$(python3 - <<'PY' "$upload_json"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId",""))
PY
)
    echo "Uploaded app=$app version=$version type=$ARTIFACT_TYPE component=${COMPONENT_PREFIX}${app} id=$artifact_id"
  done
done
