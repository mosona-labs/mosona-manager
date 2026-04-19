#!/usr/bin/env bash
set -euo pipefail

# Web project directory
PUB_DIR="../mosona-manager-pub/"

# Get the directory of the current script
SCRIPT_DIR="$(pwd)/static/"

(
  cd "$PUB_DIR"
  pnpm run build
)

if [ ! -d "$PUB_DIR/dist" ]; then
  echo "Error: $PUB_DIR/dist not found" >&2
  exit 1
fi

rm -rf "$SCRIPT_DIR/public-preview/*"
cp -a "$PUB_DIR/dist/." "$SCRIPT_DIR/public-preview/"

sed -i '' 's/\/index.js/\/preview-assets\/index.js/g' "$SCRIPT_DIR/public-preview/index.html"
sed -i '' 's/\/index.css/\/preview-assets\/index.css/g' "$SCRIPT_DIR/public-preview/index.html"

rm -f "$SCRIPT_DIR/public-preview/favicon.svg"
rm -rf "$SCRIPT_DIR/public-preview/flags"
rm -rf "$SCRIPT_DIR/public-preview/icons"