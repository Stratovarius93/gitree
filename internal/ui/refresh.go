package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

// AutoFetchInterval is how often the repo is fetched in the background.
const AutoFetchInterval = 5 * time.Minute

// fetchTimeout bounds one `git fetch` so a dead remote cannot hang the app.
const fetchTimeout = 60 * time.Second

type (
	// fetchDoneMsg is the result of a git fetch that finished at `at`.
	fetchDoneMsg struct {
		err error
		at  time.Time
	}
	// prsLoadedMsg is the result of loading pull requests through gh.
	prsLoadedMsg struct {
		prs map[string]github.PR
		err error
	}
	// autoFetchTickMsg fires every AutoFetchInterval once auto-fetch is enabled.
	autoFetchTickMsg struct{}
	// statusExpireMsg clears a transient status if it is still the one shown.
	statusExpireMsg struct{ seq int }
)

// EnableAutoFetch makes the model fetch every AutoFetchInterval. It is opt-in
// (main enables it) so tests that settle Init never wait on a timer.
func (m Model) EnableAutoFetch() Model {
	m.autoFetch = true
	return m
}

func (m Model) autoFetchTick() tea.Cmd {
	return tea.Tick(AutoFetchInterval, func(time.Time) tea.Msg { return autoFetchTickMsg{} })
}

// setStatus shows text in the status bar, remembering which source produced
// it so only that source's next success clears it (src "" is never cleared).
func (m *Model) setStatus(src, text string) {
	m.status, m.statusSrc = oneLine(text), src
	m.statusSeq++
}

// flash shows a transient status (a confirmation or a warning) that clears
// itself after statusTTL unless another message replaced it.
func (m *Model) flash(src, text string) tea.Cmd {
	m.setStatus(src, text)
	return after(statusTTL, statusExpireMsg{seq: m.statusSeq})
}

// maxStatusLen caps one status message; the bar truncates to the width anyway.
const maxStatusLen = 200

// oneLine makes s safe for the single-line status bar: whitespace runs
// (including newlines) become one space, control characters and escape
// sequences are dropped, and the length is capped.
func oneLine(s string) string {
	s = git.SanitizeLine(strings.Join(strings.Fields(s), " "))
	if r := []rune(s); len(r) > maxStatusLen {
		s = string(r[:maxStatusLen-1]) + "…"
	}
	return s
}

// clearStatus removes the status if src produced it.
func (m *Model) clearStatus(src string) {
	if src != "" && m.statusSrc == src {
		m.status, m.statusSrc = "", ""
	}
}

// loadPRs loads pull requests in the background; it never blocks the log.
func (m Model) loadPRs() tea.Cmd {
	fetch, root := m.fetchPRs, m.repo.Root()
	if fetch == nil {
		return nil
	}
	return func() tea.Msg {
		prs, err := fetch(root)
		return prsLoadedMsg{prs: prs, err: err}
	}
}

func (m Model) onPRs(msg prsLoadedMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err == nil:
		m.prs = msg.prs
		m.prNote = ""
		m.clearStatus("prs")
		m.rebuild()
		return m, m.syncPanels()
	case errors.Is(msg.err, github.ErrNotGitHub):
		m.prNote = ""
		m.clearStatus("prs")
	case errors.Is(msg.err, github.ErrGhMissing):
		m.prNote = "gh not installed: no PRs"
	case errors.Is(msg.err, github.ErrNoAccess):
		m.prNote = "no access to PRs: check gh auth status"
	default:
		m.setStatus("prs", "error: "+msg.err.Error())
	}
	return m, nil
}

// startFetch runs `git fetch` in the background unless one is already running.
func (m *Model) startFetch() tea.Cmd {
	if m.fetching {
		return nil
	}
	m.fetching, m.fetchErr = true, ""
	repo := m.repo
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		err := repo.Fetch(ctx)
		return fetchDoneMsg{err: err, at: time.Now()}
	}
}

func (m Model) onFetchDone(msg fetchDoneMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	if msg.err != nil {
		m.fetchErr = oneLine("error: fetch: " + msg.err.Error())
		return m, nil
	}
	m.lastFetch, m.fetchErr = msg.at, ""
	return m, tea.Batch(m.reloadAll(), m.loadPRs())
}

// onAutoFetchTick fetches and schedules the next tick. While lazygit has the
// terminal, or an action (pull, push, checkout, …) is running, the fetch is
// skipped (it could race those git commands), but the tick chain stays alive.
func (m Model) onAutoFetchTick() (tea.Model, tea.Cmd) {
	if m.external || m.acting {
		return m, m.autoFetchTick()
	}
	return m, tea.Batch(m.startFetch(), m.autoFetchTick())
}

// reloadAll refreshes log, working tree, tracking and branch. It reloads as
// many commits as are loaded now, so the selected commit stays present and the
// cursor does not jump; rebuild then keeps the selection by hash.
func (m *Model) reloadAll() tea.Cmd {
	m.gen++
	m.loading, m.more = true, false
	if m.loaded {
		// A reload must not re-pin the cursor to the top (new remote commits
		// would steal the selection): keep the selected commit by hash.
		m.userMoved = true
	}
	limit := max(PageSize, len(m.commits))
	return tea.Batch(m.loadLogN(0, limit, false), m.loadWIP(), m.loadTrack(), m.loadBranch(), m.loadStashes())
}

// selectedKey identifies the selected row for panels: wipKey or the hash.
func (m Model) selectedKey() string {
	if h := m.selectedHash(); h != "" {
		return h
	}
	return wipKey
}

// refreshPanels re-targets an open detail panel or preview after a log
// reload (once per reload): a different selected row reopens it; the same row
// reloads in place (the WIP row's files change with the working tree),
// keeping the scroll position and file selection.
func (m *Model) refreshPanels() tea.Cmd {
	if m.Selected() == nil {
		return nil
	}
	key := m.selectedKey()
	switch m.mode {
	case ModeDetail:
		if key != m.detail.key {
			return m.beginDetail()
		}
		return m.loadDetail()
	case ModePreviewFiles, ModePreviewDiff:
		if key != m.preview.key {
			next, cmd := m.openPreview()
			*m = next.(Model)
			return cmd
		}
		if key == wipKey {
			return m.loadPreviewFiles(true)
		}
	}
	return nil
}

// syncPanels keeps open panels on the selected row after a WIP, tracking or
// PR update, without running git for an unchanged row: a different selected
// row reopens the panel; the same row only has its labels recomposed. The
// reload that brought the update refreshes the content once, when its log
// arrives (refreshPanels).
func (m *Model) syncPanels() tea.Cmd {
	if m.Selected() == nil {
		return nil
	}
	key := m.selectedKey()
	switch m.mode {
	case ModeDetail:
		if key != m.detail.key {
			return m.beginDetail()
		}
		m.recomposeDetail()
	case ModePreviewFiles, ModePreviewDiff:
		if key != m.preview.key {
			next, cmd := m.openPreview()
			*m = next.(Model)
			return cmd
		}
	}
	return nil
}
