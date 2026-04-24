# Repo Map

Complete file inventory with MD5 hashes. Run the verification script below to detect drift.

## Verification Script

```verify
#!/usr/bin/env bash
# Run from repo root: bash docs/REPO_MAP.md is not executable; copy this or run:
#   sed -n '/^```verify/,/^```$/p' docs/REPO_MAP.md | sed '1d;$d' | bash
cd "$(git rev-parse --show-toplevel)" || exit 1
errors=0
while IFS='|' read -r expected path; do
  [ -z "$expected" ] && continue
  [[ "$expected" =~ ^# ]] && continue
  path=$(echo "$path" | xargs)
  expected=$(echo "$expected" | xargs)
  if [ ! -f "$path" ]; then
    echo "MISSING: $path"
    errors=$((errors + 1))
    continue
  fi
  actual=$(md5 -q "$path" 2>/dev/null || md5sum "$path" | awk '{print $1}')
  if [ "$actual" != "$expected" ]; then
    echo "MISMATCH: $path (expected $expected, got $actual)"
    errors=$((errors + 1))
  fi
done <<'HASHEOF'
9a3505de6ca2fb5301431c0c26e46f7a|./.claude/launch.json
794d2b11b70931ee9cd6632605e7fa20|./.claude/settings.local.json
c16763fe0dc8797b9a562fcda9af1732|./.dockerignore
881c91cfbfa39e0b936ca7a4c2cbf672|./.env.example
44e5591ae360fa99e4d696ec95aa3c9e|./.gitignore
cf6ed81bc1d2f82b6c18f35c07942e73|./Makefile
61b43cae2bdc9690a1ea927a53801402|./backend/Dockerfile
29db630e7392ba73aa7ab31f26d57825|./backend/cmd/server/main.go
76e9306f7752bfdbb37e422e2ef8f6c8|./backend/internal/crdt/apply.go
a0f215ec3b4477bee69bd92f231fdbce|./backend/internal/crdt/apply_test.go
df722e75b2d2a16519d39dc3db29019a|./backend/internal/crdt/types.go
5d66070a3bc6eec39564025d0258b010|./backend/internal/redisstore/store.go
c7025d9fdc6cbf520d3a4bb3b8c478ff|./backend/internal/server/config.go
3606b7a0cd41243409bbe2b934d2b3f6|./backend/internal/server/currency.go
882f3ecdad7cdc70c48361c15f94deaf|./backend/internal/server/fx.go
270dad65648ca93db0eed040d5f8092a|./backend/internal/server/hub.go
6d719869c815aa8813f938f00c1dc668|./backend/internal/server/hub_sort_order_test.go
8b3e84acae1b34bb6afb8c64345c0f99|./backend/internal/server/openai.go
380b5f1f5ea28b9d60d081790a69c046|./backend/internal/server/receipt_gemini3_bakeoff_test.go
9a7c2286d8ebbd78157c9aadc8d3fca0|./backend/internal/server/receipt_groundtruth_eval_test.go
4aa99ebe843419243ade3beb35048688|./backend/internal/server/receipt_modifier_tagging_test.go
aacbeb620449931e4f7aeec20b77ba7f|./backend/internal/server/receipt_normalization_test.go
a71fd1d1b8328e689db55de8f2b89762|./backend/internal/server/receipt_parse_fallback_test.go
6a1a4d65b704b5358bc60228b2d34213|./backend/internal/server/receipt_simple_eval_test.go
e36bc8617ece485daade213ecf56ecac|./backend/internal/server/server.go
13d8b1d28b3f184b771d114fb540d48a|./caddy/Caddyfile.template
79e68e04472ddbb9bfb54623668e6818|./caddy/entrypoint.sh
a0006a12601e00c092b8677c186f7ef2|./docker-compose.prod.yml
6b932b7cd6f4fd532617724f1ba278fe|./docker-compose.yml
cdf6607905ffc65e1b528170edbca469|./docs/agent-memory/deployment.md
0787914c856c2bba4a5a47b8803b89e4|./docs/agent-memory/frontend-ui.md
67c397d1b1d2d4c7543ee0c2b634e4b8|./docs/agent-memory/payments-sharing.md
8e9617cb62b96f3045346913f5a8c11c|./docs/agent-memory/receipt-parsing.md
bc65dc20b7eae2ab04865fd7f25c061c|./docs/architecture.md
fcf708095066a7e36b16c007318519ac|./docs/deploy.md
7137d5245ecafdc841f6af8f98d078dc|./frontend/Dockerfile
f593f4c84a43d84f625c05a55efda832|./frontend/package.json
86e9e20e4c60281535291ad9a3b993f6|./frontend/postcss.config.cjs
3e589b93fc4a99a5803d0eb84f823c95|./frontend/src/app.css
85e0000a246cf3cf1efa66211ca5b183|./frontend/src/app.d.ts
125ac15a3fe0b735107fc396c7d6862d|./frontend/src/app.html
84a66578562514c279500c386886880b|./frontend/src/lib/api.ts
0973cfc2cf72407697b04d977a56ef31|./frontend/src/lib/billHistory.ts
deab85c2063e0a2e7dbe60645e715b2a|./frontend/src/lib/billLogic.test.ts
82161737cc51729b31bfa4ea5fdf7c4f|./frontend/src/lib/billLogic.ts
285623b7f0a69408aaf0c75f94b513e7|./frontend/src/lib/components/Avatar.svelte
dbb2c9b9727116496e978c569221d37a|./frontend/src/lib/components/ContactsModal.svelte
a528e71d530774bf83fb8df29288abbf|./frontend/src/lib/components/ItemEditorFields.svelte
d3470aeaca22aeacf68e9855b827d1f1|./frontend/src/lib/components/ItemPricingEditor.svelte
16943747fbdb3ba5f535f05b318b3657|./frontend/src/lib/components/ReceiptCropModal.svelte
19f58d73b162382a2b6ef6343e434d8f|./frontend/src/lib/contacts.ts
e9dad5e7f679f7fb231f06c98ce0aa06|./frontend/src/lib/currency.ts
1fb214e3ff71d28c2dd3a82e3f16722f|./frontend/src/lib/friendGroups.ts
c01cec0564d7184aebc6c2e6755085e9|./frontend/src/lib/identityPrefs.ts
be1ca97f7ac57ac8906f36b23ea9f65b|./frontend/src/lib/types.ts
6168b1fc4d682e99de93d0e94846fe8c|./frontend/src/lib/utils.ts
15eaef6422a8a1f755ff7c83daeade07|./frontend/src/routes/+layout.svelte
b2d8bb30daee7c69ba3462188d7ee8ed|./frontend/src/routes/+page.svelte
fc03f68d8b6b0042066a7f60602eb7ef|./frontend/src/routes/room/[roomCode]/+page.svelte
49325f4a3fa480575c3ed607e23655ad|./frontend/src/routes/room/[roomCode]/+page.ts
bd713fdf5ca515d3b74a46d26d9b5173|./frontend/static/brand/divvi-banner-upload.svg
9b6897a5e2f8a00d8ca4d0788dd15726|./frontend/static/brand/divvi-banner.svg
38f2c10497b5b741b6ea17c9f410cb6a|./frontend/static/brand/divvi-icon-inverted.svg
2e89cb78f325531051399e0490b50c32|./frontend/static/brand/divvi-icon.svg
2a092cfc911c4fb3ed0a0ff43c517010|./frontend/svelte.config.js
e5ad2c4c07567be0b8a00cfd3962cbbf|./frontend/tailwind.config.cjs
1d79cce86cdda36f7019465ed270f963|./frontend/tsconfig.json
6227b25644dbb7df31a7820991c9642c|./frontend/vite.config.ts
effd3da0f2e10a5b9ba3d1add40388b9|./go.mod
4d037e4686cd604563a48eee6473e9ba|./readme.md
43af8a1d54b4d68f75700b4965c39c35|./scripts/gen-env.sh
HASHEOF

if [ "$errors" -eq 0 ]; then
  echo "All files verified OK."
else
  echo "$errors file(s) failed verification."
  exit 1
fi
```

