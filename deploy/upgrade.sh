#!/usr/bin/env bash
set -euo pipefail

docker compose up -d --no-deps --force-recreate bootstrap
docker compose logs -f bootstrap
docker compose build app
docker compose up -d --no-deps --force-recreate app