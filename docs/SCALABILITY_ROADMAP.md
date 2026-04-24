# Scalability Roadmap: Frontend, Backend, and Redis

## Context
The Divvi runs on a single GCP e2-small VM (2 shared vCPU, 2GB RAM) with Docker Compose: Go backend, SvelteKit frontend (adapter-node), Redis 7, and Caddy. Cloudflare proxies DNS. There are no resource limits, no rate limiting, no upload size limits, no monitoring, and the WebSocket hub is process-local (can't run multiple backend instances). The goal is to scale incrementally from ~100 concurrent users to 10,000+.

---

## Phase 1: Harden Current Infrastructure ($0/month added, 1-2 weeks)

Quick wins — all software changes on the existing VM.

### 1.1 Upload size limit
- **File:** `backend/internal/server/server.go` (line ~354)
- Add `http.MaxBytesReader(w, r.Body, 10<<20)` before `io.ReadAll` in receipt parse handler
- Add `r.ParseMultipartForm(10 << 20)` with 413 error response

### 1.2 Rate limiting
- **File:** `backend/internal/server/server.go`
- Add `golang.org/x/time/rate` per-IP limiter using `sync.Map`
- Receipt parse: 5 req/min, room create: 10 req/min, WS upgrade: 20/min
- Wrap in `Routes()` as middleware

### 1.3 Concurrent upload semaphore
- **File:** `backend/internal/server/server.go`
- Add `receiptSem chan struct{}` (cap 3-5) to Server struct
- Acquire before image decode, release after Gemini response
- Prevents memory spikes from concurrent large image processing

### 1.4 Redis memory limits
- **File:** `docker-compose.prod.yml`
- Add `--maxmemory 512mb --maxmemory-policy allkeys-lru` to Redis command
- Add `deploy.resources.limits.memory: 768M`

### 1.5 Container resource limits
- **File:** `docker-compose.prod.yml`
- Backend: 768M RAM, 1.0 CPU
- Frontend: 256M RAM, 0.5 CPU
- Prevents any container from starving others

### 1.6 Ops list trimming
- **File:** `backend/internal/redisstore/store.go`
- In `SaveSnapshot()`, add `LTrim` to clear ops already baked into snapshot
- Prevents unbounded list growth for active rooms

### 1.7 WebSocket connection limits
- **File:** `backend/internal/server/hub.go`
- Global max: 500 connections (atomic counter)
- Per-room max: 50 connections
- Reject upgrades beyond limits with 503

### 1.8 Basic metrics endpoint
- **File:** `backend/internal/server/server.go`
- Add `/api/metrics` returning: active WS connections, active rooms, goroutine count, alloc bytes
- Log limits for all containers (json-file, 10m, 3 files)

**Phase 1 capacity: ~200-500 concurrent users**

---

## Phase 2: Vertical Scale + Static Frontend ($15-30/month added, 2-4 weeks)

### 2.1 Upgrade VM
- e2-small → e2-medium (1 dedicated vCPU, 4GB RAM, ~$25/month)
- Doubles RAM for WS connections + image processing

### 2.2 Static frontend export (eliminate Node.js server)
- **File:** `frontend/svelte.config.js` — switch `adapter-node` → `adapter-static` with `fallback: 'index.html'`
- **File:** `frontend/Dockerfile` — replace prod stage with simple copy of `build/` dir
- **File:** `caddy/Caddyfile.template` — serve static files from mounted volume instead of proxying to `frontend:3000`
- **File:** `docker-compose.prod.yml` — remove frontend service, use build-only container + shared volume
- Frees ~200MB RAM, eliminates a running container

### 2.3 Client-side image resize before upload
- **File:** `frontend/src/routes/room/[roomCode]/+page.svelte`
- Before upload, resize to max 1600px longest edge, JPEG quality 85
- Reduces upload size 5-10x, drastically reduces server memory per parse
- Single biggest improvement for receipt parsing scalability

### 2.4 Cloudflare cache rules (free tier)
- `/_app/immutable/*` — Cache Everything, edge TTL 1 year
- `/_app/*` — Cache Everything, edge TTL 5 min
- Offloads all static asset serving to CDN edge

### 2.5 Graceful shutdown
- **File:** `backend/cmd/server/main.go`
- Replace bare `http.ListenAndServe` with `http.Server` + signal handling
- 30s drain timeout for WebSocket connections during deploys

**Phase 2 capacity: ~1,000-2,000 concurrent users**

---

## Phase 2.5: Density Optimizations — 50k connections on a single box (1-2 weeks, ~$50-100/month)

Before sharding, maximize what a single backend can handle. These optimizations push a single instance from ~1-2k to ~50k concurrent WebSocket connections (~10-20k active bills at 5 people/bill).

### 2.5.1 WebSocket buffer tuning
- **File:** `backend/internal/server/hub.go`
- gorilla/websocket defaults to 4KB read + 4KB write buffers per connection
- Divvi ops are small JSON (~200 bytes). Set `ReadBufferSize: 1024, WriteBufferSize: 1024`
- Saves ~6KB/connection → at 50k connections = 300MB saved

### 2.5.2 VM upgrade for density
- e2-standard-2 (2 vCPU, 8GB RAM) at ~$50/month handles 50k WS connections:
  - WS buffers: 50k × 2KB = 100MB
  - Goroutines: 100k × 4KB = 400MB
  - Redis: ~500MB for 20k rooms
  - Image processing (3 concurrent): 150MB
  - Go runtime: 100MB
  - **Total: ~1.3GB — fits easily in 8GB**

### 2.5.3 File descriptor limits
- **File:** `backend/Dockerfile` or `docker-compose.prod.yml`
- Set `ulimit -n 200000` in container (default ~1024 is too low for 50k connections)
```yaml
backend:
  ulimits:
    nofile:
      soft: 200000
      hard: 200000
```

### 2.5.4 Go runtime tuning
- **File:** `docker-compose.prod.yml` (environment)
- `GOMAXPROCS=2` (match vCPU count, avoids scheduler overhead)
- `GOGC=200` (less aggressive GC — trade RAM for lower CPU, fine when RAM is plentiful)

**Phase 2.5 capacity: ~50,000 concurrent connections (~10-20k active bills) on a single $50/month VM**

---

## Phase 3: Horizontal Scaling — Sharded Architecture (cloud-agnostic, 4-8 weeks)

Design principle: **Shard by room. Each backend owns a set of rooms with its own Redis. Zero cross-instance communication. Shard index encoded in room code — no shared state, no override table, no SPOF.**

### 3.1 Architecture: Room-Sharded Backend + Redis Pairs

```
         ┌───────────┐
         │ Cloudflare│  ← edge: DDoS, SSL, static cache
         └─────┬─────┘
               ▼
         ┌───────────┐
         │  Caddy /  │  ← extracts shard index from room code
         │  Ingress  │
         └─────┬─────┘
               │ route by shard prefix in room code
    ┌──────────┼──────────┐
    ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐
│backend0│ │backend1│ │backend2│   ← each handles ~50k conn
└───┬────┘ └───┬────┘ └───┬────┘
    ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐
│redis-0 │ │redis-1 │ │redis-2 │   ← independent, no cross-talk
└────────┘ └────────┘ └────────┘
```

- **No pub/sub, no fan-out, no shared state** — each room lives on exactly one shard
- All users in a room hit the same backend → same Redis → local WS hub works as-is
- Backend code stays almost unchanged (hub.go, broadcast, presence all remain process-local)
- Scaling = add more backend+Redis pairs

### 3.2 Shard-in-room-code routing (eliminates override table SPOF)

**Key design decision:** Encode the shard index as a 2-character hex prefix in the room code.

- Room code format: `{2-char hex shard}{6-char alphanumeric}` (e.g., `00XYZQRS`, `0AABCDEF`, `FFMNOPQR`)
- 2 hex digits = **256 possible shards** (00-FF)
- At 50k connections/shard, that's 12.8M concurrent users — well beyond any realistic scale
- Router extracts first 2 characters as hex shard index → routes to correct backend
- **Zero shared state needed** — no override table, no shared Redis, no SPOF
- **Zero-downtime resharding** — see 3.4 below
- Room codes are 8 characters total (2 shard + 6 random) — still short enough to type/share

**Shard assignment strategy:**
- Each backend has `SHARD_INDEX` env var (hex: `00`, `0A`, `FF`, etc.)
- But shards don't map 1:1 to shard indices — a backend can own MULTIPLE shard prefixes
- With 3 backends serving 256 prefixes: backend-0 owns `00-55`, backend-1 owns `56-AA`, backend-2 owns `AB-FF`
- **Rebalancing = reassigning prefix ranges** without changing room codes
- Router maintains a prefix→backend mapping (static config or ConfigMap)

**File changes:**
- `backend/internal/server/server.go` — `handleCreateRoom()` picks a random prefix from its assigned range, prepends to room code
- `caddy/Caddyfile.template` — route based on 2-char hex prefix

### 3.3 Routing layer evolution

#### Stage A: Docker Compose (1-5 shards, <50k users per shard)
- Single Caddy extracts shard prefix from room code, routes to correct backend
- Caddy handles 10k+ connections easily — it only proxies, doesn't hold room state
- Cloudflare absorbs edge load, DDoS, SSL, static assets
- **Caddy is NOT the bottleneck** — each backend handles 50k with density optimizations

#### Stage B: Kubernetes (5+ shards, 50k+ users)
- **Replace Caddy with nginx-ingress or Traefik** (horizontally scalable)
- Multiple ingress pods behind L4 load balancer
- nginx: route by URI regex extracting shard prefix
- Each ingress pod is stateless — just parses room code and forwards

#### Stage C: Massive scale (500k+ users)
- Envoy/Istio mesh or Cloudflare Spectrum for WebSocket at edge
- Geographic sharding (US-West, US-East, EU) — each region has its own shard pool

#### Routing rules (all stages)
- `/ws/{roomCode}` → extract first 2 hex chars → lookup prefix→backend mapping → route
- `/api/*?room_code={roomCode}` → same extraction
- **Room creation** (`/api/create-room`): Round-robin to any backend. Backend picks a random prefix from its assigned range.
- **Stateless endpoints** (`/api/health`, `/api/fx`, static files): Round-robin
- **Prefix→backend mapping:** Static config (Caddy map/ConfigMap). Updated when rebalancing.

### 3.4 Zero-downtime resharding

With shard index in the room code, resharding is trivial — no migration, no override table, no coordination:

#### Adding a backend
1. Start new backend + redis pair
2. Reassign some prefix ranges from existing backends to the new one (e.g., backend-0 gives up `40-55` to backend-3)
3. Update prefix→backend mapping in router config
4. **Existing rooms with prefixes `40-55`** still route correctly — mapping update is atomic
5. New rooms with those prefixes now land on backend-3
6. Existing rooms on old backend expire within 24h TTL, or can be migrated (dump/import/reconnect)

#### Removing a backend (scale down)
1. Reassign its prefix ranges to remaining backends in the router config
2. Existing rooms on the removed backend keep serving until they expire (24h)
3. Or: migrate active rooms — backend dumps state, target imports, clients reconnect
4. Once zero rooms remain, shut down

#### Rebalancing without downtime
- Prefix ranges are reassigned in the router config — atomic update
- For the brief overlap window: old backend still has the rooms, new backend doesn't yet
- Strategy: update router → new rooms go to new backend. Old rooms drain on old backend (24h max).
- For immediate migration: old backend exports room state → new backend imports → clients reconnect

#### Why this works
- Room code `0AXYZQRS` routes based on prefix `0A` → whatever backend owns `0A` in the mapping
- The mapping is a simple config (256 entries max), hot-reloadable
- Adding/removing backends just changes which backend owns which prefix ranges
- No consistent hash ring complexity — just a flat lookup table

### 3.5 High Availability — every component

| Component | Failure mode | Impact | Mitigation |
|---|---|---|---|
| **Cloudflare** | Global outage (extremely rare) | Total outage | Accept — Cloudflare has 99.99% SLA |
| **Caddy/Ingress** | Process crash | All traffic drops | **Compose:** Cloudflare LB ($5/month) health-checks 2 origins, failover. **K8s:** Ingress replicas: 2+ behind L4 LB |
| **Backend shard** | Process crash | That shard's rooms drop | Docker restarts in <5s. Clients auto-reconnect. Room state preserved in Redis. **K8s:** Pod restart + readiness probe |
| **Redis per shard** | Process crash | Room data lost = unacceptable if monetized | **Required:** Redis Sentinel (master + 1 replica per shard) with `appendfsync always`. See 3.5.1 below. |
| **Receipt parsing** | Gemini API down | Parse fails, room still works | Multi-provider fallback (Gemini → OpenAI). Already have both API keys |
| **DNS** | Cloudflare DNS outage | Unreachable | Accept — Cloudflare DNS has 100% SLA history |

### 3.5.1 Redis HA: Zero room loss guarantee

**If users are paying, losing a room is a product-killing bug.** Redis must survive any single-node failure with zero data loss.

#### Architecture per shard: Redis Sentinel (master + 1 replica + 3 sentinels)
```
backend-0 ──► redis-0-master ◄──replication──► redis-0-replica
                    ▲                              ▲
              sentinel-0  sentinel-1  sentinel-2
              (can share sentinels across all shards)
```

#### Configuration
- **`appendfsync always`** — fsync every write. Zero data loss, ~30% throughput hit (acceptable for Divvi's ops/sec)
- **Sentinel auto-failover** — if master dies, sentinel promotes replica within ~15-30 seconds
- **go-redis Sentinel support** — just change `REDIS_URL` to `redis-sentinel://sentinel-0:26379/0?master=shard-0`. go-redis handles master discovery and failover transparently.
- **Replication is async by default** — to guarantee zero loss on failover, set `min-replicas-to-write 1` and `min-replicas-max-lag 1` on master. This makes writes block until replica confirms receipt.

#### Compose setup per shard
```yaml
redis-0-master:
  image: redis:7-alpine
  command: >
    redis-server
    --appendonly yes --appendfsync always
    --min-replicas-to-write 1 --min-replicas-max-lag 1

redis-0-replica:
  image: redis:7-alpine
  command: redis-server --replicaof redis-0-master 6379 --appendonly yes --appendfsync always

sentinel-0:
  image: redis:7-alpine
  command: redis-sentinel /etc/sentinel.conf
  # sentinel.conf: sentinel monitor shard-0 redis-0-master 6379 2
```

#### K8s setup
- Redis Sentinel operator (e.g., Spotahome redis-operator) manages master/replica/sentinel lifecycle
- Or use managed Redis with replication (any provider — just need `REDIS_URL` that points to sentinel)

#### Cost impact
- 2x Redis instances per shard (master + replica) + 3 sentinel processes (shared across shards)
- Sentinels use <50MB RAM each — negligible
- Redis replica uses same RAM as master (~500MB per shard)
- **Net cost: ~$10-20/month extra per shard** (just more RAM on the VM, or a small second VM)

#### What happens during failover
1. Master dies → sentinel detects in ~5 seconds
2. Sentinel promotes replica to master (~10s)
3. go-redis detects master change → reconnects to new master (~1-2s)
4. **Total interruption: ~15-20 seconds.** During this window:
   - WebSocket connections stay alive (they're on the backend, not Redis)
   - Writes fail → backend returns errors → client retries (CRDT ops are idempotent)
   - No data lost — replica has all writes (synchronous replication)
5. After failover, everything resumes normally

#### Backend code changes
- **File:** `backend/internal/redisstore/store.go`
- Change `redis.NewClient` → `redis.NewFailoverClient` with sentinel config
- go-redis handles all failover logic internally
- Add retry logic for write failures during failover window (3 retries, 1s backoff)

**Key HA design choices:**
- No shared state between shards → no global SPOF
- Shard prefix in room code → no override table to fail
- Redis Sentinel with synchronous replication → zero data loss on any single-node failure
- Client auto-reconnect + CRDT idempotency → brief write pause during failover, no user-visible data loss
- Each shard failure only affects its rooms, not the whole system

### 3.6 Docker Compose multi-shard (dev/staging)
- **File:** `docker-compose.prod.yml`
```yaml
backend-0:
  build: ./backend
  environment:
    - SHARD_INDEX=0
    - REDIS_URL=redis://redis-0:6379/0
  ulimits:
    nofile: { soft: 200000, hard: 200000 }
redis-0:
  image: redis:7-alpine
  command: ["redis-server", "--appendonly", "yes", "--maxmemory", "1gb", "--maxmemory-policy", "allkeys-lru"]

backend-1:
  build: ./backend
  environment:
    - SHARD_INDEX=1
    - REDIS_URL=redis://redis-1:6379/0
  ulimits:
    nofile: { soft: 200000, hard: 200000 }
redis-1:
  image: redis:7-alpine
  command: ["redis-server", "--appendonly", "yes", "--maxmemory", "1gb", "--maxmemory-policy", "allkeys-lru"]

caddy:
  # parses room code prefix, routes to backend-{prefix}
```

### 3.7 Kubernetes + KEDA (production)
- **New dir:** `k8s/`
- `k8s/backend-statefulset.yaml` — StatefulSet for stable pod names (backend-0, backend-1, ...)
- `k8s/redis-statefulset.yaml` — Matching Redis StatefulSet, or sidecar Redis per backend pod
- `k8s/ingress.yaml` — nginx ingress routing by room code prefix
- `k8s/keda-scaledobject.yaml` — KEDA scales based on:
  - Active WS connections per shard (Prometheus metric from `/metrics`)
  - CPU utilization (fallback)
  - Min: 1, Max: 20 replicas
- KEDA scale-down triggers drain mode (stop new room creation on that shard, wait for 24h expiry)

### 3.8 Separate receipt worker (optional, 10k+ bills)
- Receipt parsing is CPU/memory heavy but stateless — doesn't need room affinity
- **New file:** `backend/cmd/receipt-worker/main.go`
- Backend receives upload → enqueues to a shared Redis queue (separate from shard Redis)
- Worker pool (separate Deployment) processes images + calls Gemini
- Result stored back in the room's shard Redis (worker knows shard from room code prefix)
- Backend notifies client via WS when result ready
- KEDA scales workers on queue depth

**Phase 3 capacity: ~50,000 connections per shard. 3 shards = 150k connections = 30-60k active bills. Add shards linearly.**

---

## Phase 4: Observability (ongoing, cloud-agnostic, ~$0 added)

### 4.1 Structured logging
- Replace `log.Printf` with `log/slog` (JSON output)
- Add `shard` field to all log entries for filtering
- Works with any log aggregator: Loki/Grafana, ELK, Datadog
- No vendor lock-in — just structured JSON to stdout

### 4.2 Prometheus metrics endpoint
- **File:** `backend/internal/server/server.go`
- Add `/metrics` endpoint using `prometheus/client_golang`
- Per-shard metrics: active WS connections, active rooms, ops/sec, receipt parse duration, Gemini API latency, goroutine count, memory usage
- KEDA scales on these custom metrics
- Scrape with Prometheus + Grafana dashboards

### 4.3 Health & readiness probes
- `/api/health` — liveness (already exists)
- `/api/ready` — readiness (checks own Redis connectivity + connection count below limit)
- Used by both Docker Compose health checks and Kubernetes probes

### 4.4 Edge protection
- Cloudflare WAF rules + Bot Fight Mode (free tier, provider-agnostic)
- CSRF validation on receipt parse endpoint

---

## Cost Summary

| Phase | Monthly Cost | Capacity | Effort |
|-------|-------------|----------|--------|
| Current | ~$13 | ~200 concurrent / ~40 bills | — |
| Phase 1 (harden) | ~$13 | ~500 concurrent / ~100 bills | 1-2 weeks |
| Phase 2 (static FE + VM) | ~$25-50 | ~2,000 concurrent / ~400 bills | 2-4 weeks |
| Phase 2.5 (density) | ~$50-100 | ~50,000 concurrent / ~10-20k bills | 1-2 weeks |
| Phase 3 (N shards) | ~$60-120/shard (includes Redis replica) | ~50k conn / ~10-20k bills per shard | 4-8 weeks |

## Per-Bill Cost Estimate at Scale

At 10,000 bills/month (~50k connections peak):
- Gemini API: ~$0.003/parse = $30/month
- Single VM (e2-standard-2): $50/month
- Redis (single instance): ~$0 (runs on same VM)
- **Total: ~$0.008/bill ($80/month) — single shard handles this**

At 100,000 bills/month (~500k connections peak):
- Gemini API: $300/month
- 10 shards × $50/month = $500/month
- **Total: ~$0.008/bill ($800/month)**

## Architecture Portability

| Component | Dev/Small | Production | Switch Cost |
|-----------|-----------|------------|-------------|
| Backend | Docker Compose (named services) | K8s StatefulSet + KEDA | Env vars only |
| Redis | Docker container per shard | Any managed Redis per shard | Change `REDIS_URL` |
| Frontend | Caddy file_server | Any CDN/S3 bucket | Static files |
| Routing | Caddy prefix-based LB | nginx/traefik Ingress | Config only |
| Scaling | Add compose services | KEDA autoscaler | Add manifests |
| HA (Redis) | AOF + restart | Redis Sentinel | Change `REDIS_URL` |
| HA (Router) | Cloudflare LB to 2 origins | Ingress replicas + L4 LB | K8s default |

## Why Sharding > Pub/Sub

| | Pub/Sub (rejected) | Sharded (chosen) |
|---|---|---|
| Cross-instance traffic | Every op copied to all backends | Zero |
| Shared state | Shared Redis = SPOF + bottleneck | None — fully independent |
| Scaling ceiling | Redis pub/sub throughput | None — add shards linearly |
| Backend code changes | Major (new pub/sub layer) | Minimal (prepend shard index to room code) |
| Failure blast radius | Shared Redis down = all rooms down | One shard down = only its rooms |
| Reshard complexity | Override table SPOF, migration | Zero — shard index in room code |

## Priority Order (maximum impact per effort)
1. Upload size limit + rate limiting (Phase 1.1-1.2) — **do now, prevents OOM + API cost runaway**
2. Redis maxmemory (Phase 1.4) — **prevents Redis OOM crash**
3. Client-side image resize (Phase 2.3) — **biggest single scalability win**
4. Static frontend export (Phase 2.2) — **frees 200MB RAM, removes a container**
5. WS buffer tuning + VM upgrade (Phase 2.5) — **50k connections on one box**
6. Shard-in-room-code + multi-shard compose (Phase 3.1-3.6) — **unlimited horizontal scaling, fully HA**
7. K8s StatefulSet + KEDA (Phase 3.7) — **auto-scales shards on any cloud or bare metal**

## Verification
- Phase 1: Load test with `hey` — confirm rate limiting, no OOM under 50 concurrent uploads
- Phase 2: Verify static export serves correctly via Caddy, measure memory savings
- Phase 2.5: Load test WebSocket with `websocat` or custom Go client — verify 10k+ connections on single instance
- Phase 3 (Compose): Run 2 shard pairs, create rooms on each, verify room code prefix routing, verify WS works across client reconnects
- Phase 3 (K8s): Deploy StatefulSet to minikube/kind, verify KEDA scales, verify ingress routing by room code prefix
- HA: Kill a backend container mid-session, verify client reconnects and room state preserved from Redis AOF
