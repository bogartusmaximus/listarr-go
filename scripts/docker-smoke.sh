#!/usr/bin/env bash
# Host: local docker host · User: your login
# Build and smoke-test the compose stack on loopback.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -z "${LISTARR_API_KEY:-}" ]]; then
  if [[ -f .env ]]; then
    # shellcheck disable=SC1091
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
fi
if [[ -z "${LISTARR_API_KEY:-}" ]]; then
  LISTARR_API_KEY="docker-smoke-$(date +%s)-$RANDOM"
  export LISTARR_API_KEY
  echo "Using ephemeral LISTARR_API_KEY for this smoke run"
fi

docker compose up --build -d
trap 'docker compose down' EXIT

ok=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:8787/health" >/dev/null; then
    ok=1
    break
  fi
  sleep 0.5
done
if [[ "$ok" != 1 ]]; then
  docker compose logs --no-color listarr | tail -50
  echo "FAIL: health never became ready"
  exit 1
fi

curl -fsS "http://127.0.0.1:8787/health" | tee /tmp/listarr-docker-health.json
echo
curl -fsS -H "X-Api-Key: ${LISTARR_API_KEY}" \
  "http://127.0.0.1:8787/api/v1/system/status" | tee /tmp/listarr-docker-status.json
echo
curl -fsS -H "X-Api-Key: ${LISTARR_API_KEY}" \
  "http://127.0.0.1:8787/api/v1/activity" | tee /tmp/listarr-docker-activity.json
echo
echo "PASS docker smoke (health + status + activity)"
