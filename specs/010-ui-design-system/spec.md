# Feature Specification: Console Design System

**Feature Slug**: `010-ui-design-system`
**Status**: Verified, not released
**Mode**: Lightweight SDD, agreed with the owner

## 1. Objective & Motivation
The console reads as a generated template: gradients, glows, emoji icons, nested rounded cards, monospace uppercase labels and metrics that are hardcoded rather than measured. Operators need a dense, trustworthy tool where colour means flag state and every number is real.

## 2. User Stories
- **US1**: As an operator, I want to scan flag state quickly, so that I can act without parsing decoration.
- **US2**: As an operator, I want every number on screen to come from real data, so that I can trust it during an incident.
- **US3**: As an operator working at night, I want light and dark themes, so that the console is comfortable in either.
- **US4**: As a maintainer, I want one token set, so that styling stays consistent.

## 3. Acceptance Criteria
- [x] AC1: Colours, type sizes and radii come from one token set in `web/static/app.css` and the Tailwind config in `layout.templ`.
- [x] AC2: No gradients, backdrop blur, glow shadows, looping decorative animation, emoji icons or `rounded-2xl/3xl` in `web/views` or `web/static`.
- [x] AC3: Monospace is limited to keys, values and code.
- [x] AC4: A theme control switches light and dark, defaults to the OS preference, persists, and does not flash on load.
- [x] AC5: Overview and Analytics show no invented figures. Unavailable data is omitted or shown as an empty state.
- [x] AC6: Unused CustomCursor, ThreeCanvas, Three.js and GSAP are removed.
- [x] AC7: Overview, flags, analytics, audit, toasts and sign-in have no horizontal page overflow at 390px and 1440px.
- [x] AC8: No route or behaviour is removed. CSP nonces remain on all scripts.

## 4. Edge Cases & Constraints
- `localStorage` may be unavailable; the theme falls back to the OS preference.
- Zero flags and zero evaluations render as empty states, not zeros dressed as metrics.
- Landing page claims of certification (SOC2) are removed because nothing in the repo substantiates them.
