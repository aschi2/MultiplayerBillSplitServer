# Payments + Sharing Memory

Load this file only for Venmo/payment-link/QR/share UX tasks.

## Venmo Deep Links

- `target="_blank"` Venmo web links can leave a white intermediary tab/page after returning from the Venmo app.
- Prefer button-driven open flow:
  - try `venmo://paycharge?...` first
  - avoid automatic mobile fallback to web URL because it can create duplicate charge pages
- For bulk charging, use a queue that launches the next unsent request when the page becomes visible again after returning from Venmo.
- Venmo requests should be built in USD even if bill/summary currency differs.

## Venmo Note Formatting

- Keep human-readable spaces in notes (no `+` artifacts).
- Include bill name and date in `MM/DD/YYYY` format.

## Share + QR

- For bill sharing controls, try OS share sheet first, then clipboard fallback.
- External QR APIs can intermittently fail; prefer local QR generation with fallback provider only if needed.
