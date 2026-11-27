# gitree — CHANGES column

## Intent

Show at a glance how much each commit adds and removes, and in what
proportion, without making the graph open any slower.

## What it looks like

A new column between MESSAGE and AUTHOR:

```
 MESSAGE                    ┊ CHANGES               ┊ AUTHOR
 fix: status bar keeps…     ┊ ▬▬▬▬▬▬▬▬▬▬ +42   -18   ┊ Juan
 docs: new screenshots      ┊ ▬▬▬▬▬▬▬▬▬▬ +6    -0    ┊ Juan
```

- Bar: always 10 cells of `▬`, the same length on every row. The first
  `round(10 * added / (added + deleted))` cells are green (`colors.Green`),
  the rest red (`colors.Red`). A side with at least one line always gets at
  least one cell (so `+300 -2` still shows some red).
- Numbers: `+N` in green and `-N` in red; a side that is 0 is gray
  (`colors.Gray`). Each number is left-aligned in a fixed slot so the
  column lines up.
- A commit with no line changes (empty commit, merge, binary-only) shows a
  gray bar and gray `+0 -0`.
- While a commit's stats are loading, the bar is drawn in `colors.Separator`
  with no numbers.

Prototype: the user picked `▬` from four glyphs (`━ ▬ ─ ╸`) after seeing
them rendered in the theme colors.

## Approaches

1. Add `--shortstat` to the existing paged `git log`. One command, but git
   computes every commit's diff before the page appears, which slows
   opening in large repos.
2. **Chosen:** load stats in the background. The page renders exactly as
   today; a second command fetches stats for the page's commits and fills
   the column when it returns.
3. One `git show` per commit — hundreds of processes; rejected.

The user picked 2: gitree should keep opening instantly.

## Design

- `internal/git`: `Repo.Stats(hashes []string) (map[string]Stat, error)`,
  `Stat{Added, Deleted int}`, using one
  `git log --no-walk=unsorted --format=%x1e%H --numstat <hashes>` call and
  summing per commit (binary files, shown as `-`, count as 0). Merges get
  no diff from this command and so show `+0 -0`.
- `internal/ui`: the model keeps a `map[hash]Stat` cache. Commit stats
  never change, so the cache survives live refreshes and only hashes not
  yet in it are fetched. After every log page load (initial, append,
  refresh) a `tea.Cmd` fetches the missing hashes and returns a
  `statsLoadedMsg`; results merge into the cache and rows re-render. Errors
  are ignored (the column stays in its loading state); stats are a nice-to-
  have and must never block or break the graph.
- Layout (`table.go`): a `Changes` width and `ShowChanges` flag. Fixed width
  22 cells (10 bar + space + `+NNNN -NNNN`). Counts above 9999 are shown as
  `10k`, `125k`. CHANGES is hidden first when the terminal narrows (below
  120 cells), before DATE.
- WIP row: blank in this column.

## Calls made without the user (open to correction)

- Bar width 10, column width 22, hide threshold 120 cells, hidden before
  DATE.
- Merges and empty commits: gray bar, `+0 -0`.
- WIP row left blank.
- Large counts abbreviated with `k`.

## Non-goals

- Bar length scaled by commit size (the user wants the same length for
  all).
- Per-file stats in the table (the preview already covers files).
