#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/mosona-preview-build-test.XXXXXX")"
TEST_ROOT="$(cd -- "$TEST_ROOT" && pwd -P)"
trap 'rm -rf -- "$TEST_ROOT"' EXIT

TEST_REPO="$TEST_ROOT/mosona-manager"
TEST_PUB="$TEST_ROOT/mosona-manager-pub"
PREVIEW_DIR="$TEST_REPO/static/public-preview"
MOCK_BIN="$TEST_ROOT/bin"

mkdir -p -- "$TEST_REPO/script" "$PREVIEW_DIR/old-dir" \
  "$TEST_PUB/dist/nested" "$TEST_PUB/dist/flags" "$TEST_PUB/dist/icons" \
  "$MOCK_BIN" "$TEST_ROOT/invocation-dir"
cp -- "$SOURCE_DIR/public_preview_dev_build.sh" "$TEST_REPO/script/"

printf 'stale\n' >"$PREVIEW_DIR/old.txt"
printf 'stale hidden\n' >"$PREVIEW_DIR/.old-hidden"
printf 'stale nested\n' >"$PREVIEW_DIR/old-dir/file.txt"
printf 'keep\n' >"$TEST_REPO/static/sibling.txt"

printf '<link href="/index.css"><script src="/index.js"></script>\n' >"$TEST_PUB/dist/index.html"
printf 'new\n' >"$TEST_PUB/dist/nested/asset.txt"
printf 'new hidden\n' >"$TEST_PUB/dist/.new-hidden"
printf 'remove\n' >"$TEST_PUB/dist/favicon.svg"
printf 'remove\n' >"$TEST_PUB/dist/flags/flag.svg"
printf 'remove\n' >"$TEST_PUB/dist/icons/icon.svg"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  '[[ "$*" == "run build" ]]' \
  '[[ "$PWD" == "$EXPECTED_PUB_DIR" ]]' >"$MOCK_BIN/pnpm"
chmod +x "$MOCK_BIN/pnpm"

(
  cd -- "$TEST_ROOT/invocation-dir"
  EXPECTED_PUB_DIR="$TEST_PUB" PATH="$MOCK_BIN:$PATH" \
    bash "$TEST_REPO/script/public_preview_dev_build.sh"
)

[[ -d "$PREVIEW_DIR" ]]
[[ ! -e "$PREVIEW_DIR/old.txt" ]]
[[ ! -e "$PREVIEW_DIR/.old-hidden" ]]
[[ ! -e "$PREVIEW_DIR/old-dir" ]]
[[ -f "$PREVIEW_DIR/nested/asset.txt" ]]
[[ -f "$PREVIEW_DIR/.new-hidden" ]]
[[ -f "$TEST_REPO/static/sibling.txt" ]]
[[ ! -e "$PREVIEW_DIR/favicon.svg" ]]
[[ ! -e "$PREVIEW_DIR/flags" ]]
[[ ! -e "$PREVIEW_DIR/icons" ]]
[[ ! -e "$PREVIEW_DIR/index.html.bak" ]]
grep -q '/preview-assets/index.css' "$PREVIEW_DIR/index.html"
grep -q '/preview-assets/index.js' "$PREVIEW_DIR/index.html"

rm -rf -- "$PREVIEW_DIR"
ln -s -- "$TEST_ROOT/invocation-dir" "$PREVIEW_DIR"
if EXPECTED_PUB_DIR="$TEST_PUB" PATH="$MOCK_BIN:$PATH" \
  bash "$TEST_REPO/script/public_preview_dev_build.sh" >/dev/null 2>&1; then
  echo "build unexpectedly accepted a symlink preview directory" >&2
  exit 1
fi

echo "public preview build cleanup test passed"
