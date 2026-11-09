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

## 5. Second Pass Acceptance Criteria
- [x] AC9: Evaluator, benchmark, editor, SDK and profile have no nested cards, duplicate page titles or repeated primary actions.
- [x] AC10: The flat ink-and-petrol redraw of the Flagura mark replaces the raster logo; primary actions use ink, and the accent is reserved for selection, links and focus.
- [x] AC11: The flag matrix keeps the key column visible while scrolling on phones, shows health only when it needs action, and reveals row actions on hover at desktop widths.
- [x] AC12: Icons render inside dynamically created content, native selects share one style, and keyboard focus is visible on toggles.
- [x] AC13: Benchmark and evaluator show measured values only.

## 6. Landing Page Acceptance Criteria
- [x] AC14: The hero is a live rollout field that uses the same sticky bucketing as the product and honours reduced motion.
- [x] AC15: Every number and capability on the landing page is backed by repository code or a benchmark, with the method stated.
- [x] AC16: Existing landing functions remain: SDK samples, browser benchmark, CLI install tabs, console and docs links.

## 7. Console Acceptance Criteria
- [x] AC17: Typing `/` works in every input; the header search filters the flag list; every profile tab opens; modal backdrops cover the viewport.
- [x] AC18: Fields have programmatic labels, modals take and return focus, Escape closes every modal, and the console has one `h1` per page.
- [x] AC19: Overview leads with work needing attention, and its counts follow the selected environment.
- [x] AC20: Each flag shows its effective rollout graphically.
- [x] AC21: Production toggles ask for confirmation and offer undo.

## 8. API Keys Acceptance Criteria
- [x] AC22: API keys have their own console page at `/dashboard/apikeys` with list, create, one-time reveal and revoke.
- [x] AC23: The SDKs page is documentation only and links to creating a key.

## 9. Native Controls Acceptance Criteria
- [x] AC24: Dropdowns open a Flagura-styled listbox with mouse and keyboard support, and keep working inside modals and with `x-model`.
- [x] AC25: Checkboxes and sliders use theme tokens.
- [x] AC26: Confirmations, alerts, field validation and tooltips use Flagura UI instead of browser popups.

## 10. Component Consolidation Acceptance Criteria
- [x] AC27: One switch component shows on, off, partial rollout, pressed, busy and focus states.
- [x] AC28: Buttons, inputs, tabs and table headers come from shared styles.
- [x] AC29: Overview shows the environment's flags in the logo's leaf form; navigation is grouped by task.
- [x] AC30: Unused client code and promotional comments are removed.

## 11. Polish Pass Acceptance Criteria
- [x] AC31: Spacing sits on the 4px grid except small control padding; labels are sentence case; short strings have no trailing period or exclamation mark.
- [x] AC32: One easing curve; no overshoot; no animation of width; no entrance on every view change.

## 12. Premium Library Acceptance Criteria
- [x] AC33: The console sits on a layered shell, with content on a raised sheet beside a recessed navigation rail.
- [x] AC34: Panels, controls and popovers follow one radius and elevation scale; controls share fixed heights.
- [x] AC35: Status labels use one shared badge component.
