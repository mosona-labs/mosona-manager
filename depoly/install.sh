#!/bin/bash
set -euo pipefail

GITHUB_OWNER=mosona-labs
GITHUB_REPO=mosona-manager
RELEASE_TAG=latest
BACKEND_ASSET_AMD64=hub_linux_amd64
BACKEND_ASSET_ARM64=hub_linux_arm64
FRONTEND_ZIP_ASSET=frontend_dist.zip
DEPLOY_ZIP_ASSET=deployment_files.zip

check_docker() {
  if command -v docker >/dev/null 2>&1; then
    echo "Docker is already installed: $(docker --version)"
    return 0
  fi
  return 1
}

read_yes_no() {
  local prompt="$1"
  local default="${2:-n}"
  read -r -p "$prompt" ans
  ans="${ans:-$default}"
  case "$ans" in
    [Yy]*) return 0 ;;
    *) return 1 ;;
  esac
}

install_docker() {
  echo "Installing Docker..."

  if command -v docker >/dev/null 2>&1; then
    echo "Docker already installed: $(docker --version)"
  else
    case "$(uname -s)" in
      Darwin)
        if command -v brew >/dev/null 2>&1; then
          echo "Installing Docker Desktop via Homebrew Cask..."
          brew install --cask docker || { echo "brew install failed. Please install Docker Desktop manually."; exit 1; }
          echo "Docker Desktop installed. Please start Docker.app and re-run this script."
          exit 0
        else
          echo "Homebrew not found. Please install Docker Desktop manually: https://www.docker.com/products/docker-desktop"
          exit 1
        fi
        ;;
      Linux)
        echo "Installing Docker Engine via official script..."
        curl -fsSL https://get.docker.com | sudo bash -s docker

        # enable/start docker if systemd is present
        if command -v systemctl >/dev/null 2>&1; then
          sudo systemctl enable --now docker || true
        fi
        ;;
      *)
        echo "Unsupported OS: $(uname -s). Please install Docker and Docker Compose manually."
        exit 1
        ;;
    esac
  fi

  # Ensure docker compose CLI/plugin is available (prefer Compose v2 plugin)
  if ! docker compose version >/dev/null 2>&1; then
    echo "Installing Docker Compose v2 CLI plugin..."
    sudo mkdir -p /usr/local/lib/docker/cli-plugins
    sudo curl -SL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose
    sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  fi

  # Fallback: try package-manager install for legacy `docker-compose` if still missing
  if ! command -v docker-compose >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      sudo apt-get update
      sudo apt-get install -y docker-compose-plugin || true
    elif command -v yum >/dev/null 2>&1; then
      sudo yum install -y docker-compose-plugin || true
    fi
  fi

  echo "Docker: $(docker --version 2>/dev/null || echo 'not installed')"
  echo "Compose: $(docker compose version 2>/dev/null || docker-compose --version 2>/dev/null || echo 'not installed')"
}

generate_password() {
  local len="${1:-16}"
  local pw
  if command -v openssl >/dev/null 2>&1; then
    pw="$(openssl rand -base64 $((len * 2)) 2>/dev/null | tr -dc 'A-Za-z0-9!@#%_-' | head -c "$len")"
  else
    pw="$(tr -dc 'A-Za-z0-9!@#%_-' </dev/urandom 2>/dev/null | head -c "$len")"
  fi
  if [ -z "$pw" ]; then
    pw="p@ssw0rd$(date +%s | sha256sum | head -c $((len-8)))"
  fi
  printf '%s' "$pw"
}

read_input() {
  local default=$1
  local input
  read -r -p "Enter $2 [${default}]: " input
  input="${input:-$default}"
  echo "$input"
}

main() {
  echo "Starting installation of mosona manager..."

  # App Port
  APP_PORT=$(read_input 8080 "application port")
  if [[ ! "${APP_PORT}" =~ ^[0-9]+$ ]] || (( APP_PORT < 1 || APP_PORT > 65535 )); then
    echo "Invalid port: ${APP_PORT}."
    exit 1
  fi

  # Admin Username
  echo "The admin account will be created when you open the Web UI for the first time."

  # Postgres Password
  POSTGRES_PASSWORD="$(read_input "$(generate_password 16)" "Postgres password")"

  # InfluxDB Password
  INFLUXDB_PASSWORD="$(read_input "$(generate_password 16)" "InfluxDB password")"
  # InfluxDB Token
  INFLUXDB_TOKEN="$(read_input "$(generate_password 32)" "InfluxDB token")"

  # Check for Docker installation
  if ! check_docker; then
    if read_yes_no "Docker not found. Install Docker? [y/N]: " n; then
      install_docker
    else
      echo "Please install Docker manually and re-run this script."
      exit 0
    fi
  fi

  # Install curl, jq, unzip if missing
  for cmd in curl jq unzip; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      echo "$cmd is not installed. Installing..."
      if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update
        sudo apt-get install -y "$cmd"
      elif command -v yum >/dev/null 2>&1; then
        sudo yum install -y "$cmd"
      else
        echo "Unsupported package manager. Please install $cmd manually."
        exit 1
      fi
    fi
  done

  # Fetch release metadata
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

  deployment_files_url="$(get_asset_url "$DEPLOY_ZIP_ASSET")"
  if [ -z "$deployment_files_url" ] || [ "$deployment_files_url" = "null" ]; then
    echo "Deployment files asset not found: $deployment_files_url"
    exit 1
  fi

  # Download and extract deployment files
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  cd "$tmp"
  echo "Downloading deployment files zip: $DEPLOY_ZIP_ASSET"
  curl -fL "$deployment_files_url" -o "$tmp/deployment_files.zip"
  unzip -o "$tmp/deployment_files.zip"

  cat > ".env" <<EOF
GITHUB_OWNER=${GITHUB_OWNER}
GITHUB_REPO=${GITHUB_REPO}
RELEASE_TAG=${RELEASE_TAG}
BACKEND_ASSET_AMD64=${BACKEND_ASSET_AMD64}
BACKEND_ASSET_ARM64=${BACKEND_ASSET_ARM64}
FRONTEND_ZIP_ASSET=${FRONTEND_ZIP_ASSET}
APP_PORT=${APP_PORT}
PG_DB=mm_db
PG_USER=mm_user
PG_PASSWORD=${POSTGRES_PASSWORD}
INFLUX_USER=admin
INFLUX_PASSWORD=${INFLUXDB_PASSWORD}
INFLUX_ORG=mm_org
INFLUX_BUCKET=mm_bucket
INFLUX_TOKEN=${INFLUXDB_TOKEN}
EOF

  echo "Starting services via Docker Compose..."
  docker compose up -d
  echo "Installation complete. Access the application at http://$(curl -s https://api.ip.sb/ip -A Mozilla):${APP_PORT}"
}

main