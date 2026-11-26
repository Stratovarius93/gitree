# gitree: live refresh and a loading spinner

## Why

gitree only reloads on start, on `r`, after its own actions, after lazygit
(`L`) and on the auto-fetch every 5 minutes. Commits, checkouts or new
branches made elsewhere (another terminal, the editor) stay invisible for up
to 5 minutes, so the user ends up quitting and reopening. The goal is the
lazygit feel: the view follows the repo on its own, and anything slow shows a
spinner.

## Goals

- Changes to HEAD, branches, tags, stashes and the index made outside gitree
  show up within ~2 seconds, without pressing `r`.
- Edited (unstaged) files show up in the WIP row within ~10 seconds.
- Coming back to the terminal window refreshes right away.
- A braille dots spinner (`⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏`) in the status bar
  while something is loading or running.

## Non-goals / anti-goals

- No new dependencies (no fsnotify).
- Polling must not reload the log when nothing changed: no flicker, no cursor
  jumps, no CPU burn on large repos (thousands of commits).
- Polling must never race gitree's own git commands (pull, push, checkout,
  fetch) or run while lazygit has the terminal.

## Approaches

1. **Cheap fingerprint every 2 s + refresh on focus (picked).** A light check
   decides whether a reload is needed. Chosen because it reacts fast enough,
   costs almost nothing when idle and needs no dependency.
2. Watching `.git` with fsnotify: near-instant, but a new dependency, noisy
   (lock and temp files, gitree's own commands) and fragile across platforms.
3. Full reload every N seconds: simplest, but reloads hundreds of commits for
   nothing and can disturb the view.

## Design

- **Refs fingerprint (every 2 s).** One cheap git call that captures HEAD
  (symbolic ref + hash), all refs with their hashes (`for-each-ref`, includes
  `refs/stash`) and the index mtime. If it differs from the last one, run
  `reloadAll()` (which already keeps the selection by hash). The fingerprint
  is also taken after every reload, so gitree's own changes do not cause a
  second reload.
- **Working tree check (every 10 s).** Reload only the WIP row (`loadWIP`)
  and only re-render if its file list changed. `git status` is the expensive
  part on big repos, hence the slower cadence.
- **Skipped** while `external` (lazygit), `acting`, `fetching` or a reload is
  in flight; the tick chain stays alive (same pattern as auto-fetch).
- **Focus.** Start the program with `tea.WithReportFocus()`; on
  `tea.FocusMsg` run the fingerprint check immediately (and the WIP check).
- **Auto-fetch** interval goes from 5 minutes to 1 minute, like lazygit.
- **Spinner.** `bubbles/spinner` with the `MiniDot` frames, Gruvbox aqua. It
  ticks only while busy (initial load, reload, fetch, pull, push, checkout,
  other actions) so an idle gitree does not redraw. It replaces the static
  `loading…` / `fetching…` text in the status bar (e.g. `fetching ⠋`).
  Panel placeholders (`loading…` in detail/preview/picker) get it too.
- Polling, like auto-fetch, is opt-in from `main` so tests never wait on
  timers; tests drive the tick messages directly.

## Left to the builder

- Exact git command for the fingerprint (one process preferred).
- How the spinner frame reaches the panel placeholders.
- Test shape (fake repo/system already exist in the ui tests).
