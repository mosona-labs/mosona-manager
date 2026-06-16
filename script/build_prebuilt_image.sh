#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

WEB_DIR="${WEB_DIR:-${REPO_ROOT}/../mosona-manager-web}"
PUB_DIR="${PUB_DIR:-${REPO_ROOT}/../mosona-manager-pub}"
CTX="${CTX:-${REPO_ROOT}/build/prebuilt-image}"
IMAGE="${IMAGE:-ghcr.io/mosona-labs/mosona-manager}"
VERSION="${VERSION:-}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
MODE="push"
TAG_LATEST="yes"
RUN_INSTALL="no"
BUILD_FRONTEND="yes"
BUILD_PUBLIC_FRONTEND="yes"
BUILD_BACKEND="yes"

usage() {
  cat <<EOF
Usage: $(basename "$0") --version <tag> [options]

Build a prebuilt Mosona Manager Docker image containing:
  - /srv/bin/mosona-manager
  - /srv/frontend
  - /srv/frontend/public-preview

Options:
  --version <tag>       Image version tag, for example v0.1.0. Required unless VERSION is set.
  --image <image>       Image name. Default: ${IMAGE}
  --context <dir>       Build context directory. Default: ${CTX}
  --platform <list>     Buildx platform list. Default: ${PLATFORMS}
  --push                Push multi-arch image. This is the default mode.
  --load                Load a single-platform image into local Docker. Requires one platform.
  --no-latest           Do not also tag image as latest.
  --install             Run pnpm install before frontend builds.
  --skip-frontend       Reuse existing files in <context>/frontend_dist.
  --skip-public         Reuse existing files in <context>/frontend_pub.
  --skip-backend        Reuse existing hub_linux_* files in context.
  --help                Show this help.

Environment overrides:
  WEB_DIR, PUB_DIR, CTX, IMAGE, VERSION, PLATFORMS

Examples:
  script/build_prebuilt_image.sh --version v0.1.0 --push
  script/build_prebuilt_image.sh --version dev --platform linux/arm64 --load --no-latest
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:?missing version}"
      shift 2
      ;;
    --image)
      IMAGE="${2:?missing image}"
      shift 2
      ;;
    --context)
      CTX="${2:?missing context}"
      shift 2
      ;;
    --platform|--platforms)
      PLATFORMS="${2:?missing platform list}"
      shift 2
      ;;
    --push)
      MODE="push"
      shift
      ;;
    --load)
      MODE="load"
      shift
      ;;
    --no-latest)
      TAG_LATEST="no"
      shift
      ;;
    --install)
      RUN_INSTALL="yes"
      shift
      ;;
    --skip-frontend)
      BUILD_FRONTEND="no"
      shift
      ;;
    --skip-public)
      BUILD_PUBLIC_FRONTEND="no"
      shift
      ;;
    --skip-backend)
      BUILD_BACKEND="no"
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
HUB_LDFLAGS="-s -w -X mosona-manager/internal/runtime.Version=${VERSION}"

if [ "$MODE" = "load" ] && [[ "$PLATFORMS" == *,* ]]; then
  echo "--load only supports one platform. Use --platform linux/amd64 or --platform linux/arm64." >&2
  exit 1
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_cmd docker
if [ "$BUILD_BACKEND" = "yes" ]; then
  require_cmd go
fi
if [ "$BUILD_FRONTEND" = "yes" ] || [ "$BUILD_PUBLIC_FRONTEND" = "yes" ]; then
  require_cmd pnpm
fi
if [ "$BUILD_PUBLIC_FRONTEND" = "yes" ]; then
  require_cmd perl
fi

echo "Repository root: $REPO_ROOT"
echo "Build context:    $CTX"
echo "Image:            $IMAGE"
echo "Version:          $VERSION"
echo "Platforms:        $PLATFORMS"
echo "Mode:             $MODE"

if [ "$BUILD_FRONTEND" = "yes" ] && [ "$BUILD_PUBLIC_FRONTEND" = "yes" ] && [ "$BUILD_BACKEND" = "yes" ]; then
  rm -rf "$CTX"
fi
mkdir -p "$CTX/frontend_dist" "$CTX/frontend_pub"

