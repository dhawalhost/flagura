# Verification: Console Design System

Checked against a local in-memory instance on 2026-11-09.

| Check | Result |
| :--- | :--- |
| `templ generate` (v0.3.1020) | Pass |
| `go vet ./...` | Pass |
| `go test ./...` | Pass, including `design_system_test.go` |
| `gosec -exclude-dir=web/views ./...` | Pass, no findings |
| Page errors and console errors across all views | None |
| Horizontal overflow at 1440px and 390px, light and dark | None on console, sign-in; none on landing at 390px light |
| Production flag toggle from overview | Works, toast shown |
| Theme toggle | Follows OS by default, switches, persists across reload |

## Not verified
- Formal contrast measurement of every token pair; pairs were reviewed visually.
- Analytics chart with live telemetry; only the empty state was exercised.
- Landing page in dark theme after the final pass.

## Second pass (2026-11-09)
| Check | Result |
| :--- | :--- |
| `templ generate`, `go vet ./...`, `go test ./...`, `gosec` | Pass, no gosec findings |
| Page and console errors, all views, both themes, 1440px and 390px | None |
| Horizontal page overflow on the same matrix | None |
| Flag matrix key column while scrolled on phone | Stays pinned |
| Icons inside Alpine templates | All hydrated |
| Editor create with staging rule, via UI | Saved with strategy and rule |
| Benchmark run and network probe | Work, measured values only |
| Evaluator hash position | Uses server bucket and threshold |

Not verified: measured contrast for the new accent; landing page at phone width after this pass.

## Landing page (2026-11-09)
| Check | Result |
| :--- | :--- |
| Rollout field against bucket data at 25% and 50% | 0 mismatched leaves; every leaf lit at 25% stays lit at 50% |
| Kill switch and restore | 0 lit, then original set returns |
| Keyboard control of rollout slider | Works |
| Reduced motion | Skips the sweep, shows 25% |
| Page errors, phone overflow at 390px | None |
| Text contrast on measured token pairs | At least 4.5:1 after darkening the muted neutral |

Benchmark figures come from `go test ./pkg/engine -run xxx -bench . -benchmem` on Apple silicon, 200,000 iterations.

## Console rebuild (2026-11-09)
| Check | Result |
| :--- | :--- |
| Typing `/` in fields; `/` focuses search elsewhere | Pass |
| Header search filters flags; empty filter message | Pass |
| Enterprise SSO tab | Opens, no console error |
| Modal backdrop top offset | 0 |
| Production toggle by keyboard: confirm, Escape, focus return | Pass |
| Turn off and Undo from toast | Pass |
| Staging toggle skips confirmation | Pass |
| Overview metrics follow environment | Pass |
| Measured contrast, unlabeled fields, unnamed buttons, `h1` on console views | Clean, except a decorative dot separator |

Not verified: real screen readers; a pending change request rendered in the running app (covered by a render test).

## API keys page (2026-11-09)
Create from SDK docs link, Escape, one-time reveal, copy, confirm and revoke, revoked state, `/dashboard/apikeys` and `/apikeys` routes, and 390px overflow all checked in the running app.

## Native controls (2026-11-09)
Custom select checked with mouse, keyboard, type-ahead, Escape, outside click, inside modals, x-model updates, hidden options and at 390px. Checkbox, slider fill, confirmation dialog, inline validation and tooltip checked in the running app. No browser popup fired during the run.

## Component consolidation (2026-11-09)
Switch checked for keyboard, confirmation, undo, double click, partial fill and editor use. Shared buttons, inputs, tabs and tables checked on every console view, in light and dark, and at 390px. Internal documentation links validated by script.

## Polish pass with unslop and frontend-design (2026-11-09)
Audit classed the console as light slop, so polish mode applied. Spacing, sentence case, copy, motion and navigation sizes checked; every console view rendered without errors or page overflow at 1440px, and key views at 390px.

## Premium library (2026-11-09)
Every console view measured in light and dark at 1440px: no contrast failures, no page errors, no overflow. Key views checked at 390px. Form controls measure 36px tall.
