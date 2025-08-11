#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   APP_REPLICAS=3 ./deploy_multi_instance.sh
#
# Requires Docker + Docker Compose v2

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_BASE="$ROOT_DIR/docker/docker-compose.yml"
COMPOSE_MULTI="$ROOT_DIR/docker/docker-compose.multi.yml"
APP_REPLICAS="${APP_REPLICAS:-3}"

# Ensure docker/config.yml exists (or seed from root config.yml)
if [[ ! -f "$ROOT_DIR/docker/config.yml" ]]; then
  if [[ -f "$ROOT_DIR/config.yml" ]]; then
    echo "[deploy] Seeding docker/config.yml from root config.yml"
    cp "$ROOT_DIR/config.yml" "$ROOT_DIR/docker/config.yml"
  else
    echo "[deploy] Creating minimal docker/config.yml (you can edit later)"
    cat > "$ROOT_DIR/docker/config.yml" <<'EOF'
# Minimal config. All settings can also be provided via environment variables.
# See docker/README.md for details.
EOF
  fi
fi

# Bring up services with override (adds Nginx LB, removes app port mapping)
cd "$ROOT_DIR/docker"

echo "[deploy] Building and starting services..."
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_MULTI" up -d --build

echo "[deploy] Scaling app to $APP_REPLICAS replicas..."
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_MULTI" up -d --scale app="$APP_REPLICAS"

# Wait for health endpoint via Nginx
HEALTH_URL="http://localhost:8080/healthz"
echo "[deploy] Waiting for health: $HEALTH_URL"
tries=0
until curl -fsS "$HEALTH_URL" >/dev/null 2>&1; do
  tries=$((tries+1))
  if [[ $tries -gt 60 ]]; then
    echo "[deploy] Timeout waiting for health check at $HEALTH_URL" >&2
    docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_MULTI" ps
    exit 1
  fi
  sleep 2
  if (( tries % 10 == 0 )); then
    echo "[deploy] Still waiting... ($tries)"
  fi
  :
done

echo "[deploy] Cluster is up. Endpoints:"
echo "  UI Test:       http://localhost:8080/ui"
echo "  Web App:       http://localhost:8080/app"
echo "  Health:        $HEALTH_URL"
echo "  WebSocket:     ws://localhost:8080/ws"
echo ""
echo "[deploy] Useful commands:"
echo "  Scale:         ./docker/scale.sh 5"
echo "  Logs (nginx):  docker compose -f '$COMPOSE_BASE' -f '$COMPOSE_MULTI' logs -f nginx"
echo "  Logs (app):    docker compose -f '$COMPOSE_BASE' -f '$COMPOSE_MULTI' logs -f app"
echo "  Status:        docker compose -f '$COMPOSE_BASE' -f '$COMPOSE_MULTI' ps" 