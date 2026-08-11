#!/usr/bin/env bash
# Host: local docker host · User: your login
# Build and smoke-test the compose stack on loopback.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

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

LISTARR_API_KEY="$(
  docker compose exec -T listarr cat /data/polars/settings.json \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["apiKey"])'
)"
if [[ -z "${LISTARR_API_KEY}" ]]; then
  echo "FAIL: could not read apiKey from settings.json"
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
