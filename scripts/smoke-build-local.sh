#!/usr/bin/env bash
# Smoke check for `make build-local`: it must succeed from a pristine build/ and
# again over a build/ that already holds junk, and leave a runnable build/olla
# plus the staged build/docker-config.yaml. This guards the PR #239 regression
# where --output pointed at the build/ directory the goreleaser before-hooks
# create. Needs goreleaser and bun (the hooks run `make build-web`).
#
# Not wired into CI or `make ready`: it is slow, needs goreleaser and bun, and
# `make ci` already covers the plain build. It replaces ./build (gitignored
# build output) wholesale and removes it again on exit, so it refuses to run
# until you confirm with OLLA_SMOKE_OK=1. Nothing outside ./build is touched.
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "${OLLA_SMOKE_OK:-}" != "1" ]]; then
  echo "smoke-build-local: this deletes $(pwd)/build. Set OLLA_SMOKE_OK=1 to confirm:" >&2
  echo "  OLLA_SMOKE_OK=1 make smoke-build-local" >&2
  exit 2
fi

for tool in goreleaser bun go; do
  command -v "$tool" >/dev/null || { echo "smoke-build-local: $tool not found" >&2; exit 2; }
done

# Fixed relative path under the repo root (cd above), never a variable, so the
# cleanup can't be pointed anywhere else.
cleanup() { rm -rf ./build; }
trap cleanup EXIT

exe="olla$(go env GOEXE)"

check() {
  local label=$1
  [[ -d build ]] || { echo "FAIL ($label): build is not a directory" >&2; exit 1; }
  [[ -x build/$exe ]] || { echo "FAIL ($label): build/$exe missing or not executable" >&2; exit 1; }
  [[ -f build/docker-config.yaml ]] || { echo "FAIL ($label): build/docker-config.yaml missing" >&2; exit 1; }
  "./build/$exe" --version >/dev/null || { echo "FAIL ($label): build/$exe --version failed" >&2; exit 1; }
  echo "ok ($label)"
}

rm -rf ./build
make build-local
check "clean"

# a second run over junk must be idempotent
mkdir -p build/junk
touch build/stale.txt build/junk/leftover
make build-local
check "pre-populated"
