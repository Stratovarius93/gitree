package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// MenuAction is what a menu item does.
type MenuAction int

const (
	ActCheckout MenuAction = iota
	ActMerge
	ActCherryPick
	ActCreateBranch
	ActCopyBranch
	ActCopyHash
	ActCopyShortHash
	ActCopyMessage
	ActCopyPRURL
	ActOpenPR
)

// MenuItem is one line of the action menu; Separator items are a rule only.
type MenuItem struct {
	Key       string
	Label     string
	Action    MenuAction
	Separator bool
}

// BranchMenu is the menu for branch l when current is checked out ("" when
// HEAD is detached). Merge is offered only into another, checked-out branch;
// the PR items only when l has a PR.
func BranchMenu(l Label, current string) []MenuItem {
	items := []MenuItem{{Key: "c", Label: "Check out", Action: ActCheckout}}
	if current != "" && l.Name != current {
		items = append(items, MenuItem{Key: "m", Label: "Merge into " + current, Action: ActMerge})
	}
	items = append(items,
		MenuItem{Key: "b", Label: "Create branch here", Action: ActCreateBranch},
		MenuItem{Separator: true},
		MenuItem{Key: "y", Label: "Copy branch name", Action: ActCopyBranch},
		MenuItem{Key: "h", Label: "Copy hash", Action: ActCopyHash},
	)
	if l.PR != nil {
		n := strconv.Itoa(l.PR.Number)
		items = append(items,
			MenuItem{Key: "u", Label: "Copy PR URL #" + n, Action: ActCopyPRURL},
			MenuItem{Key: "o", Label: "Open PR #" + n + " in browser", Action: ActOpenPR},
		)
	}
	return items
}

// CommitMenu is the menu for commit c when current is checked out ("" when
// HEAD is detached: there is no branch to cherry-pick onto). Merge commits
// cannot be cherry-picked without choosing a parent, so they get no `p`.
func CommitMenu(c git.Commit, current string) []MenuItem {
	items := []MenuItem{{Key: "c", Label: "Check out (detached)", Action: ActCheckout}}
	if current != "" && len(c.Parents) <= 1 {
		items = append(items, MenuItem{Key: "p", Label: "Cherry-pick onto " + current, Action: ActCherryPick})
	}
	return append(items,
		MenuItem{Key: "b", Label: "Create branch here", Action: ActCreateBranch},
		MenuItem{Separator: true},
		MenuItem{Key: "y", Label: "Copy hash", Action: ActCopyHash},
		MenuItem{Key: "Y", Label: "Copy short hash", Action: ActCopyShortHash},
		MenuItem{Key: "M", Label: "Copy message", Action: ActCopyMessage},
	)
}

// menuState is the open action menu (ModeMenu). With choices set, the row has
// several branches and the menu first asks which one.
type menuState struct {
	title   string
	commit  git.Commit
	label   *Label // the chosen branch; nil for the commit menu
	choices []Label
	items   []MenuItem
	sel     int
	// checkout: the chooser of the direct `c`; picking a branch asks to
	// check it out instead of opening its menu.
	checkout bool
}

// confirmState is a yes/no question (ModeConfirm); yes runs on y only, so
// the enter that picked a menu item can never also confirm it.
type confirmState struct {
	title string
	text  string
	yes   func(Model) (Model, tea.Cmd)
	quit  bool // the quit question: answerable while an action runs
}

// inputState is the branch-name prompt (ModeInput) for "Create branch here".
type inputState struct {
	field textinput.Model
	at    string // commit hash the branch is created at
	err   string
}

// branchLabels are the labels of r that the branch menu applies to.
func branchLabels(labels []Label) []Label {
	var out []Label
	for _, l := range labels {
		if l.Kind == LabelBranch || l.Kind == LabelRemote {
			out = append(out, l)
		}
	}
	return out
}

// openMenu opens the action menu for the selected commit row.
func (m Model) openMenu() (tea.Model, tea.Cmd) {
	r := m.Selected()
	if r == nil || r.Commit == nil {
		return m, nil
	}
	if m.acting {
		return m, m.flash("action", statusBusy)
	}
	m.menu = menuState{commit: *r.Commit}
	switch bs := branchLabels(r.Labels); len(bs) {
	case 0:
		m.menu.title = shortHash(r.Commit.Hash) + " " + r.Commit.Subject
		m.menu.items = CommitMenu(*r.Commit, m.branch)
	case 1:
		m.menu.chooseBranch(bs[0], m.branch)
	default:
		m.menu.title = "Which branch?"
		m.menu.choices = bs
	}
	m.mode = ModeMenu
	return m, nil
}

func (s *menuState) chooseBranch(l Label, current string) {
	s.label = &l
	s.title = l.Name
	s.choices = nil
	s.items = BranchMenu(l, current)
	s.sel = 0
}

// menuLen is the number of selectable lines (choices or items).
func (s menuState) menuLen() int {
	if s.choices != nil {
		return len(s.choices)
	}
	return len(s.items)
}

// step moves the highlight by dir, skipping separators.
func (s *menuState) step(dir int) {
	n := s.menuLen()
	for i := s.sel + dir; i >= 0 && i < n; i += dir {
		if s.choices != nil || !s.items[i].Separator {
			s.sel = i
			return
		}
	}
}

