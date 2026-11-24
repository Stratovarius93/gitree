package ui

import (
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const helpKeyWidth = 8

// prLegend is the PR state names shown next to their icons, in spec order.
var prLegend = []struct {
	state github.PRState
	name  string
}{
	{github.PROpen, "Open"},
	{github.PRDraft, "Draft"},
	{github.PRApproved, "Approved"},
	{github.PRChangesRequested, "Changes requested"},
	{github.PRMerged, "Merged"},
	{github.PRClosed, "Closed without merge"},
}

// helpLines is the content of the overlay: the key table (from the key
// bindings), the PR icon legend and the label icons.
func helpLines() []string {
	bold, gray := floatBold, floatMuted
	lines := []string{bold.Render(" Keys"), ""}
	for _, b := range keys.helpBindings() {
		h := b.Help()
		lines = append(lines, " "+bold.Render(padTo(h.Key, helpKeyWidth))+" "+h.Desc)
	}
	lines = append(lines, "", bold.Render(" Can't … dialogs"), "",
		" "+gray.Render("say why an action can't run and what you can do:"),
		" "+bold.Render(padTo("s", helpKeyWidth))+" Stash changes, pull, and restore them",
		" "+bold.Render(padTo("s", helpKeyWidth))+" Stash changes, check out, then restore them",
		" "+padTo("", helpKeyWidth)+" "+gray.Render("opt-in: asks y first; p and c alone never stash"),
		" "+bold.Render(padTo("L", helpKeyWidth))+" Open lazygit (commit, stash, finish an operation)",
		" "+bold.Render(padTo("P b c f", helpKeyWidth))+" Push · find a branch · check out · fetch")
	lines = append(lines, "", bold.Render(" PR icons"), "")
	for _, e := range prLegend {
		g, hex := PRIcon(e.state)
		icon := lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(padTo(g, helpKeyWidth))
		lines = append(lines, " "+icon+" "+e.name)
	}
	lines = append(lines, "", bold.Render(" Labels"), "",
		" "+padTo(iconLocal, helpKeyWidth)+" local branch",
		" "+padTo(iconRemote, helpKeyWidth)+" remote branch",
		" "+padTo(iconTag, helpKeyWidth)+" tag",
		"", gray.Render(" j/k scroll · esc or ? close"))
	return lines
}

// helpWindow is the help overlay, sized to its content and scrolled to helpOff.
func (m Model) helpWindow() (floatWin, bool) {
	content := helpLines()
	win, ok := m.fitFloat("Help", content, 4)
	if !ok {
		return win, false
	}
	ih := win.h - 2
	off := max(min(m.helpOff, len(content)-ih), 0)
	win.body = content[off:min(off+ih, len(content))]
	return win, true
}

// helpMaxOff is the largest useful scroll offset: the last line at the bottom.
func (m Model) helpMaxOff() int {
	lines := helpLines()
	win, ok := m.fitFloat("Help", lines, 4)
	if !ok {
		return 0
	}
	return max(len(lines)-(win.h-2), 0)
}

// updateHelp closes the overlay on esc or ?, scrolls on j/k.
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Esc), key.Matches(msg, keys.Help):
		m.mode = ModeList
	case key.Matches(msg, keys.Down):
		m.helpOff = min(m.helpOff+1, m.helpMaxOff())
	case key.Matches(msg, keys.Up):
		m.helpOff = max(m.helpOff-1, 0)
	}
	return m, nil
}
