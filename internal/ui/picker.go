package ui

import (
	"strconv"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickerState is the branch picker (ModeBranchPicker): a filter typed into
// input over every branch, remote branch and tag of the repo.
type pickerState struct {
	input   textinput.Model
	gen     int // identifies the refs load of this opening
	loading bool
	err     string
	refs    []git.RefInfo // ordered local → remote → tag (see git.ListRefs)
	hits    []int         // indexes into refs that match the filter, in order
	sel     int           // index into hits
	off     int           // first visible hit
}

// jumpState is a jump to a ref's commit that is still loading pages.
type jumpState struct {
	hash     string // "" when no jump is in progress
	name     string
	switched bool // the scope was switched to all branches to find it
	base     int  // commits loaded in this scope before the jump paged any
}

// refsLoadedMsg carries the refs for the picker opening gen.
type refsLoadedMsg struct {
	gen  int
	refs []git.RefInfo
	err  error
}

// Picker layout: the widest the window gets and how many refs it lists.
const (
	pickerMaxWidth = 56
	pickerMaxRows  = 12
)

// jumpPageSize is how many commits each page loaded by a jump asks for.
const jumpPageSize = 2000

// jumpMaxCommits caps how many commits a jump loads in one scope before
// giving up (a var so tests can lower it).
var jumpMaxCommits = 20000

// filterRefs returns the indexes of refs whose name contains q, ignoring case.
func filterRefs(refs []git.RefInfo, q string) []int {
	q = strings.ToLower(strings.TrimSpace(q))
	hits := make([]int, 0, len(refs))
	for i, r := range refs {
		if strings.Contains(strings.ToLower(r.Name), q) {
			hits = append(hits, i)
		}
	}
	return hits
}

// pickerWidth is the inner width of the picker window.
func (m Model) pickerWidth() int {
	return max(min(pickerMaxWidth, m.width-2), 1)
}

// pickerRows is how many refs the window lists: fixed for a terminal size so
// the window does not jump while the filter changes.
func (m Model) pickerRows() int {
	avail := max(m.height-statusLines, 0)
	return max(min(pickerMaxRows, avail-4), 1) // borders, input, footer
}

func (m Model) openPicker() (tea.Model, tea.Cmd) {
	if m.mode == ModeDetail {
		m.detail.focus = false
		m.detail.seq++ // drop a pending debounced load
	}
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "branch or tag name"
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Width = max(m.pickerWidth()-6, 1) // " " prompt value cursor
	ti.Focus()
	m.picker = pickerState{input: ti, gen: m.picker.gen + 1, loading: true}
	m.mode = ModeBranchPicker
	m.scrollToCursor()
	repo, gen := m.repo, m.picker.gen
	return m, func() tea.Msg {
		refs, err := repo.ListRefs()
		return refsLoadedMsg{gen: gen, refs: refs, err: err}
	}
}

func (m Model) onRefs(msg refsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.picker.gen {
		return m, nil // an earlier opening of the picker
	}
	m.picker.loading = false
	if msg.err != nil {
		m.picker.err = "error: " + oneLine(msg.err.Error())
		return m, nil
	}
	m.picker.refs = msg.refs
	m.refilterPicker()
	return m, nil
}

func (m *Model) refilterPicker() {
	p := &m.picker
	p.hits = filterRefs(p.refs, p.input.Value())
	p.sel, p.off = 0, 0
}

func (m *Model) pickerStep(dir int) {
	p := &m.picker
	p.sel = max(min(p.sel+dir, len(p.hits)-1), 0)
	p.off = visibleOffset(p.off, p.sel, m.pickerRows(), len(p.hits))
}

// updatePicker routes keys while the picker is open: every printable key goes
// to the filter (so j, k, q… can be typed); arrows and ctrl+n/p move.
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = ModeList
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		m.pickerStep(1)
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		m.pickerStep(-1)
		return m, nil
	case tea.KeyPgDown:
		m.pickerStep(m.pickerRows())
		return m, nil
	case tea.KeyPgUp:
		m.pickerStep(-m.pickerRows())
		return m, nil
	case tea.KeyEnter:
		p := m.picker
		if p.sel < 0 || p.sel >= len(p.hits) {
			return m, nil
		}
		m.mode = ModeList
		return m.startJump(p.refs[p.hits[p.sel]])
	}
	before := m.picker.input.Value()
	var cmd tea.Cmd
	m.picker.input, cmd = m.picker.input.Update(msg)
	if m.picker.input.Value() != before {
		m.refilterPicker()
	}
	return m, cmd
}

// refPR is the PR of a branch ref, if any (tags have none).
func (m Model) refPR(r git.RefInfo) *github.PR {
	switch r.Kind {
	case git.LocalBranch:
		return findPR(m.prs, r.Name, m.track[r.Name].UpstreamBranch)
	case git.RemoteBranch:
		return findPR(m.prs, strings.TrimPrefix(r.Name, r.Remote+"/"))
	}
	return nil
}

// pickerRow renders one ref, w cells wide: icon, name and PR chip.
func (m Model) pickerRow(r git.RefInfo, selected bool, w int) string {
	base := floatText
	if selected {
		base = base.Bold(true)
	}
	icon, hex := iconLocal, colors.Green
	switch r.Kind {
	case git.RemoteBranch:
		icon, hex = iconRemote, colors.Gray
	case git.Tag:
		icon, hex = iconTag, colors.Yellow
	}
	chip, chipW := "", 0
	if pr := m.refPR(r); pr != nil {
		text, prHex := prChipText(pr)
		if tw := lipgloss.Width(text); tw <= w-8 {
			chip, chipW = prChip(pr, text, prHex), tw
		}
	}
	name := highlightMatch(r.Name, m.picker.input.Value(), max(w-5-chipW, 0), base) // " " icon "  " name chip " "
	line := base.Render(" ") + base.Foreground(lipgloss.Color(hex)).Render(icon) + base.Render("  ") + name + chip + base.Render(" ")
	return fit(line, w)
}

