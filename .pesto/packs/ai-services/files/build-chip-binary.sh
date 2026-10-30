#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# build-chip-binary - produce the chip-binary image a chip.mode=binary stack deploys, from the
# collibra/chip checkout this node runs in, and record it digest-pinned.
#
# The magneto ai-copilot plugin, in chip.mode=binary, runs the image the spec's chip.image names
# and never builds one. collibra/chip publishes its binary only as a GitHub release asset, so a
# pull request's chip has to be compiled and wrapped here for the stack to run it.
#
# CHIP_SOURCE chooses where the image comes from:
#   source  (default) compile HEAD here (CGO_ENABLED=0 go build ./cmd/chip), crane-append it onto
#           distroless and push it to docker-sandbox under the run tag.
#   ci      CI's own image of HEAD in CI_IMAGE_REPO, tagged <version>-sha-<short commit>, waited
#           for with wait-for-ci-image.sh (bound to this node as task_env WAIT_FOR_CI_IMAGE_SH).
#           collibra/chip's CI publishes no image today, so this needs CI_IMAGE_REPO set.
#
# result: {"CHIP_IMAGE": "<repo>:<tag>@<digest>", image, digest, source_sha, version, source} -
# the provision node fills __PESTO_VAR_CHIP_IMAGE__ from it (SPEC_VARS_FROM).
# Needs secrets: [jfrog-cloud]; go and crane on PATH.
# ---------------------------------------------------------------------------
set -euo pipefail
die() { printf '\nFAILED: %s\n' "$*" >&2; exit 1; }
: "${CHIP_SOURCE:=source}"
: "${CHIP_ARCH:=amd64}"
: "${CI_IMAGE_REPO:=}"
: "${BASE_IMAGE:=gcr.io/distroless/static-debian12:nonroot}"
: "${JFROG_PROPS:=$HOME/.gradle/gradle.properties}"

if [ -z "${JFROG_USER:-}" ] && [ -f "$JFROG_PROPS" ]; then
  JFROG_USER="$(sed -n 's/^ *repositoryUsername *= *//p' "$JFROG_PROPS" | head -1)"
  JFROG_TOKEN="$(sed -n 's/^ *repositoryPassword *= *//p' "$JFROG_PROPS" | head -1)"
fi
[ -n "${JFROG_USER:-}" ] || die "JFROG_USER is not set - declare secrets: [jfrog-cloud] on this node"
[ -n "${JFROG_TOKEN:-}" ] || die "JFROG_TOKEN is not set - declare secrets: [jfrog-cloud] on this node"
export JFROG_USER JFROG_TOKEN
command -v crane >/dev/null || die "crane is not on PATH in this pod"
[ -d cmd/chip ] || die "no cmd/chip in $PWD - this node must run in a collibra/chip checkout"

SHA="$(git rev-parse HEAD)"
VERSION="pr-${SHA:0:12}"
echo "source_sha=$SHA ref=$(git rev-parse --abbrev-ref HEAD) CHIP_SOURCE=$CHIP_SOURCE" >&2

mkdir -p .pesto/artifacts
case "$CHIP_SOURCE" in
ci)
  [ -n "$CI_IMAGE_REPO" ] || die "CHIP_SOURCE=ci needs CI_IMAGE_REPO: collibra/chip's CI publishes no image of its own"
  [ -n "${WAIT_FOR_CI_IMAGE_SH:-}" ] || die "WAIT_FOR_CI_IMAGE_SH is empty - bind wait-for-ci-image.sh to this node as task_env WAIT_FOR_CI_IMAGE_SH"
  W="$(mktemp)"; printf '%s' "$WAIT_FOR_CI_IMAGE_SH" > "$W"
  REF="$(IMAGE_REPO="$CI_IMAGE_REPO" COMMIT="$SHA" bash "$W")" || exit 1
  REF="$(printf '%s\n' "$REF" | tail -1)"
  IMAGE="${REF%@*}"; D="${REF##*@}"
  ;;
source)
  : "${MAGNETO_SESSION_TAG:?MAGNETO_SESSION_TAG is stamped by submit and names a source build}"
  IMAGE="docker-sandbox.repository.collibra.io/chip-binary:${MAGNETO_SESSION_TAG}-${CHIP_ARCH}"
  command -v go >/dev/null || die "go is not on PATH in this pod"
  go version >&2
  W="$(mktemp -d)"; mkdir -p "$W/usr/local/bin"
  CGO_ENABLED=0 GOOS=linux GOARCH="$CHIP_ARCH" go build -trimpath \
    -ldflags "-X github.com/collibra/chip/pkg/chip.Version=${VERSION}" \
    -o "$W/usr/local/bin/chip" ./cmd/chip
  chmod 755 "$W/usr/local/bin/chip"
  tar -C "$W" --owner=0 --group=0 --numeric-owner -cf "$W/l.tar" usr
  crane auth login "${IMAGE%%/*}" -u "$JFROG_USER" -p "$JFROG_TOKEN" >/dev/null
  crane append --platform "linux/$CHIP_ARCH" -b "$BASE_IMAGE" -f "$W/l.tar" -t "$IMAGE" >/dev/null
  crane mutate "$IMAGE" --entrypoint /usr/local/bin/chip -t "$IMAGE" >/dev/null
  D="$(crane digest "$IMAGE")"
  REF="${IMAGE}@${D}"
  ;;
*) die "CHIP_SOURCE must be source or ci (got '$CHIP_SOURCE')" ;;
esac

echo "chip-binary image: $REF" >&2
jq -n --arg r "$REF" --arg i "$IMAGE" --arg d "$D" --arg s "$SHA" --arg v "$VERSION" --arg src "$CHIP_SOURCE" \
  '{CHIP_IMAGE: $r, image: $i, digest: $d, source_sha: $s, version: $v, source: $src}' | tee .pesto/artifacts/result
printf 'chip at %s (%s, linux/%s) -> %s\n' "$SHA" "$CHIP_SOURCE" "$CHIP_ARCH" "$REF" | tee .pesto/artifacts/test_report
