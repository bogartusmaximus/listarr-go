#!/usr/bin/env bash
# Build and start listarr-go + Tailscale Serve on the ci-development runner.
# Host: autobot-development · User: github-runner
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_BASE="${LISTARR_COMPOSE_BASE:-compose.yaml}"
COMPOSE_OVERLAY="${LISTARR_COMPOSE_OVERLAY:-compose.ci-development.yaml}"

cd "$PROJECT_ROOT"
"$SCRIPT_DIR/write-development-secrets.sh"

if [[ ! -f "$COMPOSE_BASE" || ! -f "$COMPOSE_OVERLAY" ]]; then
  echo "Missing $COMPOSE_BASE or $COMPOSE_OVERLAY" >&2
  exit 1
fi

docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_OVERLAY" up -d --build
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_OVERLAY" ps
docker compose -f "$COMPOSE_BASE" -f "$COMPOSE_OVERLAY" logs --tail=40 listarr
