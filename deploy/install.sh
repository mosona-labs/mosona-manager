#!/bin/bash
set -euo pipefail

GITHUB_OWNER=mosona-labs
GITHUB_REPO=mosona-manager
RELEASE_TAG=latest
BACKEND_ASSET_AMD64=hub_linux_amd64
BACKEND_ASSET_ARM64=hub_linux_arm64
FRONTEND_ZIP_ASSET=frontend_dist.zip
FRONTEND_PUB_ZIP_ASSET=frontend_pub.zip
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
    pw="$(openssl rand -base64 $((len * 2)) 2>/dev/null | tr -dc 'A-Za-z0-9_-' | head -c "$len")"
  else
    pw="$(tr -dc 'A-Za-z0-9_-' </dev/urandom 2>/dev/null | head -c "$len")"
  fi
  if [ -z "$pw" ]; then
    pw="passw0rd$(date +%s | cksum | tr -dc '0-9' | head -c $((len-8)))"
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

env_value() {
  local key="$1"
  local file="${2:-.env}"
  if [ ! -f "$file" ]; then
    return 1
  fi
  awk -v key="$key" '
    /^[[:space:]]*(#|$)/ { next }
    {
      line = $0
      sub(/^[[:space:]]*export[[:space:]]+/, "", line)
      eq = index(line, "=")
      if (eq == 0) {
        next
      }
      name = substr(line, 1, eq - 1)
      value = substr(line, eq + 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
      if (name != key) {
        next
      }
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if ((value ~ /^".*"$/) || (value ~ /^'\''.*'\''$/)) {
        value = substr(value, 2, length(value) - 2)
      } else {
        sub(/[[:space:]]+#.*$/, "", value)
      }
      print value
      exit
    }
  ' "$file"
}

env_or_default() {
  local key="$1"
  local default="$2"
  local file="${3:-.env}"
  local value
  value="$(env_value "$key" "$file" 2>/dev/null || true)"
  if [ -n "$value" ]; then
    printf '%s' "$value"
  else
    printf '%s' "$default"
  fi
}

public_ip() {
  local family="$1"
  local flag=""
  local ip
  case "$family" in
    4) flag="-4" ;;
    6) flag="-6" ;;
    *) return 1 ;;
  esac
  ip="$(curl "$flag" -fsS --connect-timeout 3 --max-time 5 -A Mozilla https://api.ip.sb/ip 2>/dev/null | tr -d '[:space:]' || true)"
  case "$family" in
    4)
      if [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
        printf '%s' "$ip"
      fi
      ;;
    6)
      if [[ "$ip" == *:* && "$ip" != *.* ]]; then
        printf '%s' "$ip"
      fi
      ;;
  esac
}

print_access_urls() {
  local port="$1"
  local ipv4 ipv6 printed=no
  ipv4="$(public_ip 4)"
  ipv6="$(public_ip 6)"

  echo "Installation complete."
  echo "Access the application at:"
  if [ -n "$ipv4" ]; then
    echo "  IPv4:  http://${ipv4}:${port}"
    printed=yes
  fi
  if [ -n "$ipv6" ]; then
    echo "  IPv6:  http://[${ipv6}]:${port}"
    printed=yes
  fi
  if [ "$printed" = "no" ]; then
    echo "  Local: http://127.0.0.1:${port}"
    echo "  Public IP lookup failed; use your server IP with port ${port}."
  fi
}

