package ui

import (
	"strconv"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Nerd Font glyphs of the status bar.
const (
	plArrowR    = "" // powerline separator, pointing right
	plArrowL    = "" // powerline separator, pointing left
	iconRepo    = ""
	iconBranch  = ""
	iconAll     = ""
	iconFetch   = ""
	iconVersion = ""
	iconOK      = ""
	iconWarn    = ""
	iconError   = ""
	iconInfo    = ""
	iconSearch  = ""
	iconPR      = ""
)

// minMessageRoom is the width a message keeps before optional segments show.
const minMessageRoom = 20

// maxBranchText caps the branch segment so a long name leaves room for the rest.
const maxBranchText = 40

// WithVersion sets the version shown at the right of the status bar.
func (m Model) WithVersion(v string) Model {
	m.version = v
	return m
}

// warnFlashes are transient texts that warn rather than confirm.
var warnFlashes = map[string]bool{
	statusBusy: true, statusNoLazygit: true, statusNoCheckout: true, "no PR": true,
}

// barSeg is one colored block of the bar.
type barSeg struct {
	text   string
	fg, bg string
	bold   bool
}

func (s barSeg) render() string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(s.fg)).Background(lipgloss.Color(s.bg)).Bold(s.bold)
	return st.Render(s.text)
}

func glyph(g, fg, bg string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg)).Render(g)
}

// powerLeft joins segs with right-pointing arrows and ends into the bar.
func powerLeft(segs []barSeg) string {
	var b strings.Builder
	for i, s := range segs {
		b.WriteString(s.render())
		next := colors.DimBg
		if i+1 < len(segs) {
			next = segs[i+1].bg
		}
		b.WriteString(glyph(plArrowR, s.bg, next))
	}
	return b.String()
}

// powerRight starts each seg with a left-pointing arrow out of what precedes it.
func powerRight(segs []barSeg) string {
	var b strings.Builder
	prev := colors.DimBg
	for _, s := range segs {
		b.WriteString(glyph(plArrowL, s.bg, prev))
		b.WriteString(s.render())
		prev = s.bg
	}
	return b.String()
}

// barItem is one colored text in the middle of the bar; busy items (spinners)
// come and go on their own and do not reserve room.
type barItem struct {
	text, fg string
	busy     bool
}

// statusItem is the status message with the icon and color of its kind.
func (m Model) statusItem() (barItem, bool) {
	t := m.status
	switch {
	case t == "":
		return barItem{}, false
	case strings.HasPrefix(t, "error"):
		return barItem{text: iconError + " " + t, fg: colors.Red}, true
	case warnFlashes[t] || m.statusWarn:
		return barItem{text: iconWarn + " " + t, fg: colors.Yellow}, true
	case m.acting && m.statusSrc == "action" && strings.HasSuffix(t, "…"):
		return barItem{text: m.spinning(t), fg: colors.Orange, busy: true}, true
	case m.statusOK:
		return barItem{text: iconOK + " " + t, fg: colors.Green}, true
	case m.statusSrc == "jump":
		return barItem{text: iconInfo + " " + t, fg: colors.Blue}, true
	default:
		return barItem{text: iconWarn + " " + t, fg: colors.Yellow}, true
	}
}

func (m Model) barItems() []barItem {
	var items []barItem
	if it, ok := m.statusItem(); ok {
		items = append(items, it)
	}
	switch {
	case m.search.query != "":
		items = append(items, barItem{text: iconSearch + " " + m.searchStatus(), fg: colors.Blue})
	case m.loading:
		items = append(items, barItem{text: m.spinning("loading…"), fg: colors.Orange, busy: true})
	case m.loaded && len(m.commits) == 0:
		items = append(items, barItem{text: "no commits", fg: colors.Gray})
	}
	if m.prNote != "" {
		items = append(items, barItem{text: iconPR + " " + m.prNote, fg: colors.Gray})
	}
	switch {
	case m.fetching:
		items = append(items, barItem{text: m.spinning("fetching…"), fg: colors.Orange, busy: true})
	case m.fetchErr != "":
		items = append(items, barItem{text: iconError + " " + m.fetchErr, fg: colors.Red})
	}
	return items
}