## File Inventory

### Root Config

| Hash | Path | Description |
|---|---|---|
| `c16763fe0dc8797b9a562fcda9af1732` | `.dockerignore` | Docker build exclusions (node_modules, .git, output) |
| `881c91cfbfa39e0b936ca7a4c2cbf672` | `.env.example` | Template for required environment variables with defaults |
| `44e5591ae360fa99e4d696ec95aa3c9e` | `.gitignore` | Git exclusions for build artifacts, env, node_modules |
| `cf6ed81bc1d2f82b6c18f35c07942e73` | `Makefile` | Top-level targets: `gen-env` (create .env) and `test` (Go + Svelte check) |
| `effd3da0f2e10a5b9ba3d1add40388b9` | `go.mod` | Go 1.23 module with gorilla/websocket, go-redis, google/uuid |
| `4d037e4686cd604563a48eee6473e9ba` | `readme.md` | Project overview and setup instructions |
| `43af8a1d54b4d68f75700b4965c39c35` | `scripts/gen-env.sh` | Generates .env from template, creates crypto secrets for sessions/tokens |

### Claude Config

| Hash | Path | Description |
|---|---|---|
| `9a3505de6ca2fb5301431c0c26e46f7a` | `.claude/launch.json` | Dev server launch configuration for Claude Code preview tools |
| `794d2b11b70931ee9cd6632605e7fa20` | `.claude/settings.local.json` | Local Claude Code settings (allowed commands, MCP servers) |

