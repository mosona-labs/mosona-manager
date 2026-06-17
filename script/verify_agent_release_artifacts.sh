#!/bin/bash
set -euo pipefail

BUILD_DIR="${1:-$(pwd)/build}"

expected=(
  agent_linux_amd64
  agent_linux_arm64
  agent_windows_amd64.exe
  agent_windows_arm64.exe
  agent_darwin_amd64
  agent_darwin_arm64
)

missing=0
for name in "${expected[@]}"; do
  bin="${BUILD_DIR}/${name}"
  sum="${bin}.sha256"
  if [ ! -f "$bin" ]; then
    echo "missing binary: $bin" >&2
    missing=1
    continue
  fi
  if [ ! -f "$sum" ]; then
    echo "missing checksum: $sum" >&2
    missing=1
    continue
  fi
  want=$(tr -d ' \n\r' < "$sum")
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$bin" | awk '{print $1}')
  else
    got=$(shasum -a 256 "$bin" | awk '{print $1}')
  fi
  if [ "$want" != "$got" ]; then
    echo "checksum mismatch: $bin" >&2
    missing=1
  fi
done

if [ "$missing" -ne 0 ]; then
  echo "Agent release artifacts incomplete or invalid." >&2
  exit 1
fi

echo "All agent release artifacts present and checksums match."
