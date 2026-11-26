# Live Refresh and Spinner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** gitree follows the repo on its own (refs every 2 s, WIP every 10 s, on terminal focus, fetch every 1 min) and shows a braille spinner while busy.

**Architecture:** A cheap `Repo.Fingerprint()` is polled by a tick chain in the ui model; a change triggers the existing `reloadAll()`. A separate slower tick reloads the WIP row and only rebuilds when the file list changed. A `bubbles/spinner` (MiniDot) ticks only while the model is busy and its frame replaces the trailing `…` of in-progress texts.

**Tech Stack:** Go, bubbletea v1.3.10, bubbles v0.21.1 (spinner), testify.

**Spec:** `docs/superpowers/specs/2026-11-26-gitree-live-refresh-design.md`

## Global Constraints

- No new dependencies (bubbles is already required).
- Refs poll every 2 s, WIP poll every 10 s, auto-fetch every 1 min.
- Spinner frames: `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏` (spinner.MiniDot), Gruvbox aqua (`colors` aqua constant).
- Timers (polling, spinner animation) are opt-in from `main`, so tests never wait on them; tests send tick messages directly.
- English UI text and comments; no employer names in tests or docs.

## Review Focus

- Polls must be skipped while `external`, `acting`, `fetching` or `loading`, and the tick chain must survive the skip.
- gitree's own actions must not cause a second reload: the fingerprint is re-taken after every reload.
- `git status` refreshing the index must not look like a change (fingerprint excludes the index; git runs with `GIT_OPTIONAL_LOCKS=0`).
- An unchanged WIP poll must not rebuild or touch open panels (no flicker, cursor kept).
- An unborn repo (no commits) must not error in Fingerprint.

---

### Task 1: Repo fingerprint and optional locks

**Files:**
- Create: `internal/git/fingerprint.go`, `internal/git/fingerprint_test.go`
- Modify: `internal/git/repo.go` (runCtx env)

**Interfaces:**
- Produces: `func (r *Repo) Fingerprint() (string, error)`

- [ ] **Step 1: Tests** `TestFingerprintStable` (two calls equal), `TestFingerprintChangesOnCommit`, `TestFingerprintChangesOnCheckoutSameCommit` (new branch at HEAD, then `git switch` to it: differs), `TestFingerprintChangesOnNewTagAndStash`, `TestFingerprintUnchangedByStatus` (run `WorkingFiles()` after touching a file: equal), `TestFingerprintUnbornRepo` (no error).
- [ ] **Step 2: Run** `go test ./internal/git -run Fingerprint` → FAIL (undefined).
- [ ] **Step 3: Implement.** Output of `for-each-ref --format=%(HEAD)%(refname) %(objectname)` plus `rev-parse -q --verify HEAD` (exit 1 allowed via `runAllow`). Add `GIT_OPTIONAL_LOCKS=0` to the env in `runCtx`.
- [ ] **Step 4: Run** `go test ./internal/git` → PASS.
- [ ] **Step 5: Commit** `feat: repo fingerprint for change polling`.

### Task 2: Refs and WIP polling, focus, 1-minute fetch

**Files:**
- Create: `internal/ui/poll.go`, `internal/ui/poll_test.go`
- Modify: `internal/ui/model.go` (fields, Init, Update cases, `wipLoadedMsg` handling), `internal/ui/refresh.go` (`AutoFetchInterval = time.Minute`), `main.go`

**Interfaces:**
- Consumes: `Repo.Fingerprint()`
- Produces: `RefsPollInterval = 2*time.Second`, `WIPPollInterval = 10*time.Second`, `func (m Model) EnablePolling() Model`, msgs `refsPollTickMsg{}`, `wipPollTickMsg{}`, `fingerprintMsg{fp string; err error}`; model fields `polling bool`, `fingerprint string`.

