# Frontend UI Memory

Load this file only for Svelte/mobile/modal/form UX work.

## State + Reactivity

- `bind:value` on `input type="number"` may provide numbers. Normalizers must accept `string | number | null | undefined` safely.
- For countdown text, pass live clock state into formatting helpers (for example `format(entry, nowMs)`) so Svelte rerenders in real time.
- Room codes should be canonicalized to uppercase across route param, join payload, and WS room id.

## Mobile + Modal Interaction

- Avoid global `touchend` handlers with `preventDefault()` for double-tap suppression; they can swallow valid taps.
- Transform-animated fixed/bottom-sheet modals can cause touch target offset after keyboard/form interaction. Prefer non-transform entry animation.
- In modal scrims, avoid hover/active border overrides that cause touch-scroll border flicker.
- Continuously animated primary tap targets (for example breathing QR buttons) can hurt click/tap stability. Animate nearby accents instead.

## UX Consistency Rules

- For modal/form polish, update shared components (`ItemPricingEditor`) and shared utility classes (`modal-*`, `ui-*`) before page-specific tweaks.
- If `0` is a valid value, dirty checks should treat blank numeric inputs as intentional pending clears.
- Normalize blank numeric inputs to currency zero (`0.00`) on blur so fields do not stay visually empty.
- Join-cancel from deep-link flows should use explicit `goto('/')` rather than `history.back()`.
- In receipt review, keep `Manage` add-ons and `Attach as add-on` panels inline under the selected item row (not detached at the bottom of the sheet) so touch context is clear on mobile.
- Repeated identical items now render as grouped cards by default. Keep assignment inside the servings sheet, not on the grouped card, and keep grouped-card bulk actions scoped to that sheet (`One each`, `Everyone`, `Clear`).
- Item ordering is persisted with `sort_order`. New frontend item flows must preserve it on edit and append new items/servings at deterministic increasing values so snapshots do not scramble the visible order.

## Landing TTL

- Poll jitter can make countdowns appear to reset. Clamp small upward TTL jumps to keep countdown monotonic.