func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case key.Matches(msg, keys.Esc):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, keys.Down):
		m.menu.step(1)
		return m, nil
	case key.Matches(msg, keys.Up):
		m.menu.step(-1)
		return m, nil
	}
	if m.menu.choices != nil {
		i := -1
		if key.Matches(msg, keys.Enter) {
			i = m.menu.sel
		} else if n, err := strconv.Atoi(msg.String()); err == nil {
			i = n - 1
		}
		if i >= 0 && i < len(m.menu.choices) {
			if m.menu.checkout {
				m.mode = ModeList
				l := m.menu.choices[i]
				return m.askCheckout(m.menu.commit, &l)
			}
			m.menu.chooseBranch(m.menu.choices[i], m.branch)
		}
		return m, nil
	}
	for i, it := range m.menu.items {
		if it.Separator {
			continue
		}
		if msg.String() == it.Key || (key.Matches(msg, keys.Enter) && i == m.menu.sel) {
			m.mode = ModeList
			return m.runMenuItem(it)
		}
	}
	return m, nil
}

// askConfirm opens a yes/no question.
func (m Model) askConfirm(title, text string, yes func(Model) (Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	m.confirm = confirmState{title: title, text: text, yes: yes}
	m.mode = ModeConfirm
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case msg.String() == "y":
		m.mode = ModeList
		if m.acting && !m.confirm.quit {
			return m, m.flash("action", statusBusy)
		}
		return m.confirm.yes(m)
	case msg.String() == "n", key.Matches(msg, keys.Esc):
		m.mode = ModeList
	}
	return m, nil
}

// openInput asks for the name of a branch to create at hash.
func (m Model) openInput(hash string) (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = "name: "
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Width = 40
	ti.Focus()
	m.input = inputState{field: ti, at: hash}
	m.mode = ModeInput
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case key.Matches(msg, keys.Esc):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, keys.Enter):
		name := strings.TrimSpace(m.input.field.Value())
		if !m.repo.ValidBranchName(name) {
			m.input.err = "invalid branch name"
			return m, nil
		}
		if m.repo.BranchExists(name) {
			m.input.err = "branch already exists"
			return m, nil
		}
		m.mode = ModeList
		at := m.input.at
		cmd := m.act("branch", name, at, func(r *git.Repo) error { return r.CreateBranch(name, at) })
		return m, cmd
	}
	m.input.err = ""
	var cmd tea.Cmd
	m.input.field, cmd = m.input.field.Update(msg)
	return m, cmd
}

// keepFooter shortens content to at most n lines on a short terminal. The
// last line (the key hint) and the last body line (the question) are kept;
// blank lines go first, then earlier body lines.
func keepFooter(content []string, n int) []string {
	if n < 1 || len(content) <= n {
		return content
	}
	footer := content[len(content)-1]
	var body []string
	for _, l := range content[:len(content)-1] {
		if l != "" {
			body = append(body, l)
		}
	}
	keep := min(len(body), n-1)
	return append(slices.Clone(body[len(body)-keep:]), footer)
}

// dialogWrap is the widest a dialog's text gets before wrapping.
const dialogWrap = 56

// dialogWindow is the open menu, confirmation or input, sized to its content.
func (m Model) dialogWindow() (floatWin, bool) {
	gray := floatMuted
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.Yellow))
	wrapW := max(min(dialogWrap, m.width-6), 10)
	var title string
	var content []string
	sel := -1
	switch m.mode {
	case ModeMenu:
		title = m.menu.title
		sel = m.menu.sel
		if m.menu.choices != nil {
			for i, l := range m.menu.choices {
				content = append(content, " "+keyStyle.Render(strconv.Itoa(i+1))+"  "+l.Name+" ")
			}
		} else {
			wide := 0
			for _, it := range m.menu.items {
				wide = max(wide, lipgloss.Width(it.Label)+4)
			}
			for _, it := range m.menu.items {
				if it.Separator {
					content = append(content, " "+lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Bg5)).Render(strings.Repeat("─", wide))+" ")
					continue
				}
				content = append(content, " "+keyStyle.Render(padTo(it.Key, 1))+"  "+it.Label+" ")
			}
		}
		content = append(content, gray.Render(" esc close"))
	case ModeConfirm:
		title = m.confirm.title
		for _, l := range strings.Split(ansi.Wordwrap(m.confirm.text, wrapW, ""), "\n") {
			content = append(content, " "+l+" ")
		}
		content = append(content, "", gray.Render(" y yes · n/esc no "))
	case ModeNotice:
		title = m.notice.title
		content = m.noticeContent(wrapW, m.height-statusLines-2)
	case ModeInput:
		title = "Create branch at " + shortHash(m.input.at)
		content = append(content, " "+m.input.field.View()+" ")
		if m.input.err != "" {
			content = append(content, " "+lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Red)).Render(m.input.err))
		}
		content = append(content, "", gray.Render(" enter create · esc cancel "))
	default:
		return floatWin{}, false
	}
	switch n := m.height - statusLines - 2; {
	case m.mode == ModeInput && n == 1:
		content = content[:1] // the name field matters more than its hint
	case m.mode == ModeConfirm || m.mode == ModeInput:
		content = keepFooter(content, n)
	}
	win, ok := m.fitFloat(title, content, 2)
	if !ok {
		return win, false
	}
	win.body, win.sel = content[:win.h-2], sel
	return win, true
}
