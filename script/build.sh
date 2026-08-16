#!/bin/bash
set -euo pipefail

VERSION="${VERSION:-}"
BUILD_DIR="$(pwd)/build/"
IS_HUB_ONLY=false
IS_AGENT_ONLY=false

usage() {
  cat <<EOF
Usage: $(basename "$0") --version <tag>

Options:
  --version <tag>  Build version injected into hub and agent binaries, for example v0.1.0.
  --hub            Build hub binary only.
  --agent          Build agent binary only.
  --help           Show this help.

Environment overrides:
  VERSION
  UPX     Set to 1 to compress binaries with upx (off by default; packed
          binaries can be killed silently before main on hardened Alpine
          kernels, LXC, gVisor and other restricted environments)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:?missing version}"
      shift 2
      ;;
    --hub)
      IS_HUB_ONLY=true
      shift
      ;;
    --agent)
      IS_AGENT_ONLY=true
      shift
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
(cd deploy && zip -r "$BUILD_DIR/deployment_files.zip" . -x "*.DS_Store")

targets=(
  "linux amd64"
  "linux arm64"
  "windows amd64"
  "windows arm64"
  "darwin amd64"
  "darwin arm64"
)

apps=(
  "hub"
  "agent"
)
if [ "$IS_HUB_ONLY" = true ]; then
  apps=(
    "hub"
  )
elif [ "$IS_AGENT_ONLY" = true ]; then
  apps=(
    "agent"
  )
fi


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
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="$ldflags" -o "$out" "cmd/${app}/main.go"
    echo "Built $(basename "$out")"

    if [ "${UPX:-0}" = "1" ]; then
      if command -v upx >/dev/null 2>&1; then
        upx -qqq "$out" && echo "Compressed $(basename "$out")"
      else
        echo "UPX=1 but upx is not installed, skipping compression." >&2
      fi
    fi

    if [ "$app" = "agent" ]; then
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$out" | awk '{print $1}' > "${out}.sha256"
      elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$out" | awk '{print $1}' > "${out}.sha256"
      fi
      if [ -f "${out}.sha256" ]; then
        echo "Wrote $(basename "${out}.sha256")"
      fi
    fi
  done
done

echo "Build complete. Binaries are located in the 'build' directory."