// branchSeg is the branch segment with its name capped at maxText cells.
func (m Model) branchSeg(maxText int) barSeg {
	if m.branch == "" {
		return barSeg{" " + iconBranch + " detached HEAD ", colors.DarkText, colors.Orange, true}
	}
	text := ansi.Truncate(noNewlines.Replace(m.branch), max(maxText, 1), "…")
	if t, ok := m.track[m.branch]; ok {
		if t.Ahead > 0 {
			text += " ↑" + strconv.Itoa(t.Ahead)
		}
		if t.Behind > 0 {
			text += " ↓" + strconv.Itoa(t.Behind)
		}
	}
	return barSeg{" " + iconBranch + " " + noNewlines.Replace(text) + " ", colors.DarkText, colors.Green, true}
}

func (m Model) scopeSeg() barSeg {
	icon := iconLocal
	if m.scope == git.ScopeAll {
		icon = iconAll
	}
	return barSeg{" " + icon + " " + m.scopeLabel() + " ", colors.Fg, colors.SelBg, false}
}

// statusBar renders the Powerline status bar, exactly m.width cells: repo,
// branch and scope on the left, messages in the middle, last fetch and the
// version on the right. When space is short the fetch, scope and version
// segments are dropped, in that order, until a message has minMessageRoom
// cells; the middle is then truncated.
func (m Model) statusBar() string {
	repo := barSeg{" " + iconRepo + " " + oneLine(m.repoName) + " ", colors.DarkText, colors.Aqua, true}
	version := m.version
	if version == "" {
		version = "dev"
	}
	ver := barSeg{" " + iconVersion + " " + version + " ", colors.DarkText, colors.Purple, true}
	var fetch *barSeg
	if !m.lastFetch.IsZero() {
		fetch = &barSeg{" " + iconFetch + " fetched " + m.lastFetch.Format("15:04") + " ", colors.Fg, colors.SelBg, false}
	}
	scope := m.scopeSeg()
	branch := m.branchSeg(maxBranchText)
	layouts := [][2][]barSeg{
		{{repo, branch, scope}, {ver}},
		{{repo, branch}, {ver}},
		{{repo, branch}, nil},
	}
	if fetch != nil {
		layouts = append([][2][]barSeg{{{repo, branch, scope}, {*fetch, ver}}}, layouts...)
	}
	items := m.barItems()
	minMid := 0
	for _, it := range items {
		if !it.busy {
			minMid = minMessageRoom // optional segments give way to a message
		}
	}
	var left, right string
	fits := false
	for _, l := range layouts {
		left, right = powerLeft(l[0]), powerRight(l[1])
		if lipgloss.Width(left)+lipgloss.Width(right)+minMid <= m.width {
			fits = true
			break
		}
	}
	if !fits {
		// The branch is never dropped: its name shrinks to what is left
		// (a one-cell name "…" measures the segment's fixed overhead).
		overhead := lipgloss.Width(powerLeft([]barSeg{repo, m.branchSeg(1)})) - 1
		room := m.width - minMid - overhead
		if room < 1 {
			room = m.width - overhead // too narrow to keep room for the message
		}
		left, right = powerLeft([]barSeg{repo, m.branchSeg(min(room, maxBranchText))}), ""
	}
	avail := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if avail < 0 {
		return fit(left+right, m.width)
	}
	var mid strings.Builder
	used := 0
	bg := lipgloss.NewStyle().Background(lipgloss.Color(colors.DimBg))
	for _, it := range items {
		text := " " + noNewlines.Replace(it.text) + " "
		if used+lipgloss.Width(text) > avail {
			text = ansi.Truncate(text, avail-used, "…")
		}
		mid.WriteString(bg.Foreground(lipgloss.Color(it.fg)).Render(text))
		used += lipgloss.Width(text)
		if used >= avail {
			break
		}
	}
	mid.WriteString(bg.Render(strings.Repeat(" ", max(avail-used, 0))))
	return left + mid.String() + right
}
