package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// RefsPollInterval is how often HEAD and the refs are checked for changes
// made outside gitree (another terminal, an editor).
const RefsPollInterval = 2 * time.Second

// WIPPollInterval is how often the working tree is checked; git status is the
// expensive part on large repos, hence the slower cadence.
const WIPPollInterval = 10 * time.Second

type (
	refsPollTickMsg struct{}
	wipPollTickMsg  struct{}
	// fingerprintMsg is the result of a refs check.
	fingerprintMsg struct {
		fp  string
		err error
	}
)

// EnablePolling makes the model follow outside changes on its own. It is
// opt-in (main enables it) so tests that settle Init never wait on a timer.
func (m Model) EnablePolling() Model {
	m.polling = true
	return m
}

func (m Model) refsPollTick() tea.Cmd {
	if !m.polling {
		return nil
	}
	return tea.Tick(RefsPollInterval, func(time.Time) tea.Msg { return refsPollTickMsg{} })
}

func (m Model) wipPollTick() tea.Cmd {
	if !m.polling {
		return nil
	}
	return tea.Tick(WIPPollInterval, func(time.Time) tea.Msg { return wipPollTickMsg{} })
}

// pollPaused reports whether checks must wait: lazygit has the terminal, or
// gitree's own git commands or loads are running (a check could race them,
// and their reload brings the changes anyway).
func (m Model) pollPaused() bool {
	return m.external || m.acting || m.fetching || m.loading
}

func (m Model) checkRefs() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		fp, err := repo.Fingerprint()
		return fingerprintMsg{fp: fp, err: err}
	}
}

func (m Model) onRefsPollTick() (tea.Model, tea.Cmd) {
	if m.pollPaused() {
		return m, m.refsPollTick()
	}
	return m, tea.Batch(m.checkRefs(), m.refsPollTick())
}

func (m Model) onWIPPollTick() (tea.Model, tea.Cmd) {
	if m.pollPaused() {
		return m, m.wipPollTick()
	}
	return m, tea.Batch(m.pollWIP(), m.wipPollTick())
}

// onFocus checks right away when the terminal window gets focus back.
func (m Model) onFocus() (tea.Model, tea.Cmd) {
	if m.pollPaused() {
		return m, nil
	}
	return m, tea.Batch(m.checkRefs(), m.pollWIP())
}

// pollWIP is loadWIP marked as a poll: its result keeps the selection and
// refreshes an open WIP panel (see the wipLoadedMsg case in update).
func (m Model) pollWIP() tea.Cmd {
	load := m.loadWIP()
	return func() tea.Msg {
		msg := load().(wipLoadedMsg)
		msg.poll = true
		return msg
	}
}

// onFingerprint reloads when the refs changed since the last full log load
// (which records its own fingerprint, so gitree's own changes are absorbed).
// A failed check is ignored: git may be busy (a lock held elsewhere) and the
// next tick tries again.
func (m Model) onFingerprint(msg fingerprintMsg) (tea.Model, tea.Cmd) {
	// An unknown fingerprint ("": the last load could not take one) counts
	// as changed, so a failed check at load time does not stop polling.
	if msg.err != nil || m.pollPaused() || msg.fp == m.fingerprint {
		return m, nil
	}
	return m, m.reloadAll()
}
