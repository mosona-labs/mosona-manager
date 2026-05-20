#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/deploy/compose.yml"

FORCE=no
DRY_RUN=no

usage() {
  cat <<EOF
Usage: $(basename "$0") [--yes] [--dry-run]
  --yes      Skip confirmation and remove resources
  --dry-run  Show what would be removed but do not perform destructive actions
  --help     Show this message
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --yes|-y) FORCE=yes; shift ;;
    --dry-run) DRY_RUN=yes; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown argument: $1"; usage; exit 1 ;;
  esac
done

echo "Repository root: $REPO_ROOT"

# Resources defined in deploy/compose.yml that we will remove
CONTAINERS=(
  mosona-manager-bootstrap
  mosona-manager-postgres
  mosona-manager-redis
  mosona-manager-influxdb
  mosona-manager-app
)

VOLUMES=(
  pg_data
  redis_data
  influx_data
  app_bin
  frontend_dist
  app_data
)

IMAGES=(
  mosona-manager-app:local
)

FILES_AND_DIRS=(
  "$REPO_ROOT/mosona-manager"
  "$REPO_ROOT/build"
)

echo
echo "The following resources will be removed:"
printf "  Containers:\n"
for c in "${CONTAINERS[@]}"; do printf "    - %s\n" "$c"; done
printf "  Volumes:\n"
for v in "${VOLUMES[@]}"; do printf "    - %s\n" "$v"; done
printf "  Images:\n"
for i in "${IMAGES[@]}"; do printf "    - %s\n" "$i"; done
printf "  Files/directories:\n"
for f in "${FILES_AND_DIRS[@]}"; do printf "    - %s\n" "$f"; done
echo

if [ "$DRY_RUN" = "yes" ]; then
  echo "Dry run enabled — exiting without removing anything."
  exit 0
fi

if [ "$FORCE" != "yes" ]; then
  read -r -p "Proceed and remove the above resources? This is destructive. [y/N] " ans
  ans=${ans:-N}
  if [[ ! "$ans" =~ ^[Yy]$ ]]; then
    echo "Aborted by user. No changes made."
    exit 0
  fi
fi

echo "Stopping services using compose file if available..."
if [ -f "$COMPOSE_FILE" ]; then
  if docker compose -f "$COMPOSE_FILE" down -v --rmi local --remove-orphans >/dev/null 2>&1; then
    echo "docker compose down completed"
  else
    echo "docker compose down failed or not available; trying docker-compose..."
    docker-compose -f "$COMPOSE_FILE" down -v --rmi local --remove-orphans || true
  fi
else
  echo "Compose file not found at $COMPOSE_FILE — continuing to remove containers/volumes by name"
fi

echo "Removing containers (if any)..."
for c in "${CONTAINERS[@]}"; do
  if docker ps -a --format '{{.Names}}' | grep -xq "$c"; then
    echo "  Removing container: $c"
    docker rm -f "$c" || true
  else
    echo "  Container not present: $c"
  fi
done

echo "Removing named volumes (if present)..."
for v in "${VOLUMES[@]}"; do
  if docker volume ls --format '{{.Name}}' | grep -xq "$v"; then
    echo "  Removing volume: $v"
    docker volume rm -f "$v" || true
  else
    echo "  Volume not present: $v"
  fi
done

echo "Removing local images (if present)..."
for img in "${IMAGES[@]}"; do
  if docker images --format '{{.Repository}}:{{.Tag}}' | grep -xq "$img"; then
    echo "  Removing image: $img"
    docker rmi -f "$img" || true
  else
    echo "  Image not present: $img"
  fi
done

echo "Removing local files/directories..."
for p in "${FILES_AND_DIRS[@]}"; do
  if [ -e "$p" ]; then
    echo "  Removing: $p"
    rm -rf "$p"
  else
    echo "  Not found: $p"
  fi
done

echo "Cleanup complete. You may still have Docker images from upstream (alpine, postgres, redis, influxdb) in your local cache; remove them manually if desired."

exit 0

