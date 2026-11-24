package ui

import (
	"fmt"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// detailLoadedMsg is the detail of one commit (hash "" is the WIP row). It is
// applied only while that row is still selected.
type detailLoadedMsg struct {
	hash   string
	header string
	files  []git.FileChange
	diff   string
	err    error
}

// detailDebounceMsg fires detailDebounce after a cursor move in ModeDetail;
// only the latest one (seq) loads the row.
type detailDebounceMsg struct{ seq int }

// detailState is the right-hand panel shown in ModeDetail.
type detailState struct {
	vp    viewport.Model
	focus bool   // keys scroll the detail instead of moving the list cursor
	key   string // row the panel was opened for (wipKey or hash)
	shown string // row whose content the viewport currently holds
	seq   int    // current debounce / load request; older timers are ignored
	// data is the loaded content of shown; labels and PRs are composed in at
	// display time, so their updates need no reload.
	data *detailLoadedMsg
}

// listShare is the percentage of the width the list keeps while the detail is open.
const listShare = 45

// listWidth is the width of the commit table: the full width, or its share
// while the detail panel is open.
func (m Model) listWidth() int {
	if m.mode != ModeDetail {
		return m.width
	}
	return max(m.width*listShare/100, 1)
}

// detailWidth is the inner width of the detail panel (after the separator column).
func (m Model) detailWidth() int {
	return max(m.width-m.listWidth()-1, 1)
}

// detailHeight is the number of lines the panel spans (everything above the status bar).
func (m Model) detailHeight() int {
	return max(m.height-statusLines, 1)
}

// openDetail splits the screen and loads the selected row.
func (m Model) openDetail() (tea.Model, tea.Cmd) {
	if m.Selected() == nil {
		return m, nil
	}
	m.mode = ModeDetail
	m.detail.focus = false
	m.scrollToCursor()
	return m, m.beginDetail()
}

// beginDetail resets the panel to "loading" and returns the load command.
func (m *Model) beginDetail() tea.Cmd {
	m.resetDetail()
	return m.loadDetail()
}

// resetDetail points the panel at the selected row, showing "loading…", and
// cancels a pending debounced load.
func (m *Model) resetDetail() {
	m.detail.key, m.detail.shown, m.detail.data = m.selectedKey(), "", nil
	m.detail.seq++
	m.detail.vp = viewport.New(m.detailWidth(), m.detailHeight())
	m.detail.vp.SetContent(" loading…")
}

// debounceDetail is beginDetail for cursor moves: the row is loaded only if
// the cursor is still on it detailDebounce later.
func (m *Model) debounceDetail() tea.Cmd {
	m.resetDetail()
	return after(detailDebounce, detailDebounceMsg{seq: m.detail.seq})
}

func (m Model) onDetailDebounce(msg detailDebounceMsg) (tea.Model, tea.Cmd) {
	if m.mode != ModeDetail || msg.seq != m.detail.seq {
		return m, nil
	}
	return m, m.loadDetail()
}

// recomposeDetail redraws the loaded detail with the current labels and PRs
// (after tracking or PR updates), keeping the scroll position. No git runs.
func (m *Model) recomposeDetail() {
	if m.mode != ModeDetail || m.detail.data == nil || m.detail.shown != m.selectedKey() {
		return
	}
	off := m.detail.vp.YOffset
	m.detail.vp.SetContent(m.detailContent(*m.detail.data))
	m.detail.vp.SetYOffset(off)
}

func (m Model) loadDetail() tea.Cmd {
	if m.Selected() == nil {
		return nil
	}
	repo, hash, width := m.repo, m.selectedHash(), m.detailWidth()
	return func() tea.Msg {
		msg := detailLoadedMsg{hash: hash}
		var err error
		if hash != "" {
			if msg.header, err = repo.ShowHeader(hash); err != nil {
				msg.err = err
				return msg
			}
		}
		if hash == "" {
			msg.files, err = repo.WorkingFiles()
		} else {
			msg.files, err = repo.CommitFiles(hash)
		}
		if err != nil {
			msg.err = err
			return msg
		}
		msg.diff, msg.err = repo.Diff(git.DiffTarget{Hash: hash}, "", width)
		return msg
	}
}

// onDetail applies a loaded detail if it is still the one on screen.
func (m Model) onDetail(msg detailLoadedMsg) (tea.Model, tea.Cmd) {
	if m.mode != ModeDetail || m.Selected() == nil || msg.hash != m.selectedHash() {
		return m, nil
	}
	if msg.err != nil {
		m.detail.vp.SetContent(" error: " + msg.err.Error())
		return m, nil
	}
	key := wipKey
	if msg.hash != "" {
		key = msg.hash
	}
	off := m.detail.vp.YOffset
	m.detail.vp.SetContent(m.detailContent(msg))
	m.detail.vp.GotoTop()
	if m.detail.shown == key {
		m.detail.vp.SetYOffset(off) // in-place refresh: keep the scroll position
	}
	m.detail.shown, m.detail.data = key, &msg
	return m, nil
}

// resizeDetail rebuilds the viewport for the current size and reloads, since
// the diff is rendered for the panel width.
func (m *Model) resizeDetail() tea.Cmd {
	return m.beginDetail()
}

func (m Model) detailContent(d detailLoadedMsg) string {
	w := m.detailWidth()
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.Fg))
	gray := lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray))
	var b strings.Builder
	line := func(s string) { b.WriteString(ansi.Truncate(s, w, "…") + "\n") }

	if d.hash == "" {
		line(bold.Render(" " + wipMessage(len(d.files))))
	} else {
		for _, l := range strings.Split(d.header, "\n") {
			line(" " + l)
		}
	}
	if r := m.Selected(); r != nil {
		var names []string
		for _, l := range r.Labels {
			names = append(names, l.Name)
		}
		if len(names) > 0 {
			line("")
			line(gray.Render(" Labels: ") + strings.Join(names, ", "))
		}
		for _, l := range r.Labels {
			if pr := l.PR; pr != nil {
				line(gray.Render(" PR ") + bold.Render(fmt.Sprintf("#%d %s", pr.Number, pr.Title)))
				line(gray.Render(fmt.Sprintf("    %s ← %s", pr.Base, pr.Head)))
			}
		}
	}
	line("")
	line(bold.Render(fmt.Sprintf(" Files (%d)", len(d.files))))
	for _, f := range d.files {
		path := f.Path
		if f.OldPath != "" {
			path = f.OldPath + " → " + f.Path
		}
		line(fmt.Sprintf(" %c  %s  +%d −%d", f.Status, path, f.Added, f.Deleted))
	}
	line("")
	for _, l := range strings.Split(strings.TrimRight(d.diff, "\n"), "\n") {
		line(l)
	}
	return strings.TrimRight(b.String(), "\n")
}

