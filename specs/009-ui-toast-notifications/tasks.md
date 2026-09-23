# Tasks: Modern Interactive Toast Notifications System (Spec 009)

**Status**: Specified  
**Spec**: `specs/009-ui-toast-notifications/spec.md`  
**Plan**: `specs/009-ui-toast-notifications/plan.md`  
**Created**: 2026-09-23  

---

## Phase 1: Toast Engine Implementation (`web/static/js/app.js`)
- [x] T1: Enhance global `showToast` and `globalApp.showToast` to support both string signature `showToast(msg, type)` and rich object signature `showToast({ title, message, type, duration, action })`.
- [x] T2: Implement duplicate toast suppression within 1,000ms window (resetting existing toast timer instead of duplicating).
- [x] T3: Implement queue management with max 5 concurrent toasts and auto-eviction of oldest toasts.
- [x] T4: Implement pause-on-hover (`pauseToast(id)`) and resume-on-leave (`resumeToast(id)`).
- [x] T5: Add progress countdown calculation for live progress bar animation.

## Phase 2: Toast Container UI & Template (`web/views/layout.templ`)
- [x] T6: Redesign the Toast notification container in `web/views/layout.templ` with glassmorphism, responsive positioning, and `aria-live` accessibility attributes.
- [x] T7: Embed inline SVG icons for all 5 toast types (`success`, `error`, `warning`, `info`, `copied`) ensuring 0 external icon hydration lag.
- [x] T8: Add dismissal close button (`×`) and interactive action button support.
- [x] T9: Add animated progress bar line along the bottom of each toast.

## Phase 3: Templ Compilation & Backward Compatibility
- [x] T10: Compile Templ templates via `templ generate`.
- [x] T11: Verify backwards compatibility across all existing `showToast` calls in the application.

## Phase 4: Verification & Quality Gates
- [x] T12: Run UI component tests `go test -v ./pkg/api -run "TestTemplComponents"`.
- [x] T13: Run race detector `go test -race ./...`.
- [x] T14: Run security gate `gosec -exclude-dir=web/views ./...` (0 issues).