main() {
  echo "Starting installation of mosona manager..."

  deploy_dir="./mosona-manager"
  existing_env="${deploy_dir}/.env"

  # Intro
  echo "The admin account will be created when you open the Web UI for the first time."
  echo "Database credentials will be generated automatically."

  # App Port
  APP_PORT=$(read_input "$(env_or_default APP_PORT 8080 "$existing_env")" "application port")
  if [[ ! "${APP_PORT}" =~ ^[0-9]+$ ]] || (( APP_PORT < 1 || APP_PORT > 65535 )); then
    echo "Invalid port: ${APP_PORT}."
    exit 1
  fi

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

  GITHUB_OWNER="$(env_or_default GITHUB_OWNER "$GITHUB_OWNER" "$existing_env")"
  GITHUB_REPO="$(env_or_default GITHUB_REPO "$GITHUB_REPO" "$existing_env")"
  RELEASE_TAG="$(env_or_default RELEASE_TAG "$RELEASE_TAG" "$existing_env")"
  BACKEND_ASSET_AMD64="$(env_or_default BACKEND_ASSET_AMD64 "$BACKEND_ASSET_AMD64" "$existing_env")"
  BACKEND_ASSET_ARM64="$(env_or_default BACKEND_ASSET_ARM64 "$BACKEND_ASSET_ARM64" "$existing_env")"
  FRONTEND_ZIP_ASSET="$(env_or_default FRONTEND_ZIP_ASSET "$FRONTEND_ZIP_ASSET" "$existing_env")"
  FRONTEND_PUB_ZIP_ASSET="$(env_or_default FRONTEND_PUB_ZIP_ASSET "$FRONTEND_PUB_ZIP_ASSET" "$existing_env")"
  DEPLOY_ZIP_ASSET="$(env_or_default DEPLOY_ZIP_ASSET "$DEPLOY_ZIP_ASSET" "$existing_env")"

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
  env_backup=""
  if [ -f "$existing_env" ]; then
    env_backup="$(mktemp)"
    cp "$existing_env" "$env_backup"
  fi
  mkdir -p "$deploy_dir"
  cd "$deploy_dir"
  echo "Downloading deployment files zip: $DEPLOY_ZIP_ASSET"
  curl -fL "$deployment_files_url" -o "deployment_files.zip"
  unzip -o "deployment_files.zip"
  rm -f "deployment_files.zip"
  if [ -n "$env_backup" ]; then
    cp "$env_backup" ".env"
    rm -f "$env_backup"
  fi

  if [ -f ".env" ]; then
    echo "Existing .env found. Reusing existing database and service credentials."
  fi

  PG_DB="$(env_or_default PG_DB mm_db)"
  PG_USER="$(env_or_default PG_USER mm_user)"
  POSTGRES_PASSWORD="$(env_or_default PG_PASSWORD "$(generate_password 16)")"
  INFLUX_USER="$(env_or_default INFLUX_USER admin)"
  INFLUXDB_PASSWORD="$(env_or_default INFLUX_PASSWORD "$(generate_password 16)")"
  INFLUX_ORG="$(env_or_default INFLUX_ORG mm_org)"
  INFLUX_BUCKET="$(env_or_default INFLUX_BUCKET mm_bucket)"
  INFLUXDB_TOKEN="$(env_or_default INFLUX_TOKEN "$(generate_password 32)")"

  cat > ".env" <<EOF
GITHUB_OWNER=${GITHUB_OWNER}
GITHUB_REPO=${GITHUB_REPO}
RELEASE_TAG=${RELEASE_TAG}
BACKEND_ASSET_AMD64=${BACKEND_ASSET_AMD64}
BACKEND_ASSET_ARM64=${BACKEND_ASSET_ARM64}
FRONTEND_ZIP_ASSET=${FRONTEND_ZIP_ASSET}
FRONTEND_PUB_ZIP_ASSET=${FRONTEND_PUB_ZIP_ASSET}
DEPLOY_ZIP_ASSET=${DEPLOY_ZIP_ASSET}
APP_PORT=${APP_PORT}
PG_DB=${PG_DB}
PG_USER=${PG_USER}
PG_PASSWORD=${POSTGRES_PASSWORD}
INFLUX_USER=${INFLUX_USER}
INFLUX_PASSWORD=${INFLUXDB_PASSWORD}
INFLUX_ORG=${INFLUX_ORG}
INFLUX_BUCKET=${INFLUX_BUCKET}
INFLUX_TOKEN=${INFLUXDB_TOKEN}
EOF

  echo "Starting services via Docker Compose..."
  docker compose up -d
  print_access_urls "$APP_PORT"
}

main
