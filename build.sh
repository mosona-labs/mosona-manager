#!/bin/bash
set -euo pipefail

rm -rf build
mkdir -p build

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

echo "Packaging deployment files..."
cd deploy && zip -r ../build/deployment_files.zip . -x "*.DS_Store" && cd ..

echo "Build complete. Binaries are located in the 'build' directory."