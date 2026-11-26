# Status Bar and Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans.

**Goal:** Powerline status bar with icons and version; stronger selected row.
**Spec:** `docs/superpowers/specs/2026-11-26-gitree-statusbar-design.md`

## Review Focus

- Bar width exactly `m.width` for widths 1..200, with every segment present.
- Long branch names and long errors truncate without breaking the arrows.
- Message kinds map to the right icon/color.
- Selected row keeps no background holes; marker does not change row width.

### Task 1: selected row
- [ ] Tests: selected message cells bold + `#ebdbb2`; cell 0 is `▌` aqua; bg `#504945` in message/author; width unchanged.
- [ ] Implement in `RenderRow` (`colors.SelBg`, `colors.Aqua`).
- [ ] Commit `feat: stronger selected row`.

### Task 2: status bar
- [ ] Tests (`statusbar_test.go`): width for 1..200; segments' text and icons present; error → red ``; flash → green ``; warning flash (statusBusy) → yellow ``; acting `…` → spinner; version pill on the right (`WithVersion("v1.2.3")`, default `dev`); narrow width drops fetch before version.
- [ ] Implement `func (m Model) statusBar() string`, `statusKind`, `WithVersion`; View uses it outside search mode; `main.go` passes `version`.
- [ ] Commit `feat: powerline status bar with icons and version`.

### Task 3: check
- [ ] Full suite, pty screenshot, fresh review.
