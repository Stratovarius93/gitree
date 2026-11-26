package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot))
}

// EnableSpinner animates the busy indicator. It is opt-in (main enables it) so
// tests that settle never wait on its timer; without it the first frame is
// shown, still.
func (m Model) EnableSpinner() Model {
	m.spinOn = true
	return m
}

// busy reports whether something the user waits for is running: a log load,
// a fetch, an action, or a panel that is still loading.
func (m Model) busy() bool {
	if m.loading || m.fetching || m.acting {
		return true
	}
	switch m.mode {
	case ModeDetail:
		return m.detail.loading
	case ModePreviewFiles:
		return m.preview.loading
	case ModePreviewDiff:
		return m.preview.diffLoading
	case ModeBranchPicker:
		return m.picker.loading
	}
	return false
}

// spinning turns an in-progress text such as "fetching…" into "fetching ⠋"
// with the current frame.
func (m Model) spinning(text string) string {
	return strings.TrimSuffix(text, "…") + " " + m.spin.View()
}

// onSpinTick advances the frame while busy; once idle the chain stops, so an
// idle gitree does not redraw.
func (m Model) onSpinTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.busy() {
		m.animating = false
		return m, nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

// startSpin starts the animation when the model just became busy.
func (m *Model) startSpin() tea.Cmd {
	if !m.spinOn || m.animating || !m.busy() {
		return nil
	}
	m.animating = true
	return m.spin.Tick
}
