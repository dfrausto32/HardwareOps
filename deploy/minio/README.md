# parcel-minio — pinned, source-built MinIO image

Parcel stores artifacts in MinIO (S3 API). MinIO no longer publishes container
images (`minio/minio` on Docker Hub and `quay.io/minio/minio` fail to pull) and
removed the signed release binaries from `dl.min.io` (HTTP 410). The upstream
repository is archived, but the release tag the stack was built and tested
against still exists, so this directory builds that exact release from source.

- **Release:** `RELEASE.2024-12-18T13-15-44Z` (commit
  `16f8cf1c52f0a77eeb8f7565aaf7f7df12454583`). Same release ⇒ same on-disk data
  format ⇒ no migration for existing deployments.
- **Unofficial build.** It is not produced or supported by MinIO, Inc.
- **Parcel only uses the S3 API** (via `minio-go`): no `mc`/admin dependency.
  Verified: bucket create with Object Lock, put with retention, ranged GET,
  presigned URLs, and refusal to delete a retained version.

## Build locally

```bash
docker build -t parcel-minio:local deploy/minio
docker run --rm parcel-minio:local minio --version
```

The Dockerfile verifies that the tag still resolves to the pinned commit and
fails otherwise (a moved or forged tag cannot be built silently). Base images
are pinned by digest.

## Publish

Run the **MinIO image** workflow (`workflow_dispatch`) with a patch revision.
It builds `linux/amd64` + `linux/arm64`, smoke-tests the image, runs a
report-only Trivy scan, pushes
`ghcr.io/<owner>/parcel-minio:<release>-p<revision>` with SBOM and provenance
attestations, and signs it with cosign (keyless). Tags are immutable by
convention: rebuilding the same release (new base image, Dockerfile fix) bumps
the revision. **Reference the image by digest** in compose files and installers
(the workflow summary prints it).

## Updating the pinned release

Change `MINIO_TAG` and `MINIO_COMMIT` together in the Dockerfile. Note that the
upstream tag is *annotated*: `git ls-remote` shows the tag object, while the
commit is what `git rev-parse HEAD` returns after cloning — use the commit.
Moving to a different MinIO release changes the on-disk format guarantees:
test an upgrade against a copy of real data first.

## Patching policy

Upstream no longer ships fixes, so this image is frozen at the pinned release.
The Trivy report from each build is published as a workflow artifact (it does
not fail the build). Someone must own reviewing it, deciding whether a finding
is reachable in Parcel's deployment (MinIO sits behind the control plane;
agents only ever see short-lived presigned URLs), and rebuilding when the base
image has fixes. A medium-term exit plan (evaluating a maintained S3 store or
fork, including an Object Lock and data-compatibility test) is tracked in the
roadmap.

## License / corresponding source (AGPLv3)

MinIO is licensed under the GNU Affero General Public License v3.0. The image
includes the license at `/licenses/LICENSE`. The complete corresponding source
for the binary in this image is the upstream tag
<https://github.com/minio/minio/tree/RELEASE.2024-12-18T13-15-44Z> built with
the unmodified source and the recipe in `deploy/minio/Dockerfile` (no patches
are applied). If you distribute this image to customers, keep that pointer
available to them. Have counsel confirm the exact notice wording.
