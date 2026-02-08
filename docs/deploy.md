# Deployment to GCP VM (bill.thetravelbug.club)

## Prereqs
- gcloud CLI authenticated to project `disney-bill-splitter`
- Cloudflare DNS: A record `bill.thetravelbug.club` -> static IP `35.188.115.203`, proxy **ON**, SSL mode **Full** (not Strict)
- VM: `bill-bugs` (e2-small, us-central1-a) with Docker + docker-compose installed
- Static IP: `bill-bugs-ip`

## Update code & redeploy
> IMPORTANT: do **not** copy your local `.env` to the VM; it will break origin TLS. The VM already has the correct prod `.env`.

```bash
# from repo root, sync code only (omit .env)
gcloud compute scp --recurse backend frontend caddy docker-compose.prod.yml docker-compose.yml go.mod go.sum Makefile docs bill-bugs:~/app \
  --project disney-bill-splitter --zone us-central1-a

# SSH to VM
gcloud compute ssh bill-bugs --project disney-bill-splitter --zone us-central1-a

# on VM: rebuild/restart with current code/env
cd app
sudo docker-compose -f docker-compose.prod.yml up -d --build
```

## Environment (.env on VM)
Already present on the VM. Key values:
- OPENAI_API_KEY=<your key>
- DOMAIN=bill.thetravelbug.club
- TLS_MODE=local (Cloudflare Full mode)
- PUBLIC_BASE_URL=https://bill.thetravelbug.club
- COOKIE_SECURE=true, CORS_ALLOWED_ORIGINS=https://bill.thetravelbug.club

If you ever need to recreate `.env` on the VM:
```bash
OPENAI_KEY=... # keep secret
SESSION_SECRET=$(openssl rand -hex 32)
JOIN_KEY=$(openssl rand -hex 32)
CSRF_SECRET=$(openssl rand -hex 32)
cat > .env <<EOF
NODE_ENV=production
PUBLIC_BASE_URL=https://bill.thetravelbug.club
ROOM_TTL_SECONDS=86400

# Backend
BACKEND_PORT=8080
REDIS_URL=redis://redis:6379/0
SESSION_SECRET=$SESSION_SECRET
JOIN_TOKEN_SIGNING_KEY=$JOIN_KEY
CSRF_SECRET=$CSRF_SECRET
COOKIE_SECURE=true
COOKIE_DOMAIN=bill.thetravelbug.club
CORS_ALLOWED_ORIGINS=https://bill.thetravelbug.club
OPENAI_API_KEY=$OPENAI_KEY
PUBLIC_BASE_URL=https://bill.thetravelbug.club

# Frontend
VITE_API_BASE_URL=https://bill.thetravelbug.club/api
VITE_WS_BASE_URL=wss://bill.thetravelbug.club/ws

# Proxy / TLS
DOMAIN=bill.thetravelbug.club
LETSENCRYPT_EMAIL=austinchi2@yahoo.com
TLS_MODE=local
EOF
```
Then:
```bash
sudo docker-compose -f docker-compose.prod.yml down
sudo docker-compose -f docker-compose.prod.yml up -d
```

## Checking status
```bash
gcloud compute ssh bill-bugs --project disney-bill-splitter --zone us-central1-a --command "cd app && sudo docker-compose -f docker-compose.prod.yml ps"
```

## Logs
```bash
sudo docker-compose -f docker-compose.prod.yml logs -f backend
sudo docker-compose -f docker-compose.prod.yml logs -f frontend
sudo docker-compose -f docker-compose.prod.yml logs -f caddy
```

## Notes
- Cloudflare must stay in **Full** SSL mode (proxy on). Strict will fail with the self-signed origin cert.
- Caddy health may show “starting” briefly after restart; give ~30s.
- To switch to Let’s Encrypt direct-to-origin later, disable proxy temporarily and set TLS_MODE=letsencrypt.
