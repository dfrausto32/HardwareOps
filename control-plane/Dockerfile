FROM golang:1.22 AS build

WORKDIR /src
COPY control-plane/go.mod control-plane/go.sum ./
RUN go mod download

COPY control-plane/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/control-plane ./cmd/control-plane

FROM docker:24.0.7-cli

WORKDIR /app
RUN apk add --no-cache bash curl
COPY --from=build /out/control-plane /app/control-plane
COPY control-plane/migrations /app/migrations
COPY scripts/apply-upgrade.sh /app/scripts/apply-upgrade.sh
RUN chmod +x /app/scripts/apply-upgrade.sh

EXPOSE 8080
ENTRYPOINT ["/app/control-plane"]
