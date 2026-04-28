#!/usr/bin/env bash
set -euo pipefail

GITHUB_OWNER_DEFAULT=mosona-labs
GITHUB_REPO_DEFAULT=mosona-manager
RELEASE_TAG_DEFAULT=latest
DEPLOY_ZIP_ASSET_DEFAULT=deployment_files.zip

require_commands() {
  local cmd
  for cmd in curl jq unzip docker; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      echo "Required command not found: $cmd"
      exit 1
    fi
  done
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

fetch_deployment_files() {
  local github_owner github_repo release_tag deploy_zip_asset api rel_url json deploy_url tmp env_backup

  github_owner="$(env_or_default GITHUB_OWNER "$GITHUB_OWNER_DEFAULT")"
  github_repo="$(env_or_default GITHUB_REPO "$GITHUB_REPO_DEFAULT")"
  release_tag="$(env_or_default RELEASE_TAG "$RELEASE_TAG_DEFAULT")"
  deploy_zip_asset="$(env_or_default DEPLOY_ZIP_ASSET "$DEPLOY_ZIP_ASSET_DEFAULT")"

  api="https://api.github.com/repos/${github_owner}/${github_repo}/releases"
  if [ "$release_tag" = "latest" ]; then
    rel_url="${api}/latest"
  else
    rel_url="${api}/tags/${release_tag}"
  fi

  echo "Fetching release metadata: $rel_url"
  json="$(curl -fsSL "$rel_url")"
  deploy_url="$(echo "$json" | jq -r --arg NAME "$deploy_zip_asset" '.assets[] | select(.name==$NAME) | .browser_download_url' | head -n1)"

  if [ -z "$deploy_url" ] || [ "$deploy_url" = "null" ]; then
    echo "Deployment files asset not found: $deploy_zip_asset"
    exit 1
  fi

  tmp="$(mktemp -d)"
  env_backup=""
  if [ -f ".env" ]; then
    env_backup="$tmp/.env"
    cp ".env" "$env_backup"
  fi

  echo "Downloading deployment files zip: $deploy_zip_asset"
  curl -fL "$deploy_url" -o "$tmp/deployment_files.zip"
  unzip -o "$tmp/deployment_files.zip"

  if [ -n "$env_backup" ]; then
    cp "$env_backup" ".env"
  fi
  rm -rf "$tmp"
}

check_postgres_volume_layout() {
  local volume_name pg_versions image_version mount_target

  volume_name="$(docker compose config --volumes | awk '$0 == "pg_data" { print; exit }')"
  if [ -z "$volume_name" ]; then
    return 0
  fi
  volume_name="${COMPOSE_PROJECT_NAME:-$(basename "$PWD")}_${volume_name}"
  if ! docker volume inspect "$volume_name" >/dev/null 2>&1; then
    return 0
  fi

  pg_versions="$(
    docker run --rm -v "${volume_name}:/v" alpine sh -c \
      'find /v -maxdepth 6 -name PG_VERSION -exec cat {} \; 2>/dev/null | sort -u' || true
  )"
  if [ -z "$pg_versions" ]; then
    return 0
  fi

  image_version="$(
    docker compose config | awk '
      $0 ~ /^  postgres:$/ { in_postgres=1; next }
      in_postgres && $1 == "image:" {
        if ($2 ~ /postgres:18/) print "18";
        else if ($2 ~ /postgres:17/) print "17";
        else print $2;
        exit
      }
      in_postgres && $0 ~ /^  [a-zA-Z0-9_-]+:$/ { in_postgres=0 }
    '
  )"
  mount_target="$(
    docker compose config | awk '
      $0 ~ /^  postgres:$/ { in_postgres=1; next }
      in_postgres && $1 == "source:" && $2 == "pg_data" { in_pg_volume=1; next }
      in_postgres && in_pg_volume && $1 == "target:" { print $2; exit }
      in_postgres && $0 ~ /^  [a-zA-Z0-9_-]+:$/ { in_postgres=0; in_pg_volume=0 }
    '
  )"

  if echo "$pg_versions" | grep -q '^18$' && { [ "$image_version" != "18" ] || [ "$mount_target" != "/var/lib/postgresql" ]; }; then
    echo "Refusing to continue: pg_data contains PostgreSQL 18 data, but compose uses image version '$image_version' and target '$mount_target'."
    echo "Keep postgres:18-alpine with pg_data mounted at /var/lib/postgresql, or perform a planned dump/restore migration."
    exit 1
  fi

  if echo "$pg_versions" | grep -q '^17$' && { [ "$image_version" != "17" ] || [ "$mount_target" != "/var/lib/postgresql/data" ]; }; then
    echo "Refusing to continue: pg_data contains PostgreSQL 17 data, but compose uses image version '$image_version' and target '$mount_target'."
    echo "Keep postgres:17-alpine with pg_data mounted at /var/lib/postgresql/data, or perform a planned pg_upgrade/dump/restore migration."
    exit 1
  fi
}

