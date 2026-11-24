package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/graph"
	"github.com/charmbracelet/lipgloss"
)

// RowKind distinguishes the WIP pseudo-row from commit rows.
type RowKind int

const (
	RowWIP RowKind = iota
	RowCommit
)

// Row is one table line, prepared for rendering.
type Row struct {
	Kind     RowKind
	Commit   *git.Commit
	Labels   []Label
	Graph    string
	Lane     int
	WIPCount int
}

// graphSymbols is the lazygit symbol set used for the graph. Box drawing is the
// safe default: the branch drawing set needs terminal-specific glyph support.
var graphSymbols = graph.BoxDrawingSymbols

// BuildRows renders the graph and labels for commits, prepending a WIP row
// when wip is non-empty. The WIP row sits in the lane of HEAD's commit. The
// model does not use it (it keeps the layout and renders only the visible
// window); it is the reference rendering for tests.
func BuildRows(commits []git.Commit, wip []git.FileChange, track map[string]git.Track, prs map[string]github.PR, selectedHash string) []Row {
	layout := newGraphLayout(commits)
	rows := layoutRows(commits, layout, wip, track, prs)
	sel := -1
	for i := range commits {
		if commits[i].Hash == selectedHash {
			sel = i
			break
		}
	}
	lines := layout.render(0, len(commits), sel)
	for i := range rows {
		if k := i - (len(rows) - len(commits)); rows[i].Kind == RowCommit && k < len(lines) {
			rows[i].Graph = lines[k]
		}
	}
	return rows
}

// graphLayout is the selection-independent part of the commit graph. It is
// computed only when the commits change; render then draws any window of it
// with the current selection highlighted, so moving the cursor costs only the
// window, and WIP, tracking or PR updates do not touch it at all.
type graphLayout struct {
	commits  []*graph.Commit
	pipeSets [][]graph.Pipe
	lanes    []int // commit column of each commit
	widths   []int // width of each commit's graph line
	headLane int   // lane of HEAD's commit (where the WIP row goes)
}

func newGraphLayout(commits []git.Commit) graphLayout {
	var g graphLayout
	if len(commits) == 0 {
		return g
	}
	opts := make([]graph.CommitOpts, len(commits))
	for i, c := range commits {
		opts[i] = graph.CommitOpts{Hash: c.Hash, Parents: c.Parents}
	}
	g.commits = graph.NewCommits(opts)
	g.pipeSets = graph.GetPipeSets(g.commits, colors.ForColumn)
	g.lanes = make([]int, len(commits))
	g.widths = make([]int, len(commits))
	foundHead := false
	for i, ps := range g.pipeSets {
		g.lanes[i] = graph.CommitColumn(ps)
		g.widths[i] = graph.PipeSetWidth(ps)
		if !foundHead {
			for _, ref := range commits[i].Refs {
				if ref.IsHead {
					g.headLane, foundHead = g.lanes[i], true
				}
			}
		}
	}
	return g
}

// render returns the graph lines of commits [from, to) with commit sel
// highlighted (sel < 0: no selection).
func (g graphLayout) render(from, to, sel int) []string {
	var selected *string
	if sel >= 0 && sel < len(g.commits) {
		selected = g.commits[sel].HashPtr()
	}
	return graph.RenderRange(g.pipeSets, g.commits, selected, graphSymbols, colors.ForColumn, from, to)
}

// maxWidth is the widest graph line among commits [from, to).
func (g graphLayout) maxWidth(from, to int) int {
	n := 0
	for _, w := range g.widths[max(from, 0):max(min(to, len(g.widths)), 0)] {
		n = max(n, w)
	}
	return n
}

// wipGraphWidth is the width of the WIP row's graph: its ◌ in lane.
func wipGraphWidth(lane int) int { return 2*lane + 1 }

// layoutRows builds the rows of commits (whose layout is given) with their
// labels, prepending the WIP row when wip is non-empty. It is linear in the
// commits and does no graph work; Row.Graph is left empty.
func layoutRows(commits []git.Commit, layout graphLayout, wip []git.FileChange, track map[string]git.Track, prs map[string]github.PR) []Row {
	rows := make([]Row, 0, len(commits)+1)
	if len(wip) > 0 {
		rows = append(rows, Row{Kind: RowWIP, Lane: layout.headLane, WIPCount: len(wip)})
	}
	for i := range commits {
		c := &commits[i]
		r := Row{Kind: RowCommit, Commit: c}
		if len(c.Refs) > 0 {
			r.Labels = BuildLabels(c.Refs, track, prs)
		}
		if i < len(layout.lanes) {
			r.Lane = layout.lanes[i]
		}
		rows = append(rows, r)
	}
	return rows
}

// GraphWidth is the widest graph cell among rows (rendered by BuildRows).
func GraphWidth(rows []Row) int {
	n := 0
	for _, r := range rows {
		n = max(n, lipgloss.Width(r.Graph))
	}
	return n
}

func wipMessage(n int) string {
	if n == 1 {
		return "1 file changed"
	}
	return strconv.Itoa(n) + " files changed"
}

// RenderRow renders a row as exactly w's total width.
func RenderRow(r Row, w Widths, selected bool, now time.Time) string {
	hex := colors.HexForColumn(r.Lane)
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(lipgloss.Color(colors.DimBg))
	}
	text := func(s string, width int, fg string) string {
		return base.Foreground(lipgloss.Color(fg)).Render(fit(s, width))
	}

	var label, gph, msg, author, date, hash string
	switch r.Kind {
	case RowWIP:
		label = RenderLabels([]Label{{Name: "// WIP", Kind: LabelBranch}}, hex, w.Label)
		gph = fit(base.Foreground(lipgloss.Color(hex)).Render(strings.Repeat("  ", r.Lane)+"◌"), w.Graph)
		msg = text(wipMessage(r.WIPCount), w.Message, colors.Fg)
		author, date, hash = text("", w.Author, colors.Gray), text("", w.Date, colors.Gray), text("", w.Hash, colors.Gray)
	default:
		c := r.Commit
		label = RenderLabels(r.Labels, hex, w.Label)
		gph = fit(r.Graph, w.Graph)
		msg = text(c.Subject, w.Message, colors.Fg)
		author = text(authorName(c.AuthorName), w.Author, colors.Fg)
		date = text(RelativeTime(c.Time, now), w.Date, colors.Gray)
		short := c.Hash
		if len(short) > 7 {
			short = short[:7]
		}
		hash = text(short, w.Hash, colors.Gray)
	}
	if selected {
		label, gph = withBg(label, colors.DimBg), withBg(gph, colors.DimBg)
	}
	byKey := map[string]string{"label": label, "graph": gph, "message": msg, "author": author, "date": date, "hash": hash}
	sep := base.Foreground(lipgloss.Color(colors.Separator)).Render(" ┊ ")
	var parts []string
	for _, c := range w.cells() {
		parts = append(parts, byKey[c.key])
	}
	return strings.Join(parts, sep)
}

// withBg gives s a background that survives the resets inside it: the
// background escape is emitted first and re-applied after every reset
// (full or background-only).
func withBg(s, hex string) string {
	esc, _, _ := strings.Cut(lipgloss.NewStyle().Background(lipgloss.Color(hex)).Render("x"), "x")
	if esc == "" {
		return s
	}
	s = strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+esc,
		"\x1b[m", "\x1b[m"+esc,
		"\x1b[49m", "\x1b[49m"+esc,
	).Replace(s)
	return esc + s + "\x1b[0m"
}

// authorName is the full author name; "?" when the commit has none.
func authorName(name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return "?"
	}
	return name
}