// updateDetail routes keys in ModeDetail: tab moves focus, esc and enter
// close, and the rest drives either the detail scroll or the list (which
// reloads the detail once the cursor rests).
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Esc), key.Matches(msg, keys.Enter):
		m.mode = ModeList
		m.detail.focus = false
		m.detail.seq++ // drop a pending debounced load
		m.scrollToCursor()
		return m, nil
	case key.Matches(msg, keys.Tab):
		m.detail.focus = !m.detail.focus
		return m, nil
	}
	if m.detail.focus {
		vp := &m.detail.vp
		switch {
		case key.Matches(msg, keys.Down):
			vp.LineDown(1)
		case key.Matches(msg, keys.Up):
			vp.LineUp(1)
		case key.Matches(msg, keys.HalfDown):
			vp.HalfViewDown()
		case key.Matches(msg, keys.HalfUp):
			vp.HalfViewUp()
		case key.Matches(msg, keys.Top):
			vp.GotoTop()
		case key.Matches(msg, keys.Bottom):
			vp.GotoBottom()
		}
		return m, nil
	}
	before, hadRow := m.selectedHash(), m.Selected() != nil
	next, cmd := m.updateList(msg)
	m = next.(Model)
	if m.Selected() != nil && (!hadRow || m.selectedHash() != before) {
		return m, tea.Batch(cmd, m.debounceDetail())
	}
	return m, cmd
}

// joinDetail places the detail panel to the right of the list lines (each
// listW wide), separated by a rule that is brighter while the detail has focus.
func (m Model) joinDetail(list []string, listW int) []string {
	sepColor := colors.Separator
	if m.detail.focus {
		sepColor = colors.Gray
	}
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color(sepColor)).Render("│")
	panel := strings.Split(m.detail.vp.View(), "\n")
	n := max(m.height-statusLines, 0) // like the list, nothing fits on a 1-line terminal
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		l, p := "", ""
		if i < len(list) {
			l = list[i]
		}
		if i < len(panel) {
			p = panel[i]
		}
		l = ansi.Truncate(l, listW, "")
		l += strings.Repeat(" ", max(listW-lipgloss.Width(l), 0))
		out = append(out, ansi.Truncate(l+sep+p, m.width, ""))
	}
	return out
}