### Infrastructure

| Hash | Path | Description |
|---|---|---|
| `a0006a12601e00c092b8677c186f7ef2` | `docker-compose.prod.yml` | Production stack: Redis (AOF), backend, frontend, Caddy with TLS |
| `6b932b7cd6f4fd532617724f1ba278fe` | `docker-compose.yml` | Dev stack: Redis (no persistence), backend, frontend, Caddy (local TLS) |
| `13d8b1d28b3f184b771d114fb540d48a` | `caddy/Caddyfile.template` | Reverse proxy template with `{{DOMAIN}}`/`{{TLS_DIRECTIVE}}` substitution, cache headers |
| `79e68e04472ddbb9bfb54623668e6818` | `caddy/entrypoint.sh` | Substitutes env vars into Caddyfile template and starts Caddy |

### Backend — Entry Point

| Hash | Path | Description |
|---|---|---|
| `61b43cae2bdc9690a1ea927a53801402` | `backend/Dockerfile` | Multi-stage build: Go 1.23 builder → Alpine 3.20 runtime, CGO_ENABLED=0 |
| `29db630e7392ba73aa7ab31f26d57825` | `backend/cmd/server/main.go` | Server entry point: loads config, connects Redis, registers routes, starts HTTP |

### Backend — Core

| Hash | Path | Description |
|---|---|---|
| `e36bc8617ece485daade213ecf56ecac` | `backend/internal/server/server.go` | All HTTP handlers: room CRUD, receipt parse, WebSocket upgrade, CORS, helpers (~2300 lines) |
| `c7025d9fdc6cbf520d3a4bb3b8c478ff` | `backend/internal/server/config.go` | Environment variable loading with defaults for all server configuration |
| `8b3e84acae1b34bb6afb8c64345c0f99` | `backend/internal/server/openai.go` | Receipt parsing via Gemini API (despite filename): image normalization, prompt construction, JSON extraction |
| `270dad65648ca93db0eed040d5f8092a` | `backend/internal/server/hub.go` | WebSocket hub: connection lifecycle, room subscriptions, CRDT op broadcast, presence tracking |
| `882f3ecdad7cdc70c48361c15f94deaf` | `backend/internal/server/fx.go` | Foreign exchange endpoint: fetches ECB rates, 24h Redis cache |
| `3606b7a0cd41243409bbe2b934d2b3f6` | `backend/internal/server/currency.go` | Currency metadata maps (symbols, exponents, flags) and validation functions |

### Backend — CRDT

| Hash | Path | Description |
|---|---|---|
| `df722e75b2d2a16519d39dc3db29019a` | `backend/internal/crdt/types.go` | Core data structures: RoomDoc, Op, Item, Participant with tombstone maps |
| `76e9306f7752bfdbb37e422e2ef8f6c8` | `backend/internal/crdt/apply.go` | Op application logic: last-write-wins merge with timestamp-based conflict resolution |
| `5d66070a3bc6eec39564025d0258b010` | `backend/internal/redisstore/store.go` | Redis storage: atomic snapshot+ops save, sequence tracking, room TTL management |

### Backend — Tests

