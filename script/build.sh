#!/bin/bash
set -euo pipefail

WEB_DIR="../mosona-manager-web/"
PUB_DIR="../mosona-manager-pub/"

BUILD_DIR="$(pwd)/build/"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

read -r -p "Is it necessary to build the frontend? (main) [Y/n] " REPLY1
REPLY1=${REPLY1:-y}
read -r -p "Is it necessary to build the public frontend? (public) [Y/n] " REPLY2
REPLY2=${REPLY2:-y}
read -r -p "Is it necessary to build the backend binaries? (backend) [Y/n] " REPLY3
REPLY3=${REPLY3:-y}

# Build and package frontend
if [[ "$REPLY1" =~ ^[Yy]$ ]]; then
  (cd $WEB_DIR && pnpm run build) || { echo "Build error"; exit 1; }
  if [ ! -d "$WEB_DIR/dist" ]; then
    echo "Error: $WEB_DIR/dist not found" >&2
    exit 1
  fi
  mkdir -p "$BUILD_DIR/frontend"
  cp -a "$WEB_DIR/dist/." "$BUILD_DIR/frontend/"
  rm -f "$BUILD_DIR/frontend/.DS_Store"
  (cd "$BUILD_DIR/frontend/" && zip -qr "../frontend_dist.zip" .) || { echo "Zip error"; exit 1; }
  rm -rf "$BUILD_DIR/frontend"
fi

# Build and package public frontend
if [[ "$REPLY2" =~ ^[Yy]$ ]]; then
  (cd $PUB_DIR && pnpm run build) || { echo "Build error"; exit 1; }
  if [ ! -d "$PUB_DIR/dist" ]; then
    echo "Error: $PUB_DIR/dist not found" >&2
    exit 1
  fi
  mkdir -p "$BUILD_DIR/frontend_pub"
  cp -a "$PUB_DIR/dist/." "$BUILD_DIR/frontend_pub/"
  sed -i '' 's/\/index.js/\/preview-assets\/index.js/g' "$BUILD_DIR/frontend_pub/index.html"
  sed -i '' 's/\/index.css/\/preview-assets\/index.css/g' "$BUILD_DIR/frontend_pub/index.html"
  rm -f "$BUILD_DIR/frontend_pub/favicon.svg"
  rm -rf "$BUILD_DIR/frontend_pub/flags"
  rm -rf "$BUILD_DIR/frontend_pub/icons"
  rm -f "$BUILD_DIR/frontend_pub/.DS_Store"
  (cd "$BUILD_DIR/frontend_pub/" && zip -qr "../frontend_pub.zip" .) || { echo "Zip error"; exit 1; }
  rm -rf "$BUILD_DIR/frontend_pub"
fi

echo "Packaging deployment files..."
cd deploy && zip -r ../build/deployment_files.zip . -x "*.DS_Store" && cd ..

if [[ "$REPLY3" =~ ^[Yy]$ ]]; then
  targets=(
    "linux amd64"
    "linux arm64"
    "windows amd64"
    "darwin amd64"
    "darwin arm64"
  )
  apps=(
    "hub"
    "agent"
  )

  for t in "${targets[@]}"; do
    read -r GOOS GOARCH <<< "$t"
    for app in "${apps[@]}"; do
      out="build/${app}_${GOOS}_${GOARCH}"
      if [ "$GOOS" = "windows" ]; then
        out="${out}.exe"
      fi

      echo "Building ${app} for ${GOOS}/${GOARCH} -> ${out}"
      GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$out" "cmd/${app}/main.go"
      echo "Built $(basename "$out")"

      if command -v upx >/dev/null 2>&1; then
        upx -qqq "$out" && echo "Compressed $(basename "$out")"
      fi
    done
  done
fi

cp deploy/install.sh "$BUILD_DIR/"
cp deploy/uninstall.sh "$BUILD_DIR/"
cp deploy/upgrade.sh "$BUILD_DIR/"

echo "Build complete. Binaries are located in the 'build' directory."