if [ "$BUILD_FRONTEND" = "yes" ]; then
  echo "Building main frontend..."
  rm -rf "$CTX/frontend_dist"
  mkdir -p "$CTX/frontend_dist"
  if [ ! -d "$WEB_DIR" ]; then
    echo "Frontend directory not found: $WEB_DIR" >&2
    exit 1
  fi
  (
    cd "$WEB_DIR"
    if [ "$RUN_INSTALL" = "yes" ]; then
      pnpm install
    fi
    pnpm run build
  )
  if [ ! -d "$WEB_DIR/dist" ]; then
    echo "Frontend dist not found: $WEB_DIR/dist" >&2
    exit 1
  fi
  cp -a "$WEB_DIR/dist/." "$CTX/frontend_dist/"
  rm -f "$CTX/frontend_dist/.DS_Store"
fi

if [ "$BUILD_PUBLIC_FRONTEND" = "yes" ]; then
  echo "Building public preview frontend..."
  rm -rf "$CTX/frontend_pub"
  mkdir -p "$CTX/frontend_pub"
  if [ ! -d "$PUB_DIR" ]; then
    echo "Public frontend directory not found: $PUB_DIR" >&2
    exit 1
  fi
  (
    cd "$PUB_DIR"
    if [ "$RUN_INSTALL" = "yes" ]; then
      pnpm install
    fi
    pnpm run build
  )
  if [ ! -d "$PUB_DIR/dist" ]; then
    echo "Public frontend dist not found: $PUB_DIR/dist" >&2
    exit 1
  fi
  cp -a "$PUB_DIR/dist/." "$CTX/frontend_pub/"
  perl -pi -e 's#/index\.js#/preview-assets/index.js#g' "$CTX/frontend_pub/index.html"
  perl -pi -e 's#/index\.css#/preview-assets/index.css#g' "$CTX/frontend_pub/index.html"
  rm -f "$CTX/frontend_pub/favicon.svg"
  rm -rf "$CTX/frontend_pub/flags" "$CTX/frontend_pub/icons"
  rm -f "$CTX/frontend_pub/.DS_Store"
fi

if [ "$BUILD_BACKEND" = "yes" ]; then
  echo "Building backend binaries..."
  rm -f "$CTX/hub_linux_amd64" "$CTX/hub_linux_arm64"
  (
    cd "$REPO_ROOT"
    GOOS=linux GOARCH=amd64 go build -ldflags="$HUB_LDFLAGS" -o "$CTX/hub_linux_amd64" cmd/hub/main.go
    GOOS=linux GOARCH=arm64 go build -ldflags="$HUB_LDFLAGS" -o "$CTX/hub_linux_arm64" cmd/hub/main.go
  )
fi

for arch in amd64 arm64; do
  if [[ "$PLATFORMS" == *"linux/${arch}"* ]] && [ ! -x "$CTX/hub_linux_${arch}" ]; then
    echo "Missing executable backend binary: $CTX/hub_linux_${arch}" >&2
    exit 1
  fi
done
if [ ! -f "$CTX/frontend_dist/index.html" ]; then
  echo "Missing main frontend index: $CTX/frontend_dist/index.html" >&2
  exit 1
fi
if [ ! -f "$CTX/frontend_pub/index.html" ]; then
  echo "Missing public frontend index: $CTX/frontend_pub/index.html" >&2
  exit 1
fi

cat > "$CTX/Dockerfile" <<'EOF'
FROM debian:bookworm-slim

ARG TARGETARCH

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /var/lib/mosona-manager

COPY hub_linux_${TARGETARCH} /srv/bin/mosona-manager
COPY frontend_dist/ /srv/frontend/
COPY frontend_pub/ /srv/frontend/public-preview/

RUN chmod +x /srv/bin/mosona-manager

ENV HOST=0.0.0.0
ENV PORT=8080
ENV FRONTEND_DIR=/srv/frontend

EXPOSE 8080

CMD ["/srv/bin/mosona-manager"]
EOF

tags=(-t "${IMAGE}:${VERSION}")
if [ "$TAG_LATEST" = "yes" ]; then
  tags+=(-t "${IMAGE}:latest")
fi

build_args=(
  buildx build
  --platform "$PLATFORMS"
  "${tags[@]}"
)

if [ "$MODE" = "push" ]; then
  build_args+=(--push)
else
  build_args+=(--load)
fi

build_args+=("$CTX")

echo "Running docker ${build_args[*]}"
docker "${build_args[@]}"

echo "Prebuilt image build complete."
if [ "$MODE" = "push" ]; then
  echo "Pushed: ${IMAGE}:${VERSION}"
  if [ "$TAG_LATEST" = "yes" ]; then
    echo "Pushed: ${IMAGE}:latest"
  fi
else
  echo "Loaded: ${IMAGE}:${VERSION}"
fi
