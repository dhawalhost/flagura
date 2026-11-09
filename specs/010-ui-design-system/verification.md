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
