# Lane Tint and HEAD Connector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Lane-tinted graph cells with a strong lane bar before the message, and a `━` connector from the current branch label to its node.

**Architecture:** All in `RenderRow` (`internal/ui/rows.go`) plus a color mixer in `internal/colors`. The graph string from the renderer is cut at the node with `ansi.Cut`; the right part gets a tint background, the left part is rewritten cell by cell for the connector.

**Tech Stack:** Go, lipgloss, charmbracelet/x/ansi.

**Spec:** `docs/superpowers/specs/2026-11-26-gitree-lane-tint-design.md`

## Global Constraints

- Tint = lane hex mixed into `#282828`: 0.20 normal, 0.30 selected.
- Row width stays exactly `w.sum()`; separators stay 3 cells.

## Review Focus

- Narrow terminals: graph column clipped (node beyond `w.Graph`), label or graph hidden.
- Selected row: no background holes (existing `TestSelectedRowBackgroundHasNoHoles`).
- HEAD label truncated with `…` or with PR chip / `+N`: connector only fills real free space.
- Detached HEAD and WIP rows.

---

### Task 1: `colors.Mix`

- [ ] Test `TestMix`: `Mix("#ffffff", "#000000", 0.5) == "#808080"` (rounded), `Mix(x, y, 0) == y`, `Mix(x, y, 1) == x`.
- [ ] Implement `func Mix(fg, bg string, a float64) string` and `const Background = "#282828"` in `internal/colors/colors.go`.
- [ ] Commit `feat: color mixing for lane tints`.

### Task 2: lane tint and bar

- [ ] Tests in `rows_test.go`: `TestRowTintFromNode` (stripped text unchanged; the cells from the node to the graph end carry the 20% tint bg, cells left of the node do not); `TestRowBarBeforeMessage` (the graph→message separator is ` ▌ ` stripped, `▌` in lane fg); `TestSelectedRowTintStronger` (30% tint); width test still passes for all totals; WIP row tinted from `◌`; node beyond the clipped graph width → no tint, separator still 3 cells.
- [ ] Implement in `RenderRow`: `func tintGraph(g string, node, width int, tint string) string`; per-row separators (graph→message built per row).
- [ ] Commit `feat: lane tint behind the graph and a lane bar before the message`.

### Task 3: HEAD connector

- [ ] Tests: `TestHeadConnector` (row with IsHead label: stripped label ends with `━` up to its width, label→graph separator `━━━`, a crossing `│` becomes `┿`, spaces before the node become `━`); `TestNoConnectorWithoutHead`; `TestConnectorWithTruncatedLabel` (no `━` when the pill fills the column).
- [ ] Implement `func connectLabel(label string, width int, hex string) string` and `func connectGraph(g string, node int, hex string) string`.
- [ ] Commit `feat: connector from the current branch to its commit`.

### Task 4: check and docs

- [ ] Full suite, build, run in pty on a repo with branches; README sentence under the intro screenshots is not needed (visual).
