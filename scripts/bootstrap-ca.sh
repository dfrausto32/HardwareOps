#!/usr/bin/env bash
set -euo pipefail

OUT_DIR=${OUT_DIR:-/opt/parcel/certs}
CA_CN=${CA_CN:-Parcel Root CA}
CA_DAYS=${CA_DAYS:-3650}
FORCE=${FORCE:-0}

mkdir -p "$OUT_DIR"

CA_KEY="$OUT_DIR/ca.key"
CA_CERT="$OUT_DIR/ca.crt"
CA_CSR="$OUT_DIR/ca.csr"

if [ -f "$CA_KEY" ] || [ -f "$CA_CERT" ]; then
  if [ "$FORCE" != "1" ]; then
    echo "CA already exists in $OUT_DIR. Set FORCE=1 to overwrite." >&2
    exit 1
  fi
fi

tmp_cfg=$(mktemp)
cat > "$tmp_cfg" <<EOF
[req]
distinguished_name = dn
x509_extensions = v3_ca
prompt = no

[dn]
CN = ${CA_CN}

[v3_ca]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true
keyUsage = critical, keyCertSign, cRLSign
EOF

openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout "$CA_KEY" \
  -out "$CA_CERT" \
  -days "$CA_DAYS" \
  -config "$tmp_cfg" \
  -extensions v3_ca

rm -f "$tmp_cfg" "$CA_CSR" "$OUT_DIR/ca.srl"

chmod 0600 "$CA_KEY"
chmod 0644 "$CA_CERT"

echo "CA created:"
echo "  $CA_CERT"
