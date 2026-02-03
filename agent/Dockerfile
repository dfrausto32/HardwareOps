FROM golang:1.22 AS build

WORKDIR /src
COPY agent/go.mod agent/go.sum ./agent/
WORKDIR /src/agent
RUN go mod download

COPY agent/ .
RUN CGO_ENABLED=0 go build -o /out/agent ./cmd/agent

FROM alpine:3.19
RUN adduser -D -u 10001 agent && \
    apk add --no-cache ca-certificates && \
    mkdir -p /data && chown -R agent:agent /data

USER agent
WORKDIR /home/agent

ENV CONTROL_PLANE_URL=http://host.docker.internal:8080
ENV STATE_PATH=/data/agent-state.json
ENV ARTIFACT_ROOT=/data/artifacts

VOLUME ["/data"]

COPY --from=build /out/agent /usr/local/bin/agent

ENTRYPOINT ["agent"]
