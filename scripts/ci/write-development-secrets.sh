#!/usr/bin/env bash
# Materialize gitignored .env and tailscale.env for ci-development deploy.
#
# Priority:
#   1. GitHub Actions secret CI_DEVELOPMENT_LISTARR_SECRETS_ENV
#   2. Host files under /opt/listarr-go-config/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENV_FILE="$PROJECT_ROOT/.env"
TAILSCALE_FILE="$PROJECT_ROOT/tailscale.env"
HOST_DIR="${LISTARR_HOST_CONFIG:-/opt/listarr-go-config}"
HOST_ENV="${HOST_DIR}/.env"
HOST_TAILSCALE="${HOST_DIR}/tailscale.env"

cd "$PROJECT_ROOT"

remove_stray_dir() {
  local path="$1"
  if [[ -d "$path" ]]; then
    echo "Removing stray directory at $path (expected a file)" >&2
    rm -rf "$path"
  fi
}

write_env_file() {
  if [[ -n "${CI_DEVELOPMENT_LISTARR_SECRETS_ENV:-}" ]]; then
    printf '%s\n' "$CI_DEVELOPMENT_LISTARR_SECRETS_ENV" >"$ENV_FILE"
  elif [[ -f "$HOST_ENV" ]]; then
    cp "$HOST_ENV" "$ENV_FILE"
  else
    : >"$ENV_FILE"
  fi
  chmod 600 "$ENV_FILE"
}

write_tailscale_env() {
  if [[ -f "$HOST_TAILSCALE" ]]; then
    cp "$HOST_TAILSCALE" "$TAILSCALE_FILE"
  elif grep -q '^TS_AUTHKEY=.' "$ENV_FILE" 2>/dev/null; then
    grep '^TS_AUTHKEY=' "$ENV_FILE" >"$TAILSCALE_FILE"
  else
    echo "No TS_AUTHKEY. Set CI_DEVELOPMENT_LISTARR_SECRETS_ENV or ${HOST_TAILSCALE}" >&2
    exit 1
  fi
  chmod 600 "$TAILSCALE_FILE"
}

remove_stray_dir "$ENV_FILE"
remove_stray_dir "$TAILSCALE_FILE"
write_env_file
write_tailscale_env
echo "Wrote development .env and tailscale.env"
