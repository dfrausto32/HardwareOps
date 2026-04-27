#!/usr/bin/env bash
set -euo pipefail

OUT_DIR=${OUT_DIR:-/opt/parcel/certs}
DOMAIN=${DOMAIN:-parcel.internal}
AGENT_DOMAIN=${AGENT_DOMAIN:-agent.${DOMAIN}}
SERVER_DAYS=${SERVER_DAYS:-825}

CA_CERT="$OUT_DIR/ca.crt"
CA_KEY="$OUT_DIR/ca.key"
SERVER_KEY="$OUT_DIR/server.key"
SERVER_CSR="$OUT_DIR/server.csr"
SERVER_CERT="$OUT_DIR/server.crt"

if [ ! -f "$CA_CERT" ] || [ ! -f "$CA_KEY" ]; then
  echo "CA cert/key not found in $OUT_DIR. Run scripts/bootstrap-ca.sh first." >&2
  exit 1
fi

tmp_cfg=$(mktemp)
cat > "$tmp_cfg" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${DOMAIN}

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
subjectAltName = @alt_names

[v3_cert]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${DOMAIN}
EOF
if [ -n "${AGENT_DOMAIN}" ] && [ "${AGENT_DOMAIN}" != "${DOMAIN}" ]; then
  cat >> "$tmp_cfg" <<EOF
DNS.2 = ${AGENT_DOMAIN}
EOF
fi

rm -f "$SERVER_KEY" "$SERVER_CERT" "$SERVER_CSR"

openssl req -new -newkey rsa:2048 -nodes \
  -keyout "$SERVER_KEY" \
  -out "$SERVER_CSR" \
  -config "$tmp_cfg"

openssl x509 -req -in "$SERVER_CSR" \
  -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$SERVER_CERT" -days "$SERVER_DAYS" \
  -extensions v3_cert -extfile "$tmp_cfg"

rm -f "$tmp_cfg"

chmod 0600 "$SERVER_KEY"
chmod 0644 "$SERVER_CERT"

echo "Server cert created:"
echo "  $SERVER_CERT"
