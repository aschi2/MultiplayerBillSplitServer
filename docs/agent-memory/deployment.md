# Deployment Memory

Load this file only for deploy/production/VM/container tasks.

## Baseline

- Use `docs/deploy.md` for the standard deployment workflow.

## Gotchas

- If `docker-compose ... --build` shows `COPY frontend ./` as cached unexpectedly, recent frontend edits may not have reached the VM. Re-sync first, then rebuild.
- For paths with brackets (for example `src/routes/room/[roomCode]/+page.svelte`), plain `scp` can misplace files. Prefer tar-over-SSH so paths are preserved.
- Local `docker compose` success is not enough. Verify a changed live endpoint or bundle marker after deploy.
- Recursive `gcloud compute scp` can silently include `frontend/node_modules` and stall. Prefer tar-over-SSH with explicit excludes (`frontend/node_modules`, `.svelte-kit`, local artifacts).
- Tar-over-SSH sync must also exclude `.env`. Overwriting VM prod `.env` with local/dev values can break Cloudflare origin TLS and cause `525` even when containers are healthy.
- Canonical prod stack path is `~/app` (compose project `app_*`). Deploying to a different path can spin up a second compose project and conflict on ports 80/443.
- `scp write ... Failure` during deploy can mean the VM disk is full. Reclaim space (for example `docker image prune -af`) and retry.
- For Caddy health in this stack, `https://localhost/...` probes can be flaky in-container. Healthcheck against `http://localhost:2019/config/` has been reliable.
- A compose run can leave newer dangling frontend layers while `app_frontend:latest` still points to an older image. Always verify:
  - `docker image ls app_frontend --no-trunc` (image id changed as expected)
  - live HTML bundle hash changed (`start.<hash>.js`)
  If not, run `docker-compose -f docker-compose.prod.yml build frontend` then `docker-compose -f docker-compose.prod.yml up -d frontend`.
- If SSH streaming output is unreliable, run remote build commands with output redirected to a log file and inspect `BUILD_EXIT` plus log tail.
