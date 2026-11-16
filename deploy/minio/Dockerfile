# syntax=docker/dockerfile:1.7
#
# Parcel-built MinIO server image, compiled from the upstream source tag.
#
# Why this exists: MinIO stopped publishing prebuilt images (minio/minio on
# Docker Hub and quay.io/minio/minio no longer pull) and removed the signed
# release binaries from dl.min.io (HTTP 410). The upstream repository is
# archived but the release tag remains, so we build the exact release the stack
# was developed and tested against. Same release => same on-disk data format;
# no data migration for existing deployments.
#
# This is an UNOFFICIAL build. MinIO is licensed under the GNU AGPLv3; see
# deploy/minio/README.md for the corresponding-source offer.
#
# Build:   docker build -t parcel-minio:local deploy/minio
# Update:  change MINIO_TAG + MINIO_COMMIT together (the commit pin makes a
#          moved/forged tag fail the build instead of silently shipping).

ARG MINIO_TAG=RELEASE.2024-12-18T13-15-44Z
# Commit the (annotated) tag points at. NB: `git ls-remote`/the tags API show the
# tag object (e1f26f7c...); the commit is what `git rev-parse HEAD` returns.
ARG MINIO_COMMIT=16f8cf1c52f0a77eeb8f7565aaf7f7df12454583

# Base images are pinned by digest for reproducible builds.
# golang:1.23-alpine (go.mod at the tag requires go 1.23)
FROM --platform=$BUILDPLATFORM golang:1.23-alpine@sha256:383395b794dffa5b53012a212365d40c8e37109a626ca30d6151c8348d380b5f AS build

ARG MINIO_TAG
ARG MINIO_COMMIT
ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git ca-certificates

WORKDIR /src
# Shallow clone of the release tag, then verify the commit. `git log` inside the
# tree is also what upstream's gen-ldflags.go uses for the embedded commit id.
RUN git clone --quiet --depth 1 --branch "${MINIO_TAG}" https://github.com/minio/minio.git . \
 && actual="$(git rev-parse HEAD)" \
 && if [ "${actual}" != "${MINIO_COMMIT}" ]; then \
      echo "ERROR: ${MINIO_TAG} resolves to ${actual}, expected ${MINIO_COMMIT}" >&2; exit 1; \
    fi

# Mirror upstream's `make build` (CGO off, kqueue tag, -trimpath, release
# ldflags). gen-ldflags takes the version as an ISO timestamp derived from the
# tag: RELEASE.2024-12-18T13-15-44Z -> 2024-12-18T13:15:44Z.
RUN set -eu; \
    version="$(echo "${MINIO_TAG#RELEASE.}" | sed -E 's/T([0-9]{2})-([0-9]{2})-([0-9]{2})Z/T\1:\2:\3Z/')"; \
    export GOPATH=/go MINIO_RELEASE=RELEASE; \
    ldflags="$(go run buildscripts/gen-ldflags.go "${version}")"; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -tags kqueue -trimpath -ldflags "${ldflags}" -o /out/minio .

# ---------------------------------------------------------------------------
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc

ARG MINIO_TAG
ARG MINIO_COMMIT

LABEL org.opencontainers.image.title="parcel-minio" \
      org.opencontainers.image.description="MinIO ${MINIO_TAG} built from source for Parcel (unofficial build)" \
      org.opencontainers.image.version="${MINIO_TAG}" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.source="https://github.com/minio/minio" \
      org.opencontainers.image.revision="${MINIO_COMMIT}" \
      org.opencontainers.image.vendor="Parcel (unofficial MinIO build)"

# curl: keeps existing `curl -sf .../minio/health/live` container healthchecks
# working (the upstream image shipped a static curl).
RUN apk add --no-cache ca-certificates curl

# Same environment as the upstream release image (secret-file names etc.).
ENV MINIO_ACCESS_KEY_FILE=access_key \
    MINIO_SECRET_KEY_FILE=secret_key \
    MINIO_ROOT_USER_FILE=access_key \
    MINIO_ROOT_PASSWORD_FILE=secret_key \
    MINIO_KMS_SECRET_KEY_FILE=kms_master_key \
    MINIO_CONFIG_ENV_FILE=config.env

COPY --from=build /out/minio /usr/bin/minio
# Upstream's entrypoint: prepends `minio` when the command starts with an
# option/subcommand, so compose `command: server /data ...` keeps working.
COPY --from=build /src/dockerscripts/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
COPY --from=build /src/LICENSE /licenses/LICENSE
COPY --from=build /src/CREDITS /licenses/CREDITS

EXPOSE 9000
VOLUME ["/data"]

ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
CMD ["minio"]