- [ ] **Step 1: Tests**
  - `TestRefsPollReloadsOnExternalCommit`: loaded model, commit outside, send `refsPollTickMsg{}`, settle → new commit is row 0's message, `m.gen` increased.
  - `TestRefsPollNoChangeNoReload`: tick with nothing changed → `m.gen` unchanged.
  - `TestRefsPollAfterOwnReloadNoSecondReload`: press `r`, settle, tick → `m.gen` unchanged after the tick.
  - `TestRefsPollSkippedWhileBusy`: with `acting` / `external` / `fetching` true, tick returns only the next tick (no fingerprint run, `gen` unchanged).
  - `TestWIPPollRebuildsOnlyOnChange`: tick with no change keeps `m.rows` identical and no `syncPanels`; after writing a file, tick + settle shows WIP row.
  - `TestFocusChecksRefs`: `tea.FocusMsg{}` after an external commit reloads.
- [ ] **Step 2: Run** `go test ./internal/ui -run 'Poll|Focus'` → FAIL.
- [ ] **Step 3: Implement.**
  - `fingerprint` is captured after every completed full reload (first page of a non-append log load) via a `loadFingerprint()` cmd, so own changes are absorbed.
  - Tick handler: if `external || acting || fetching || loading` → only reschedule. Otherwise batch `checkRefs()` and reschedule. `fingerprintMsg` with a different non-empty previous fp → `reloadAll()`; always store fp.
  - WIP: `wipLoadedMsg` skips `rebuild`/`syncPanels` when `slices.Equal(msg.files, m.wip)`.
  - `tea.FocusMsg` → `checkRefs()` + `loadWIP()` (no reschedule).
  - Init schedules both ticks when `polling`.
  - `main.go`: `.EnableAutoFetch().EnablePolling()` and `tea.WithReportFocus()`.
- [ ] **Step 4: Run** `go test ./...` → PASS.
- [ ] **Step 5: Commit** `feat: follow repo changes live (refs poll, WIP poll, focus)`.

### Task 3: Spinner

**Files:**
- Create: `internal/ui/spin.go`, `internal/ui/spin_test.go`
- Modify: `internal/ui/model.go` (Update wrapper, Status, View placeholder), `internal/ui/detail.go`, `internal/ui/preview.go`, `internal/ui/picker.go`, existing tests asserting `fetching…` / `loading…`, `main.go`

**Interfaces:**
- Produces: `func (m Model) EnableSpinner() Model`, `func (m Model) busy() bool`, `func (m Model) spinning(text string) string` (turns `"fetching…"` into `"fetching ⠋"` with the current frame, styled aqua).

- [ ] **Step 1: Tests**
  - `TestStatusShowsSpinnerWhileFetching`: after `f`, `Status()` contains `fetching ⠋`, not `fetching…`.
  - `TestStatusShowsSpinnerWhilePulling`: after a pull starts, status contains `pulling ⠋`.
  - `TestSpinnerTicksOnlyWhileBusy`: with spinner enabled, an Update that makes the model busy returns a cmd yielding `spinner.TickMsg`; a `spinner.TickMsg` when not busy returns nil and advances nothing.
  - `TestSpinnerFrameAdvances`: busy model, send the spinner's TickMsg → frame changes from `⠋` to `⠙`.
- [ ] **Step 2: Run** `go test ./internal/ui -run Spinner` → FAIL.
- [ ] **Step 3: Implement.**
  - `busy()` = `loading || fetching || acting || detail/preview/picker loading`.
  - Rename `Update` body to `update`; new `Update` calls it then, if spinner enabled, `busy()` and not already animating, sets `animating=true` and batches `spin.Tick`. `spinner.TickMsg`: if busy, `spin.Update` and return its cmd; else `animating=false`, nil.
  - Status bar: `loading`/`fetching` parts and in-progress action statuses ending in `…` while `acting` go through `spinning`. Show `loading ⠋` for any in-flight load (not only the first).
  - Panel placeholders render `spinning("loading…")` at View time instead of static viewport content.
  - Update existing tests that look for `fetching…` / `loading…`.
  - `main.go`: `.EnableSpinner()`.
- [ ] **Step 4: Run** `go test ./... && go vet ./...` → PASS.
- [ ] **Step 5: Commit** `feat: braille spinner while gitree is busy`.

### Task 4: Docs and manual check

- [ ] README: a short "Live updates" section (refs 2 s, working tree 10 s, on focus, fetch every minute; `r` still forces a reload).
- [ ] Build (`go build -o gitree .`) and run in a scratch repo: commit/checkout from another terminal shows up within ~2 s; spinner visible on `f`.
- [ ] Commit `docs: live updates`.
