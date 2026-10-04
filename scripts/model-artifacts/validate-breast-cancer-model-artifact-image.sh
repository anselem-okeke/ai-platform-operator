#!/usr/bin/env bash

set -euo pipefail

IMAGE="${1:?usage: $0 <image-reference> [expected-model-version] [expected-source-revision]}"
EXPECTED_MODEL_VERSION="${2:-v1}"
EXPECTED_SOURCE_REVISION="${3:-}"

EXPORT_DIR="$(
  mktemp -d \
    /tmp/breast-cancer-model-artifact-export.XXXXXX
)"

TAMPER_DIR="$(
  mktemp -d \
    /tmp/breast-cancer-model-artifact-tamper.XXXXXX
)"

TAMPER_LOG="$(
  mktemp \
    /tmp/breast-cancer-model-artifact-tamper.XXXXXX.log
)"

cleanup() {
  rm -rf \
    "$EXPORT_DIR" \
    "$TAMPER_DIR" \
    "$TAMPER_LOG"
}

trap cleanup EXIT

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

echo "Inspecting Breast cancer model artifact image..."

IMAGE_USER="$(
  docker image inspect "$IMAGE" \
    --format '{{.Config.User}}'
)"

IMAGE_ENTRYPOINT="$(
  docker image inspect "$IMAGE" \
    --format '{{json .Config.Entrypoint}}'
)"

IMAGE_ARCHITECTURE="$(
  docker image inspect "$IMAGE" \
    --format '{{.Architecture}}'
)"

MODEL_NAME="$(
  docker image inspect "$IMAGE" \
    --format '{{index .Config.Labels "platform.anselem.dev.model.name"}}'
)"

MODEL_VERSION="$(
  docker image inspect "$IMAGE" \
    --format '{{index .Config.Labels "platform.anselem.dev.model.version"}}'
)"

MODEL_FORMAT="$(
  docker image inspect "$IMAGE" \
    --format '{{index .Config.Labels "platform.anselem.dev.model.format"}}'
)"

SOURCE_REVISION="$(
  docker image inspect "$IMAGE" \
    --format '{{index .Config.Labels "org.opencontainers.image.revision"}}'
)"

printf '%s\n' \
  "user=${IMAGE_USER}" \
  "entrypoint=${IMAGE_ENTRYPOINT}" \
  "architecture=${IMAGE_ARCHITECTURE}" \
  "modelName=${MODEL_NAME}" \
  "modelVersion=${MODEL_VERSION}" \
  "modelFormat=${MODEL_FORMAT}" \
  "sourceRevision=${SOURCE_REVISION}"

test "$IMAGE_USER" = "1000" \
  || fail "bundle image must run as user 1000"

test "$IMAGE_ENTRYPOINT" = "null" \
  || fail "bundle image must reset the inherited runtime entrypoint"

test "$IMAGE_ARCHITECTURE" = "amd64" \
  || fail "expected amd64 image, found ${IMAGE_ARCHITECTURE}"

test "$MODEL_NAME" = "breast-cancer" \
  || fail "expected model name breast-cancer, found ${MODEL_NAME}"

test "$MODEL_VERSION" = "$EXPECTED_MODEL_VERSION" \
  || fail \
    "expected model version ${EXPECTED_MODEL_VERSION}, found ${MODEL_VERSION}"

test "$MODEL_FORMAT" = "sklearn" \
  || fail "expected sklearn model format, found ${MODEL_FORMAT}"

test -n "$SOURCE_REVISION" \
  || fail "source revision label is empty"

if test -n "$EXPECTED_SOURCE_REVISION"; then
  test "$SOURCE_REVISION" = "$EXPECTED_SOURCE_REVISION" \
    || fail \
      "expected source revision ${EXPECTED_SOURCE_REVISION}, found ${SOURCE_REVISION}"
fi

echo "PASS: image identity and execution configuration validated"

chmod 0777 "$EXPORT_DIR"

echo "Exporting verified artifact bundle..."

docker run \
  --rm \
  --volume "${EXPORT_DIR}:/output" \
  "$IMAGE"

EXPECTED_FILES="$(
  printf '%s\n' \
    checksums.sha256 \
    manifest.json \
    model.joblib
)"

ACTUAL_FILES="$(
  find "$EXPORT_DIR" \
    -maxdepth 1 \
    -type f \
    -printf '%f\n' \
  | sort
)"

if test "$ACTUAL_FILES" != "$EXPECTED_FILES"; then
  echo "Expected files:" >&2
  printf '%s\n' "$EXPECTED_FILES" >&2

  echo "Actual files:" >&2
  printf '%s\n' "$ACTUAL_FILES" >&2

  fail "exported bundle contains an unexpected file set"
fi

echo "PASS: exported bundle contains the expected files"

(
  cd "$EXPORT_DIR"
  sha256sum --check checksums.sha256
)

echo "PASS: exported artifact checksums validated"

echo "Verifying the exported artifact in the production runtime..."

docker run \
  --rm \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --volume "${EXPORT_DIR}:/artifact:ro" \
  "$IMAGE" \
  python /opt/breast-cancer/verify.py \
    --artifact-dir /artifact \
    --expected-model-version "$EXPECTED_MODEL_VERSION"

echo "PASS: exported artifact loads and predicts in the production runtime"

echo "Preparing deliberately tampered artifact..."

cp -R \
  "${EXPORT_DIR}/." \
  "$TAMPER_DIR/"

# The production bundle is deliberately read-only. Permit the
# non-root container to traverse the disposable test directory, then
# make only its copied model writable for deliberate corruption.
chmod 0755 "$TAMPER_DIR"

chmod u+w \
  "${TAMPER_DIR}/model.joblib"

printf '%s' \
  'deliberate-integrity-test-tampering' \
  >> "${TAMPER_DIR}/model.joblib"

if docker run \
  --rm \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --volume "${TAMPER_DIR}:/artifact:ro" \
  "$IMAGE" \
  python /opt/breast-cancer/verify.py \
    --artifact-dir /artifact \
    --expected-model-version "$EXPECTED_MODEL_VERSION" \
  > "$TAMPER_LOG" \
  2>&1
then
  cat "$TAMPER_LOG"
  fail "tampered artifact unexpectedly passed verification"
fi

cat "$TAMPER_LOG"

TAMPER_OUTPUT="$(
  cat "$TAMPER_LOG"
)"

case "$TAMPER_OUTPUT" in
  *"ERROR: checksum mismatch for model.joblib:"*)
    ;;
  *)
    fail \
      "tampered artifact failed for an unexpected reason"
    ;;
esac

echo "PASS: tampered model was rejected because its checksum changed"
echo "PASS: Breast cancer model artifact image validation completed"