// pickerWindow is the branch picker: filter input, ref list and footer.
func (m Model) pickerWindow() (floatWin, bool) {
	p := m.picker
	w, rows := m.pickerWidth(), m.pickerRows()
	gray := floatMuted
	content := []string{padTo(" "+p.input.View(), w)}
	sel := -1
	var list []string
	switch {
	case p.loading:
		list = append(list, gray.Render(" "+m.spinning("loading…")))
	case p.err != "":
		list = append(list, lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Red)).Render(" "+p.err))
	case len(p.hits) == 0:
		list = append(list, gray.Render(" no results"))
	default:
		off := visibleOffset(p.off, p.sel, rows, len(p.hits))
		for i := off; i < min(off+rows, len(p.hits)); i++ {
			if i == p.sel {
				sel = len(content) + len(list)
			}
			list = append(list, m.pickerRow(p.refs[p.hits[i]], i == p.sel, w))
		}
	}
	for len(list) < rows {
		list = append(list, "")
	}
	content = append(content, list...)
	content = append(content, gray.Render(padTo(pickerFooter(p, w), w)))
	win, ok := m.fitFloat("Find branch", content, 2)
	if !ok {
		return win, false
	}
	win.body, win.sel = content[:win.h-2], sel
	return win, true
}

// pickerFooter is the key hint (j/k type into the filter, so arrows and
// ctrl+n/p move), with the position when it fits in w cells.
func pickerFooter(p pickerState, w int) string {
	pos := ""
	if len(p.hits) > 0 {
		pos = " " + strconv.Itoa(p.sel+1) + "/" + strconv.Itoa(len(p.hits)) + " ·"
	}
	for _, f := range []string{
		pos + " ↑↓ ctrl+n/p move · enter go · esc close",
		" ↑↓ ctrl+n/p move · enter go · esc close",
		" ↑↓ ^n/^p move · enter go · esc close",
	} {
		if lipgloss.Width(f) <= w {
			return f
		}
	}
	return " ↑↓ ^n/^p · enter · esc"
}

// startJump selects ref's commit: at once when it is loaded, otherwise by
// loading pages of the current scope and, if the ref is not in it, of all
// branches (see continueJump).
func (m Model) startJump(ref git.RefInfo) (tea.Model, tea.Cmd) {
	m.cancelJump()
	for i, r := range m.rows {
		if r.Commit != nil && r.Commit.Hash == ref.Hash {
			return m.moveTo(i)
		}
	}
	m.userMoved = true
	m.jump = jumpState{hash: ref.Hash, name: ref.Name, base: len(m.commits)}
	m.pendingSelect = ref.Hash
	if m.scope == git.ScopeDefault && ref.Kind == git.RemoteBranch && !m.isUpstream(ref.Name) {
		return m, m.jumpToAll() // not walked by the default view
	}
	m.setStatus("jump", searchingText(ref.Name))
	if m.loading {
		return m, nil // the load in flight continues the jump (onLog)
	}
	cmd, _ := m.continueJump()
	return m, cmd
}

func searchingText(name string) string { return "searching " + name + "… (esc cancels)" }

// isUpstream reports whether remote branch name is the upstream of a local
// branch, which puts it in the default view.
func (m Model) isUpstream(name string) bool {
	for _, t := range m.track {
		if t.Upstream == name {
			return true
		}
	}
	return false
}

// continueJump runs after each log load while a jump is in progress: it
// finishes the jump when the commit is among the rows, selecting it and
// refreshing an open detail or preview (done reports that refresh); else it requests the next page,
// else switches the default view to all branches, else gives up.
func (m *Model) continueJump() (cmd tea.Cmd, done bool) {
	j := m.jump
	for i, r := range m.rows {
		if r.Commit != nil && r.Commit.Hash == j.hash {
			m.cancelJump()
			m.cursor = i
			m.scrollToCursor()
			cmds := []tea.Cmd{m.maybeLoadMore(), m.refreshPanels()}
			if j.switched {
				cmds = append(cmds, m.flash("jump", "switched to all branches to show "+j.name))
			}
			return tea.Batch(cmds...), true
		}
	}
	if paged := len(m.commits) - j.base; m.more && paged < jumpMaxCommits {
		if !j.switched {
			m.setStatus("jump", searchingText(j.name))
		}
		m.loading = true
		return m.loadLogN(len(m.commits), min(jumpPageSize, jumpMaxCommits-paged), true), false
	}
	if m.scope == git.ScopeDefault {
		return m.jumpToAll(), false
	}
	m.cancelJump()
	text := "could not find " + j.name
	if m.more {
		text += " in the first " + strconv.Itoa(len(m.commits)) + " commits"
	}
	return m.flash("jump", text), false
}

// jumpToAll switches to all branches and reloads; the jump continues there.
func (m *Model) jumpToAll() tea.Cmd {
	m.scope = git.ScopeAll
	m.jump.switched, m.jump.base = true, 0 // the reload starts the count again
	m.setStatus("jump", "switched to all branches to show "+m.jump.name)
	return m.reloadLog()
}

// cancelJump stops a jump in progress (the user moved, pressed esc or
// changed the scope); pages already requested still arrive.
func (m *Model) cancelJump() {
	if m.jump.hash == "" {
		return
	}
	if m.pendingSelect == m.jump.hash {
		m.pendingSelect = ""
	}
	m.jump = jumpState{}
	m.clearStatus("jump")
}
