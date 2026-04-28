#!/usr/bin/env bash
set -euo pipefail

run_db_migrations() {
  if [ ! -d "./postgres/migrations" ]; then
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

  docker compose exec -T postgres sh -c 'until pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"; do sleep 1; done'
  docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(255) PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
SQL

  local migration version applied
  for migration in ./postgres/migrations/*.sql; do
    [ -e "$migration" ] || continue
    version="$(basename "$migration")"
    applied="$(
      docker compose exec -T postgres sh -c \
        "psql -At -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -c \"SELECT 1 FROM schema_migrations WHERE version = '${version}'\""
    )"
    if [ "$applied" = "1" ]; then
      echo "Skipping migration: $version"
      continue
    fi

    echo "Applying migration: $version"
    docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$migration"
    docker compose exec -T postgres sh -c \
      "psql -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -c \"INSERT INTO schema_migrations (version) VALUES ('${version}')\""
  done
}

docker compose up -d --no-deps --force-recreate bootstrap
docker compose logs -f bootstrap
run_db_migrations
docker compose build app
docker compose up -d --no-deps --force-recreate app
