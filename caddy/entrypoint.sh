#!/usr/bin/env sh
set -e

TLS_MODE=${TLS_MODE:-local}
TLS_DIRECTIVE="internal"
DOMAIN=${DOMAIN:-localhost}

case "$TLS_MODE" in
  letsencrypt)
    if [ -z "$LETSENCRYPT_EMAIL" ]; then
      echo "LETSENCRYPT_EMAIL is required for letsencrypt mode" >&2
      exit 1
    fi
    TLS_DIRECTIVE="$LETSENCRYPT_EMAIL"
    ;;
  selfsigned|local)
    TLS_DIRECTIVE="internal"
    ;;
  *)
    echo "Unknown TLS_MODE: $TLS_MODE" >&2
    exit 1
    ;;
 esac

sed -e "s/{{TLS_DIRECTIVE}}/${TLS_DIRECTIVE}/" -e "s/{{DOMAIN}}/${DOMAIN}/" /etc/caddy/Caddyfile.template > /etc/caddy/Caddyfile

exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
