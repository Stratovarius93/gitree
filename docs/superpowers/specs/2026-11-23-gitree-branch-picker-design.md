# gitree: adaptive branch column and branch finder

Date: 2026-11-23 · Status: approved by the user

## Why
After trying gitree on a large real-world repository (~20k commits): branch names are cut off in their
column, and searching for `master` with `/` finds commits whose message says
"master" instead of the branch (also, `master` is not in the default view
because there is no local copy).

## 1. Adaptive Branch / Tag column
- Width = the longest rendered label (pill + icons + ↑/↓ + PR chip + " +N")
  among the loaded rows, minimum 24, maximum 45 (and never more than ~35 % of
  the total width on narrow terminals).
- The message gives up space first (it can drop to 20). The graph and the hiding
  order (Date → Author → Hash) are kept. The exact-width invariants still hold.
- It is computed when data changes (not on every cursor move).

## 2. Branch finder (`b`)
- Floating window with a text field; it filters as you type (case-insensitive
  substring) over all local branches, remote branches (without `*/HEAD`) and
  tags, via `git for-each-ref`.
- Order: local, remote, tags; alphabetical within each group. Shows
  local/cloud/tag icons and the PR chip if there is one.
- `j/k`/arrows move, `enter` jumps, `esc` closes. Title "Find branch".
- On jump: if the branch's commit is loaded, it is selected. If it is not
  loaded but the branch is reachable in the current view, pages are loaded until
  it is found (with a "searching…" indicator). If the branch does not belong to
  the current view (e.g. `origin/master` with no local copy in the default
  view), the view switches to "all branches" automatically, the status bar says
  so, and the search continues.
- `/` stays the same (searches messages, authors, visible labels, hash).
- The help (`?`) and the README list `b`.

## Approaches
The only reasonable way: a dedicated picker reusing the existing overlay
(`spliceBox`) and the existing paged loading. Rejected: a prefix in `/` (less
discoverable, mixes with message search).
