#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"
PUB_DIR="$REPO_DIR/../mosona-manager-pub"
STATIC_DIR="$REPO_DIR/static"
PREVIEW_DIR="$STATIC_DIR/public-preview"

(
  cd "$PUB_DIR"
  pnpm run build
)

if [[ ! -d "$PUB_DIR/dist" ]]; then
  echo "Error: $PUB_DIR/dist not found" >&2
  exit 1
fi

if [[ ! -d "$STATIC_DIR" || -L "$STATIC_DIR" ]]; then
  echo "Error: expected a non-symlink static directory at $STATIC_DIR" >&2
  exit 1
fi

if [[ -L "$PREVIEW_DIR" ]]; then
  echo "Error: refusing to write through symlink $PREVIEW_DIR" >&2
  exit 1
fi

mkdir -p -- "$PREVIEW_DIR"
if [[ "$(cd -- "$PREVIEW_DIR" && pwd -P)" != "$PREVIEW_DIR" ]]; then
  echo "Error: preview directory resolved outside the expected path" >&2
  exit 1
fi

find "$PREVIEW_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
cp -a "$PUB_DIR/dist/." "$PREVIEW_DIR/"

sed -i.bak 's/\/index.js/\/preview-assets\/index.js/g' "$PREVIEW_DIR/index.html"
sed -i.bak 's/\/index.css/\/preview-assets\/index.css/g' "$PREVIEW_DIR/index.html"
rm -f -- "$PREVIEW_DIR/index.html.bak"

rm -f -- "$PREVIEW_DIR/favicon.svg"
rm -rf -- "$PREVIEW_DIR/flags"
rm -rf -- "$PREVIEW_DIR/icons"
