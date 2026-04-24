# Receipt Parsing Memory

Load this file only for receipt-import/parser/model tasks.

## Extraction Behavior

- Rotated/photo-of-screen/journal-style receipts can under-extract (first-item-only). Keep explicit orientation guidance (0/90/180/270).
- Require non-empty `raw_text` per parsed item when possible.
- Prefer one-shot prompt + post-parse normalization. Retry/fallback on hard call failure, not just low-quality output.
- After parse normalization, run second-pass Gemini row tagging (`base` vs `modifier`) on suspicious dense outputs (or high-accuracy retries), then consolidate tagged modifier rows into parent item `addons`.
- Modifier-tagging prompt rule: `2x/x2` markers alone are not enough to classify a row as a modifier.
- If Gemini returns one sparse/composite item, run guarded line-level recovery from dense `raw_text`/`unparsed_lines`:
  - extract rightmost money token per purchasable line
  - strip seat/guest prefixes
  - skip subtotal/tax/tip/payment rows
- On photographed/folded simple receipts, Gemini can emit the correct item `raw_text` but attach the wrong structured `line_price_cents`. A subtotal-guided repair that only trusts single-money `raw_text` when it improves subtotal reconciliation fixed this without hurting the hard benchmark.
- Normalize parsed/fallback item and add-on labels by removing dangling trailing currency symbols (for example `Item $` -> `Item`) before showing/importing review rows.

## Model Notes

- Active backend defaults:
  - primary: `gemini-2.5-flash-lite`
  - fallback: `gemini-2.5-flash-lite` (same model; no `2.5-flash` parser usage)
- Receipt import "Try Again" / high-accuracy mode (`parse_mode=accurate`/`retry`/`high`) uses:
  - primary: `gemini-3.1-pro-preview`
  - fallback: `gemini-2.5-flash-lite`
  - decode temperature: `0.0`
- Standard parse uses:
  - standard parse temperature: `0.0`
- Keep the flow single-parse-per-attempt for cost control; avoid quality-based multi-call stabilization loops unless explicitly requested.
- UI should surface retry outcome so users can tell whether "Try Again" changed parsed lines or returned the same result.
- On dense club/journal screenshots, flash-lite can recover more lines but may include noisier structure; 2.5-flash is often cleaner but can under/over-group lines.
- Do not assume newer/more expensive model tiers OCR this receipt format better.
- Prefer sending a single image when possible; use prompt/parsing improvements before adding multi-image transformations.
- Standard parse output cap is `max_output_tokens=6800`, targeting ~25% usage (75% headroom) on the hard benchmark receipt while avoiding oversized token ceilings.
- Retry/high-accuracy parse output cap is `max_output_tokens=12000`. Retry uses `thinkingLevel=LOW` which consumes part of the output budget; a tighter cap (previously 4000) truncated JSON mid-output on receipts with many items, especially when fallback to flash-lite kicked in.
- Keep decode temperature low (`0.0`). Recent regressions on photographed long receipts were not fixed by temperature changes; prompt/flow quality mattered more.
- Avoid overloading the parse prompt with too many modifier-heavy edge-case rules. A shorter prompt that defaults to "each priced pre-total row is a standalone item unless clearly subordinate" performed better on simple receipts without hurting the hard benchmark.
- Tip parsing now relies on prompt guardrails only; do not apply deterministic post-parse clearing of `tip_cents`/`total_cents` based on suggested-tip percentages.

## Tip Parsing Rule

- Parse `tip_cents` only from already-applied gratuity/tip charges.
- Ignore suggested/recommended tip options unless the receipt indicates a tip was actually applied.
