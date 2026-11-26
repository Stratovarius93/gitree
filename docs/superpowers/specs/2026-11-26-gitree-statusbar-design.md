# gitree: Powerline status bar and a stronger selected row

## Why

On top of the lane tint the selected row (`#3c3836`) is hard to find, and the
status bar is plain gray text. The user wants a colored bar with icons that
match each message and the version on the right. Approved 2026-11-26 after
mockups; the user picked option A (Powerline).

## Selected row

- Background `#504945` (was `#3c3836`); the lane tint stays 30%.
- The first cell of the row becomes `▌` in aqua.
- The message is bold, in `#ebdbb2`.

## Status bar (option A, Powerline)

Left, segments joined by `` (fg = previous bg, bg = next bg), then the bar
background `#3c3836`:

1. repo: ` <name>`, aqua bg, dark bold text;
2. branch: ` <branch> ↑a ↓b` (from tracking), green bg; detached HEAD on
   orange;
3. scope: ` local` / ` all`, `#504945` bg, normal text.

Middle (bar background, colored text), joined by two spaces:

- the status message: red `` for `error:`; orange spinner while in
  progress (text ending in `…` while acting, jumps, fetch); green `` for
  confirmations (`flash`); yellow `` for warnings (other `setStatus` texts
  and the known warning flashes: busy, no lazygit, no checkout, no PR);
- search results ` 2/5`, PR note gray ``, fetch error red ``.

Right, joined by ``: last fetch ` 10:25` on `#504945`, then the version
` v0.4.0` on purple (`dev` for local builds).

When the width is short: the middle is truncated first (with `…`), then the
fetch segment, then the scope, then the version are dropped. Search mode keeps
its input line as today. `Status()` keeps returning plain text (tests).

## Approaches

Picked: render the bar in a new `statusbar.go` from the same model fields
`Status()` reads. Rejected: styling `Status()`'s joined string (no structure
to color by).
