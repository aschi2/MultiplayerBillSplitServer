# Agent Memory Index

This file is intentionally short so it can stay in default context without bloat.

## Default Loading Rule

- Always load `AGENTS.md`.
- Do not auto-load all memory docs.
- Load memory docs only when the task clearly touches that topic.

## On-Demand Memory Docs

- `docs/agent-memory/deployment.md`
  - Load for deploys, VM ops, docker/caddy health, and sync issues.
- `docs/agent-memory/receipt-parsing.md`
  - Load for receipt import/parser/model/prompt behavior.
- `docs/agent-memory/frontend-ui.md`
  - Load for Svelte/mobile/modals/forms/timer UX consistency issues.
- `docs/agent-memory/payments-sharing.md`
  - Load for Venmo deep links, QR/share, and payment-note formatting.

## Maintenance Rules

1. If something should survive compaction, add it to the narrowest file under `docs/agent-memory/`.
2. Keep entries concise, specific, and deduplicated.
3. Update this index only when adding/removing a memory doc or a truly always-on rule.
4. Avoid putting detail here unless it is needed for most tasks.
