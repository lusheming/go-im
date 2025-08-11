#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scale.sh 5

REPLICAS="${1:-3}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_BASE="$ROOT_DIR/docker/docker-compose.yml"
COMPOSE_MULTI="$ROOT_DIR/docker/docker-compose.multi.yml"

cd "$ROOT_DIR/docker"

echo "[scale] Scaling app to $REPLICAS replicas..."
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_MULTI" up -d --scale app="$REPLICAS"

echo "[scale] Done. Current services:"
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_MULTI" ps 