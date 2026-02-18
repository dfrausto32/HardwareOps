#!/bin/sh
set -eu

: "${CONTROL_PLANE_UPSTREAM:=control-plane:8080}"
: "${DNS_RESOLVER:=127.0.0.11}"

template="/etc/nginx/nginx-http.conf.template"
if [ "${GATEWAY_TLS:-1}" = "1" ] && [ -f /certs/server.crt ] && [ -f /certs/server.key ] && [ -f /certs/ca.crt ]; then
  template="/etc/nginx/nginx-https.conf.template"
fi

envsubst '${CONTROL_PLANE_UPSTREAM} ${DNS_RESOLVER}' < "$template" > /etc/nginx/nginx.conf
