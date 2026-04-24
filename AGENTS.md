# The Divvi

Real-time collaborative bill splitter: scan receipt, assign items, split costs via WebSocket CRDT sync.

## Workflow Instructions

**Use Context7:** Always use the context7 MCP server to fetch current docs when working with any library, framework, or SDK -- even well-known ones. Training data may not reflect recent changes.

**File Integrity Checks:** Before starting work, verify files match their hashes in `docs/REPO_MAP.md`.
Hashing command: `md5 -q <filepath>` (macOS) or `md5sum <filepath> | awk '{print $1}'` (Linux).
If a file's hash doesn't match, reanalyze it and update both its description and hash in `docs/REPO_MAP.md`.

**Feature Development Workflow:** After every new feature:
1. Test end-to-end -- verify it fulfills all requirements
2. Bug sweep -- run all tests, fix ALL bugs (not just high/critical)
3. Repeat sweeps until clean
4. E2E retest -- confirm nothing regressed
5. Update `docs/REPO_MAP.md` -- new descriptions and hashes for all touched files

## Commands

```
make test              # Run Go backend tests + Svelte check
make gen-env           # Generate .env from template with crypto secrets
cd frontend && npm run dev       # Dev server with HMR on port 5173
cd frontend && npm run build     # Production SvelteKit build
cd frontend && npm run check     # Svelte type checking
docker-compose up                # Full dev stack (backend + frontend + redis + caddy)
docker-compose -f docker-compose.prod.yml up -d --build   # Production deploy
```

## Gotchas

- `openai.go` uses **Gemini API**, not OpenAI. `OPENAI_API_KEY` env var is unused; check `GEMINI_API_KEY`.
- Join token validation is **disabled** if `JOIN_TOKEN_SIGNING_KEY` is unset (silently accepts any token).
- Never copy local `.env` to the VM -- it breaks origin TLS. Prod `.env` lives on the VM only.
- Use tar-over-SSH for deploys, not plain scp -- bracket paths like `[roomCode]` get mangled.
- `rand.Seed()` is called per-invocation in `randomCode()` -- parallel room creates can collide.
- Room codes exclude I/O/0/1 to avoid confusion: alphabet is `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`.
- Frontend uses `adapter-node` (requires Node.js runtime), not static adapter.
- Redis rooms expire after `ROOM_TTL_SECONDS` (default 24h) -- hard delete, no recovery.

## On-Demand Memory Docs

- `docs/agent-memory/deployment.md` -- deploys, VM ops, docker/caddy health, sync issues
- `docs/agent-memory/receipt-parsing.md` -- receipt import/parser/model/prompt behavior
- `docs/agent-memory/frontend-ui.md` -- Svelte/mobile/modals/forms/timer UX
- `docs/agent-memory/payments-sharing.md` -- Venmo deep links, QR/share, payment-note formatting

## Key References

| Topic | Location |
|---|---|
| File inventory & hashes | [docs/REPO_MAP.md](docs/REPO_MAP.md) |
| Architecture overview | [docs/architecture.md](docs/architecture.md) |
| Deployment runbook | [docs/deploy.md](docs/deploy.md) |
| Scalability roadmap | [docs/SCALABILITY_ROADMAP.md](docs/SCALABILITY_ROADMAP.md) |

## Quick Reference

- **Stack:** Go 1.23 backend, SvelteKit frontend (adapter-node), Redis 7, Caddy 2, Docker Compose
- **Receipt OCR:** Gemini `gemini-2.5-flash-lite` (primary), `gemini-3.1-pro-preview` (retry)
- **Sync model:** CRDT with last-write-wins + tombstones, ops stored in Redis lists
- **Auth model:** Room code only (~32-bit entropy). Join tokens optional.
- **Domain:** thedivvi.com (Cloudflare DNS proxy, Full SSL mode)