| Hash | Path | Description |
|---|---|---|
| `a0f215ec3b4477bee69bd92f231fdbce` | `backend/internal/crdt/apply_test.go` | CRDT apply tests: set/delete items, tombstone behavior, timestamp ordering |
| `6d719869c815aa8813f938f00c1dc668` | `backend/internal/server/hub_sort_order_test.go` | Tests for item sort order maintenance during hub operations |
| `380b5f1f5ea28b9d60d081790a69c046` | `backend/internal/server/receipt_gemini3_bakeoff_test.go` | Gemini 3 model comparison tests against receipt ground truth |
| `9a7c2286d8ebbd78157c9aadc8d3fca0` | `backend/internal/server/receipt_groundtruth_eval_test.go` | Receipt parsing accuracy evaluation against labeled test fixtures |
| `4aa99ebe843419243ade3beb35048688` | `backend/internal/server/receipt_modifier_tagging_test.go` | Tests for addon/modifier consolidation logic in parsed receipts |
| `aacbeb620449931e4f7aeec20b77ba7f` | `backend/internal/server/receipt_normalization_test.go` | Tests for receipt data normalization (backfill, addon attachment, zero-price extraction) |
| `a71fd1d1b8328e689db55de8f2b89762` | `backend/internal/server/receipt_parse_fallback_test.go` | Tests for model fallback behavior when primary parse fails |
| `6a1a4d65b704b5358bc60228b2d34213` | `backend/internal/server/receipt_simple_eval_test.go` | Simplified receipt parsing evaluation tests |
| `deab85c2063e0a2e7dbe60645e715b2a` | `frontend/src/lib/billLogic.test.ts` | Unit tests for bill calculation functions (even split, per-person costs) |

### Frontend — Config

| Hash | Path | Description |
|---|---|---|
| `7137d5245ecafdc841f6af8f98d078dc` | `frontend/Dockerfile` | Multi-stage build: npm install → dev (HMR) or prod (optimized build + Node runtime) |
| `f593f4c84a43d84f625c05a55efda832` | `frontend/package.json` | SvelteKit app deps: Skeleton UI, Tailwind, CropperJS, jsPDF, qrcode |
| `2a092cfc911c4fb3ed0a0ff43c517010` | `frontend/svelte.config.js` | SvelteKit config with adapter-node (requires Node.js runtime) |
| `e5ad2c4c07567be0b8a00cfd3962cbbf` | `frontend/tailwind.config.cjs` | Tailwind CSS with Skeleton UI theme plugin |
| `86e9e20e4c60281535291ad9a3b993f6` | `frontend/postcss.config.cjs` | PostCSS: Tailwind + Autoprefixer processing |
| `6227b25644dbb7df31a7820991c9642c` | `frontend/vite.config.ts` | Vite build config with SvelteKit plugin and jsdom test environment |
| `1d79cce86cdda36f7019465ed270f963` | `frontend/tsconfig.json` | TypeScript config extending SvelteKit defaults |

### Frontend — App Shell

| Hash | Path | Description |
|---|---|---|
| `3e589b93fc4a99a5803d0eb84f823c95` | `frontend/src/app.css` | Global styles: Tailwind directives, dark theme, glass card effects, modal animations |
| `85e0000a246cf3cf1efa66211ca5b183` | `frontend/src/app.d.ts` | TypeScript declarations for SvelteKit and Vite client types |
| `125ac15a3fe0b735107fc396c7d6862d` | `frontend/src/app.html` | HTML entry point with viewport config and gesture/zoom prevention |
| `15eaef6422a8a1f755ff7c83daeade07` | `frontend/src/routes/+layout.svelte` | Root layout applying global CSS to all pages |

### Frontend — Pages

| Hash | Path | Description |
|---|---|---|
| `b2d8bb30daee7c69ba3462188d7ee8ed` | `frontend/src/routes/+page.svelte` | Landing page: bill create/join forms, bill history with TTL countdown, contacts management |
| `fc03f68d8b6b0042066a7f60602eb7ef` | `frontend/src/routes/room/[roomCode]/+page.svelte` | Main room page: receipt import, item assignment, participant management, payment sharing (~28k lines) |
| `49325f4a3fa480575c3ed607e23655ad` | `frontend/src/routes/room/[roomCode]/+page.ts` | Server loader: extracts and normalizes room code from URL params |

### Frontend — Libraries

