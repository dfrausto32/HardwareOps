FROM golang:1.22 AS build

WORKDIR /src
COPY control-plane/go.mod control-plane/go.sum ./
RUN go mod download

COPY control-plane/ ./
ARG LICENSE_EMBED_PUBKEY_B64=""
RUN set -eux; \
  if [ -n "$LICENSE_EMBED_PUBKEY_B64" ]; then \
    LDFLAGS="-ldflags=-X=github.com/hardwareops/control-plane/internal/license.EmbeddedPublicKey=$LICENSE_EMBED_PUBKEY_B64"; \
  else \
    LDFLAGS=""; \
  fi; \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $LDFLAGS -o /out/control-plane ./cmd/control-plane

FROM docker:25.0.5-cli

WORKDIR /app
RUN apk add --no-cache bash curl
COPY --from=build /out/control-plane /app/control-plane
COPY control-plane/migrations /app/migrations
COPY scripts/apply-upgrade.sh /app/scripts/apply-upgrade.sh
COPY scripts/backup-stack.sh /app/scripts/backup-stack.sh
COPY scripts/restore-stack.sh /app/scripts/restore-stack.sh
RUN chmod +x /app/scripts/apply-upgrade.sh
RUN chmod +x /app/scripts/backup-stack.sh /app/scripts/restore-stack.sh

EXPOSE 8080
ENTRYPOINT ["/app/control-plane"]
