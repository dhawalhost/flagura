# Plan: Console Design System

## Approach
- Tokens: RGB triples as CSS variables in `app.css`, swapped by `html[data-theme]`. The Tailwind CDN config exposes them as `neutral`, `accent`, `ok`, `warn`, `danger`, plus `surface` and `canvas`.
- Migration: a one-off codemod rewrites palette, radius, type-size and decoration classes in `*.templ` and `app.js`. Screens that need structural change are edited by hand: shell, overview, analytics, toasts, auth, landing copy.
- Theme: inline nonced head script applies the stored or OS theme before paint; `globalApp` exposes `toggleTheme` (Alpine CSP build compatible). Charts read tokens at render.
- Metrics: overview counts are computed server-side from flags and audit entries. Analytics uses the existing stats endpoint and drops figures with no data source.

## Non-functional
- WCAG 2.1 AA text contrast (1.4.3) for token pairs; visible focus (2.4.7); `prefers-reduced-motion` honoured (2.3.3).
- Removing Three.js and GSAP lowers script payload on every page.
- No API, schema, or route changes.

## Constitution check
No new dependencies, no data model change, CSP nonce policy preserved.
