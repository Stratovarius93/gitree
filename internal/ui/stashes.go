package ui

import (
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// stashesLoadedMsg lists the stash entries gitree made for a checkout; seq is
// the action count when the listing started.
type stashesLoadedMsg struct {
	entries []git.StashEntry
	seq     int
	err     error
}

func (m Model) loadStashes() tea.Cmd {
	repo, seq := m.repo, m.actSeq
	return func() tea.Msg {
		es, err := repo.GitreeStashes()
		return stashesLoadedMsg{entries: es, seq: seq, err: err}
	}
}

func (m *Model) markStashSeen(hash string) {
	if m.stashSeen == nil {
		m.stashSeen = map[string]bool{}
	}
	if hash != "" {
		m.stashSeen[hash] = true
	}
}

// onStashes announces, once per entry and session, changes left in the stash
// by a checkout that never restored them (gitree quit or was killed between
// stashing and restoring). Listings made around an action are ignored: its
// own temporary entry is not an interrupted one. The dialog only opens over
// the list; otherwise the next reload tries again.
func (m Model) onStashes(msg stashesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || m.acting || msg.seq != m.actSeq || m.mode != ModeList {
		return m, nil
	}
	var fresh []git.StashEntry
	for _, e := range msg.entries {
		if !m.stashSeen[e.Hash] {
			fresh = append(fresh, e)
		}
	}
	if len(fresh) == 0 {
		return m, nil
	}
	for _, e := range fresh {
		m.markStashSeen(e.Hash)
	}
	n := noticeState{title: "Changes in the stash",
		text: "Changes from an interrupted checkout are in the stash (" + fresh[0].Ref + ": " + fresh[0].Message + ")."}
	for _, e := range fresh[1:] {
		n.details = append(n.details, "Also "+e.Ref+": "+e.Message+".")
	}
	n.opts = []noticeOption{optLazygit("Restore them in lazygit")}
	m.notice = n
	m.mode = ModeNotice
	return m, nil
}

// quitKey reports whether msg would quit gitree in the current view.
func (m Model) quitKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlC {
		return true
	}
	switch m.mode {
	case ModeList, ModeDetail, ModeHelp, ModePreviewFiles, ModePreviewDiff:
		return key.Matches(msg, keys.Quit)
	}
	return false
}

// askQuit is the question before quitting while an action runs: quitting
// then could leave a checkout's changes in the stash. Only y (or a second
// ctrl+c) quits.
func (m Model) askQuit() (tea.Model, tea.Cmd) {
	m.confirm = confirmState{title: "Quit", quit: true,
		text: "An action is running. Quit anyway? Your changes may be left in the stash.",
		yes:  func(m Model) (Model, tea.Cmd) { return m, tea.Quit }}
	m.mode = ModeConfirm
	return m, nil
}
