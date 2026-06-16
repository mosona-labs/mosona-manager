#!/bin/bash
set -euo pipefail

VERSION="${VERSION:-}"
BUILD_DIR="$(pwd)/build/"

usage() {
  cat <<EOF
Usage: $(basename "$0") --version <tag>

Options:
  --version <tag>  Build version injected into hub and agent binaries, for example v0.1.0.
  --help           Show this help.

Environment overrides:
  VERSION
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:?missing version}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  echo "Missing --version <tag> or VERSION environment variable." >&2
  usage >&2
  exit 1
fi

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "Build version: $VERSION"
echo "Packaging deployment files..."
cd deploy && zip -r ../build/deployment_files.zip . -x "*.DS_Store" && cd ..

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
    if [ "$app" = "hub" ]; then
      ldflags="-s -w -X mosona-manager/internal/runtime.Version=${VERSION}"
    else
      ldflags="-s -w -X mosona-manager/agent/runtime.Version=${VERSION}"
    fi
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="$ldflags" -o "$out" "cmd/${app}/main.go"
    echo "Built $(basename "$out")"

    if command -v upx >/dev/null 2>&1; then
      upx -qqq "$out" && echo "Compressed $(basename "$out")"
    fi
  done
done

echo "Build complete. Binaries are located in the 'build' directory."
