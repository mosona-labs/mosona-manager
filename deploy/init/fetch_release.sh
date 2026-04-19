#!/bin/sh
set -eu

apk add --no-cache curl jq unzip ca-certificates tar

: "${GITHUB_OWNER:?missing}"
: "${GITHUB_REPO:?missing}"
: "${RELEASE_TAG:?missing}"
: "${FRONTEND_ZIP_ASSET:?missing}"
: "${FRONTEND_PUB_ZIP_ASSET:?missing}"

OUT_BIN="/out/bin"
OUT_FE="/out/frontend"

mkdir -p "$OUT_BIN" "$OUT_FE"

arch="$(uname -m)"
case "$arch" in
  x86_64) backend_asset="${BACKEND_ASSET_AMD64:?missing BACKEND_ASSET_AMD64}" ;;
  aarch64|arm64) backend_asset="${BACKEND_ASSET_ARM64:?missing BACKEND_ASSET_ARM64}" ;;
  *)
    echo "Unsupported arch: $arch"
    exit 1
    ;;
esac

api="https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases"
if [ "$RELEASE_TAG" = "latest" ]; then
  rel_url="${api}/latest"
else
  rel_url="${api}/tags/${RELEASE_TAG}"
fi

echo "Fetching release metadata: $rel_url"
json="$(curl -fsSL "$rel_url")"

get_asset_url() {
  name="$1"
  echo "$json" | jq -r --arg NAME "$name" '.assets[] | select(.name==$NAME) | .browser_download_url' | head -n1
}

backend_url="$(get_asset_url "$backend_asset")"
frontend_url="$(get_asset_url "$FRONTEND_ZIP_ASSET")"
frontend_pub_url="$(get_asset_url "$FRONTEND_PUB_ZIP_ASSET")"

if [ -z "$backend_url" ] || [ "$backend_url" = "null" ]; then
  echo "Backend asset not found: $backend_asset"
  exit 1
fi
if [ -z "$frontend_url" ] || [ "$frontend_url" = "null" ]; then
  echo "Frontend asset not found: $FRONTEND_ZIP_ASSET"
  exit 1
fi
if [ -z "$frontend_pub_url" ] || [ "$frontend_pub_url" = "null" ]; then
  echo "Frontend pub asset not found: $FRONTEND_PUB_ZIP_ASSET"
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading backend: $backend_asset"
curl -fL "$backend_url" -o "$tmp/backend"

echo "Downloading frontend zip: $FRONTEND_ZIP_ASSET"
curl -fL "$frontend_url" -o "$tmp/frontend.zip"

echo "Downloading frontend pub zip: $FRONTEND_PUB_ZIP_ASSET"
curl -fL "$frontend_pub_url" -o "$tmp/frontend_pub.zip"

install -m 0755 "$tmp/backend" "$OUT_BIN/mosona-manager"

rm -rf "$OUT_FE"/*
unzip -q "$tmp/frontend.zip" -d "$OUT_FE"
unzip -q "$tmp/frontend_pub.zip" -d "$OUT_FE/public-preview"

echo "Bootstrap done."
ls -la "$OUT_BIN"
ls -la "$OUT_FE" | head