| Hash | Path | Description |
|---|---|---|
| `84a66578562514c279500c386886880b` | `frontend/src/lib/api.ts` | Resolves API and WebSocket base URLs from environment/browser location |
| `0973cfc2cf72407697b04d977a56ef31` | `frontend/src/lib/billHistory.ts` | Cookie-based storage of recent bill entries with TTL tracking and auto-cleanup |
| `82161737cc51729b31bfa4ea5fdf7c4f` | `frontend/src/lib/billLogic.ts` | Pure functions for bill calculations: even-split, per-person costs with tax/tip/discount |
| `19f58d73b162382a2b6ef6343e434d8f` | `frontend/src/lib/contacts.ts` | Local storage contacts and recent people with friend group integration |
| `e9dad5e7f679f7fb231f06c98ce0aa06` | `frontend/src/lib/currency.ts` | Currency metadata (symbols, exponents, flags) for 15 common currencies |
| `1fb214e3ff71d28c2dd3a82e3f16722f` | `frontend/src/lib/friendGroups.ts` | Reusable friend group creation and persistence for quick participant selection |
| `c01cec0564d7184aebc6c2e6755085e9` | `frontend/src/lib/identityPrefs.ts` | Cookie-based user identity preferences (name, Venmo username) |
| `be1ca97f7ac57ac8906f36b23ea9f65b` | `frontend/src/lib/types.ts` | TypeScript interfaces: Participant, Item, RoomDoc, and related domain models |
| `6168b1fc4d682e99de93d0e94846fe8c` | `frontend/src/lib/utils.ts` | Utility functions: initials generation, hex color conversion, currency formatting |

### Frontend — Components

| Hash | Path | Description |
|---|---|---|
| `285623b7f0a69408aaf0c75f94b513e7` | `frontend/src/lib/components/Avatar.svelte` | Circular user avatars with initials, completion status rings, configurable size/color |
| `dbb2c9b9727116496e978c569221d37a` | `frontend/src/lib/components/ContactsModal.svelte` | Modal for managing saved contacts and promoting recent people to permanent |
| `a528e71d530774bf83fb8df29288abbf` | `frontend/src/lib/components/ItemEditorFields.svelte` | Reusable form for editing item names with parent callback integration |
| `d3470aeaca22aeacf68e9855b827d1f1` | `frontend/src/lib/components/ItemPricingEditor.svelte` | Form for item pricing: quantity, unit price, line total, discount with mode toggles |
| `16943747fbdb3ba5f535f05b318b3657` | `frontend/src/lib/components/ReceiptCropModal.svelte` | Receipt image crop modal with Cropper.js and free rotation slider |

### Frontend — Brand Assets

| Hash | Path | Description |
|---|---|---|
| `9b6897a5e2f8a00d8ca4d0788dd15726` | `frontend/static/brand/divvi-banner.svg` | "The Divvi" wordmark banner for landing page header |
| `bd713fdf5ca515d3b74a46d26d9b5173` | `frontend/static/brand/divvi-banner-upload.svg` | "The Divvi" wordmark variant for upload/import screens |
| `2e89cb78f325531051399e0490b50c32` | `frontend/static/brand/divvi-icon.svg` | App icon (dark background variant) |
| `38f2c10497b5b741b6ea17c9f410cb6a` | `frontend/static/brand/divvi-icon-inverted.svg` | App icon (light/inverted variant) |

### Documentation

| Hash | Path | Description |
|---|---|---|
| `bc65dc20b7eae2ab04865fd7f25c061c` | `docs/architecture.md` | System architecture: service topology, data flow, CRDT sync model |
| `fcf708095066a7e36b16c007318519ac` | `docs/deploy.md` | Deployment runbook: VM setup, Docker build, Cloudflare/TLS configuration |
| `cdf6607905ffc65e1b528170edbca469` | `docs/agent-memory/deployment.md` | Agent memory: deploy procedures, VM ops, docker/caddy health notes |
| `0787914c856c2bba4a5a47b8803b89e4` | `docs/agent-memory/frontend-ui.md` | Agent memory: Svelte/mobile/modals/forms UX conventions |
| `67c397d1b1d2d4c7543ee0c2b634e4b8` | `docs/agent-memory/payments-sharing.md` | Agent memory: Venmo deep links, QR/share, payment-note formatting |
| `8e9617cb62b96f3045346913f5a8c11c` | `docs/agent-memory/receipt-parsing.md` | Agent memory: receipt parser behavior, model config, prompt conventions |
