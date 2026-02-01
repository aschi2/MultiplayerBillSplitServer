#!/usr/bin/env bash
set -euo pipefail

ENV_FILE=".env"
EXAMPLE_FILE=".env.example"

if [[ ! -f "$EXAMPLE_FILE" ]]; then
  echo "Missing $EXAMPLE_FILE" >&2
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$EXAMPLE_FILE" "$ENV_FILE"
fi

ensure_secret() {
  local key="$1"
  local bytes="$2"
  if ! grep -q "^${key}=" "$ENV_FILE"; then
    echo "${key}=" >> "$ENV_FILE"
  fi
  local current
  current=$(grep "^${key}=" "$ENV_FILE" | tail -n1 | cut -d'=' -f2-)
  if [[ -z "$current" ]]; then
    local value
    value=$(openssl rand -base64 "$bytes")
    perl -0777 -i -pe "s/^${key}=.*$/${key}=${value}/m" "$ENV_FILE"
  fi
}

ensure_secret "SESSION_SECRET" 48
ensure_secret "JOIN_TOKEN_SIGNING_KEY" 48
ensure_secret "CSRF_SECRET" 48

echo "Generated secrets in $ENV_FILE"