wait_for_postgres() {
  docker compose exec -T postgres sh -c 'until pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"; do sleep 1; done'

  local attempt
  for attempt in $(seq 1 30); do
    if docker compose exec -T postgres sh -c 'psql -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1"' >/dev/null; then
      return 0
    fi
    echo "Postgres is not ready for SQL yet, retrying (${attempt}/30)..."
    sleep 2
  done

  echo "Postgres did not become SQL-ready. Recent logs:"
  docker compose logs --tail=80 postgres
  exit 1
}

psql_file_retry() {
  local sql_file="$1"
  local attempt
  for attempt in $(seq 1 5); do
    if docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$sql_file"; then
      return 0
    fi
    echo "psql failed, retrying (${attempt}/5)..."
    docker compose logs --tail=40 postgres
    sleep 2
  done
  return 1
}

psql_query_retry() {
  local query="$1"
  local attempt output
  for attempt in $(seq 1 5); do
    if output="$(
      docker compose exec -T postgres sh -c \
        "psql -At -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -c \"$query\"" 2>&1
    )"; then
      printf '%s' "$output"
      return 0
    fi
    echo "psql query failed, retrying (${attempt}/5): $query" >&2
    echo "$output" >&2
    docker compose logs --tail=40 postgres >&2
    sleep 2
  done
  return 1
}

run_db_migrations() {
  if [ ! -d "./postgres/migrations" ]; then
    echo "No migrations directory found, skipping file-based migrations"
    return 0
  fi

  echo "Waiting for postgres..."
  docker compose up -d postgres
  local postgres_container
  postgres_container="$(docker compose ps -q postgres)"
  if [ -z "$postgres_container" ]; then
    echo "Postgres container not found"
    exit 1
  fi

  wait_for_postgres

  local schema_migration_file
  schema_migration_file="$(mktemp)"
  cat > "$schema_migration_file" <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(255) PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
SQL
  psql_file_retry "$schema_migration_file"
  rm -f "$schema_migration_file"

  local migration version applied
  for migration in ./postgres/migrations/*.sql; do
    [ -e "$migration" ] || continue
    version="$(basename "$migration")"
    applied="$(psql_query_retry "SELECT 1 FROM schema_migrations WHERE version = '${version}'")"
    if [ "$applied" = "1" ]; then
      echo "Skipping migration: $version"
      continue
    fi

    echo "Applying migration: $version"
    psql_file_retry "$migration"
    psql_query_retry "INSERT INTO schema_migrations (version) VALUES ('${version}')" >/dev/null
  done
}

require_commands
fetch_deployment_files
check_postgres_volume_layout
docker compose up -d --no-deps --force-recreate bootstrap
docker compose logs -f bootstrap
run_db_migrations
docker compose build app
docker compose up -d --no-deps --force-recreate app
