# Multiplayer Bill Splitter

A production-ready MVP of a multiplayer bill-splitting app with a SvelteKit + Skeleton UI frontend, Go backend, Redis persistence, WebSocket realtime sync, and receipt parsing via OpenAI vision.

## Quick start (local)

```bash
cp .env.example .env
./scripts/gen-env.sh
# edit .env to add OPENAI_API_KEY

docker compose up --build
```

Open **https://localhost**. Local HTTPS is handled via Caddy's internal CA; your browser will warn until you trust the cert.

## Deploy to a VM

1. Set the following in `.env`:
   - `DOMAIN=yourdomain.com`
   - `LETSENCRYPT_EMAIL=you@yourdomain.com`
   - `PUBLIC_BASE_URL=https://yourdomain.com`
   - `TLS_MODE=letsencrypt`
2. Open firewall ports **80** and **443**.
3. Run:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

Healthchecks:
- `https://yourdomain.com/api/health`

### No-domain / IP-only mode

Set `TLS_MODE=selfsigned` and `DOMAIN=<public-ip>`. Caddy will serve a self-signed cert; browsers will warn. For HTTP-only deployments, set `COOKIE_SECURE=false` and terminate TLS upstream.

## Security notes

- `.env` must not be committed. Use `scripts/gen-env.sh` to generate strong secrets.
- Rotate `SESSION_SECRET`, `JOIN_TOKEN_SIGNING_KEY`, and `CSRF_SECRET` by re-running the generator and restarting containers.
- Receipt images are sent to OpenAI for parsing; inform users and handle privacy accordingly.

## Architecture

See [docs/architecture.md](docs/architecture.md) for CRDT, WebSocket schema, Redis keys, and math rules.
