#!/usr/bin/env bash
set -euo pipefail

KSERVE_VERSION="v0.19.0"
KSERVE_COMMIT="b0eda63d2c105479140af8ec9149d992b7e44be5"

SCRIPT_DIR="$(
  cd -- "$(dirname -- "${BASH_SOURCE[0]}")" \
    >/dev/null 2>&1
  pwd
)"

REPO_ROOT="$(
  cd "${SCRIPT_DIR}/../.." \
    >/dev/null 2>&1
  pwd
)"

DEST="${1:-${REPO_ROOT}/.runtime-src/kserve}"

ARCHIVE="$(mktemp)"
WORKDIR="$(mktemp -d)"

cleanup() {
  rm -f "$ARCHIVE"
  rm -rf "$WORKDIR"
}

trap cleanup EXIT

echo "Fetching KServe ${KSERVE_VERSION}"
echo "Expected commit: ${KSERVE_COMMIT}"
echo "Destination: ${DEST}"

# ------------------------------------------------------------
# Verify the immutable upstream tag before using the source.
# ------------------------------------------------------------

ACTUAL_COMMIT="$(
  git ls-remote \
    https://github.com/kserve/kserve.git \
    "refs/tags/${KSERVE_VERSION}" \
  | awk '{print $1}'
)"

if [[ "$ACTUAL_COMMIT" != "$KSERVE_COMMIT" ]]; then
  echo "ERROR: unexpected KServe tag commit" >&2
  echo "expected=${KSERVE_COMMIT}" >&2
  echo "actual=${ACTUAL_COMMIT}" >&2
  exit 1
fi

# ------------------------------------------------------------
# Download and extract outside /mnt/data.
#
# The upstream KServe tree contains symbolic links.
# The /mnt/data filesystem used by this environment does not
# reliably support creating those symlinks.
# ------------------------------------------------------------

curl -fsSL \
  "https://github.com/kserve/kserve/archive/refs/tags/${KSERVE_VERSION}.tar.gz" \
  -o "$ARCHIVE"

mkdir -p "${WORKDIR}/source"

tar \
  -xzf "$ARCHIVE" \
  -C "${WORKDIR}/source" \
  --strip-components=1

# ------------------------------------------------------------
# Materialize the source into the repository build context.
#
# -L deliberately dereferences upstream symbolic links and
# copies their target contents as normal files.
# This makes the source usable on /mnt/data and by Docker.
# ------------------------------------------------------------

rm -rf "$DEST"
mkdir -p "$DEST"

cp -aL \
  "${WORKDIR}/source/." \
  "$DEST/"

printf '%s\n' "$KSERVE_COMMIT" \
  > "${DEST}/.ai-platform-upstream-commit"

echo
echo "KServe source prepared:"
echo "  version=${KSERVE_VERSION}"
echo "  commit=${KSERVE_COMMIT}"
echo "  path=${DEST}"
echo "  symlink handling=dereferenced"
