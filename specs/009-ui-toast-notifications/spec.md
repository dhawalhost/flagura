# Feature Specification: Modern Interactive Toast Notifications System

**Feature Slug**: `009-ui-toast-notifications`  
**Status**: Completed  
**Created**: 2026-09-23  
**Owner**: Flagura Team / SDD Engine  

---

## 1. Objective & Motivation

Feature management consoles are highly interactive: operators constantly toggle flags, update rollout percentages, switch projects, copy SDK keys, trigger canaries, and approve 4-eyes change requests. Each of these actions relies on toast notifications to communicate state changes, confirmations, and errors.

Currently, Flagura's toast system in `web/views/layout.templ` and `web/static/js/app.js` is barebones:
1. **Limited Types & Aesthetics**: Toasts only support a binary state (`error` vs generic white text), lacking distinct styling for `success`, `warning`, `info`, and `copied`.
2. **Missing Interactivity**: Users cannot manually dismiss toasts with a close button (`×`), and toasts do not pause their auto-dismiss timer on mouse hover.
3. **No Rich Content**: Toasts only accept a plain string, precluding titles, multi-line descriptions, or action buttons (e.g., "Undo" flag toggle, "View in Audit").
4. **Dynamic Icon Rendering Flaws**: Dynamic Alpine `<template x-for>` loops struggle with post-DOM Lucide icon hydration unless custom SVGs are embedded cleanly.
5. **No Queue & Timer Limits**: Rapid clicks stack unbounded toasts off-screen with no visual progress countdown.

This specification defines a **modern, enterprise-grade, accessible, glassmorphic toast notification system** that elevates user feedback while preserving 100% backward compatibility with all existing `showToast(msg, type)` call sites.

---

## 2. User Stories

### US-1: Multi-Type Status Feedback
**As an** operator managing flags,  
**I want** visual feedback with distinctive color tokens, badges, and crisp icons for **Success**, **Error**, **Warning**, **Info**, and **Copied** events,  
**so that** I can instantly understand the nature and severity of system responses.

### US-2: Interactive Dismissal & Pause-on-Hover
**As an** engineer reading an error or audit notification,  
**I want to** hover over the toast to pause the auto-dismiss timer and click a close (`×`) button to dismiss it immediately,  
**so that** I have sufficient time to read long messages without them vanishing unexpectedly.

### US-3: Rich Notifications with Actions
**As a** developer toggling an environment flag,  
**I want** the toast to optionally display a title, detailed description, and an action button (such as "Undo" or "View Audit"),  
**so that** I can recover from accidental actions or navigate to relevant context with a single click.

### US-4: Visual Countdown & Queue Management
**As a** power user triggering multiple bulk actions,  
**I want** visible progress countdown bars on each toast and a maximum stack limit (e.g., 5 toasts),  
**so that** toasts do not flood the viewport or obstruct critical dashboard controls.

### US-5: Backward Compatibility & CSP Compliance
**As a** maintainer of Flagura's strict Content Security Policy (zero `unsafe-eval`),  
**I want** all existing calls like `showToast("Flag updated", "success")` and `showToast("Toggle failed", "error")` to seamlessly work with the new system,  
**so that** no existing features or handlers break.

---

## 3. Acceptance Criteria

### A. Toast Types & Visual Palette
- **AC-1.1**: The toast system supports 5 primary visual types:
  - **`success`**: Emerald theme with check-circle icon (`bg-emerald-50/95 border-emerald-200 text-emerald-950`).
  - **`error`**: Rose/Red theme with alert-circle icon (`bg-rose-50/95 border-rose-200 text-rose-950`).
  - **`warning`**: Amber theme with alert-triangle icon (`bg-amber-50/95 border-amber-200 text-amber-950`).
  - **`info`**: Indigo/Sky theme with info icon (`bg-indigo-50/95 border-indigo-200 text-indigo-950`).
  - **`copied`**: Minimal dark slate pill with clipboard-check icon (`bg-slate-900/95 border-slate-700 text-slate-100`).
- **AC-1.2**: All icons must use crisp inline SVG to avoid CSP issues or post-render Lucide initialization lag.
- **AC-1.3**: Styling uses sleek glassmorphism: `backdrop-blur-md shadow-xl rounded-2xl border` with smooth entering slide-in (`translate-y-2 opacity-0` $\rightarrow$ `translate-y-0 opacity-100`).

### B. Interactive Controls & Timing
- **AC-2.1**: Each toast provides a subtle `×` close button that dismisses the toast immediately on click.
- **AC-2.2**: Hovering the pointer over a toast (`mouseenter`) pauses the auto-dismiss timer; moving the mouse away (`mouseleave`) resumes the countdown.
- **AC-2.3**: Each toast features a subtle animated progress bar along the bottom indicating remaining display time.
- **AC-2.4**: Default auto-dismiss duration is 4,000ms for standard messages, 6,000ms for errors/warnings, and 2,000ms for simple copied badges.

### C. Rich Content & Action Buttons
- **AC-3.1**: The dispatcher supports both legacy format `showToast(msg, type)` and rich object format:
  ```javascript
  showToast({
    title: 'Flag Disabled (Kill-Switch)',
    message: 'Feature checkout-v2 was disabled in production.',
    type: 'warning',
    duration: 5000,
    action: {
      label: 'Undo',
      onClick: () => undoAction()
    }
  });
  ```
- **AC-3.2**: When an `action` is present, clicking the action button executes the callback and automatically dismisses the toast.

### D. Queue Management & Accessibility
- **AC-4.1**: Maximum visible toast limit is capped at 5; new toasts beyond the limit gracefully evict the oldest toast.
- **AC-4.2**: The toast container is wrapped in an accessible container with `aria-live="polite"` and `role="status"` (`aria-live="assertive"` for errors).
- **AC-4.3**: Responsive design ensures toasts appear bottom-right on desktop and bottom-center on mobile with `max-w-[calc(100vw-2rem)]`.

---

## 4. Edge Cases & Constraints

1. **Zero Unsafe-Eval CSP Invariant**: Alpine.js runs under Flagura's strict CSP build. All handlers, callbacks, and loops must use pre-compiled Alpine methods or direct component methods—no dynamic string evaluations (`eval` or `new Function`).
2. **Duplicate Toast Suppression**: If the exact same message and type are triggered within 1,000ms, the system resets the existing toast's timer rather than spawning duplicate stacked cards.
3. **Z-Index Hierarchy**: The toast container must sit at `z-50` or higher to ensure it floats above modals, dropdowns, and sticky headers.
