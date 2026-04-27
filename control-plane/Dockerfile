FROM golang:1.25 AS build

WORKDIR /src
COPY control-plane/go.mod control-plane/go.sum ./
RUN go mod download

COPY control-plane/ ./
ARG LICENSE_EMBED_PUBKEY_B64=""
RUN set -eux; \
  if [ -n "$LICENSE_EMBED_PUBKEY_B64" ]; then \
    LDFLAGS="-ldflags=-X=github.com/parcel/control-plane/internal/license.EmbeddedPublicKey=$LICENSE_EMBED_PUBKEY_B64"; \
  else \
    LDFLAGS=""; \
  fi; \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $LDFLAGS -o /out/control-plane ./cmd/control-plane; \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $LDFLAGS -o /out/maintenance-runner ./cmd/maintenance-runner

FROM docker:25.0.5-cli

WORKDIR /app
RUN apk add --no-cache bash curl openssl && \
    mkdir -p /app/demo-certs && \
    openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
      -subj "/CN=Parcel Demo Device CA" \
      -keyout /app/demo-certs/ca.key \
      -out /app/demo-certs/ca.crt && \
    chmod 0600 /app/demo-certs/ca.key && \
    chmod 0644 /app/demo-certs/ca.crt
COPY --from=build /out/control-plane /app/control-plane
COPY --from=build /out/maintenance-runner /app/maintenance-runner
COPY control-plane/migrations /app/migrations
COPY scripts/apply-upgrade.sh /app/scripts/apply-upgrade.sh
COPY scripts/backup-stack.sh /app/scripts/backup-stack.sh
COPY scripts/restore-stack.sh /app/scripts/restore-stack.sh
RUN chmod +x /app/scripts/apply-upgrade.sh
RUN chmod +x /app/scripts/backup-stack.sh /app/scripts/restore-stack.sh

EXPOSE 8080
ENTRYPOINT ["/app/control-plane"]
