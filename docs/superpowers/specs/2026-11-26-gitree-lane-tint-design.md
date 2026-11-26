# gitree: lane tint and current-branch connector

## Why

The user wants the GitKraken look: each commit row reads as part of its
branch's lane, and the branch you are on is obvious at a glance.

## Design (approved 2026-11-26)

1. **Lane tint.** On every row, from the commit's node (`●`/`◌`, graph cell
   `2*lane`) to the end of the graph column, the background is the lane color
   mixed ~20% into the Gruvbox background (`#282828`); ~30% on the selected
   row, which keeps its full-row highlight elsewhere. The separator between
   graph and message becomes a tinted space, a `▌` in the strong lane color and
   a plain space.
2. **Current-branch connector.** On the row whose first label is the checked
   out branch (`IsHead`), the free space after the label (after the PR chip
   and `+N`) is filled with `━` in the lane color, the label→graph separator
   becomes `━━━`, and the graph cells left of the node turn spaces into `━`;
   a crossing lane's `│` becomes `┿` in that lane's color, so neither line is
   cut.
3. WIP row gets the tint from its `◌`. Hidden graph or label columns skip the
   parts that need them.

## Approaches

- Picked: per-cell rendering in `RenderRow` on top of the existing graph
  output (cut the ANSI line at the node with `ansi.Cut`). No change to the
  graph renderer.
- Rejected: teaching the lazygit-derived renderer about backgrounds (wider
  change, render cache keyed by style).

## Non-goals

- No tint in message/author/date/hash columns; no connector for non-HEAD
  branches.
