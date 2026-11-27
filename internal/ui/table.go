package ui

import (
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const sepWidth = 3 // " ┊ "

// Widths holds the cell widths of the table. Columns with width 0 (or hidden)
// are omitted together with their separator.
type Widths struct {
	Label, Graph, Message, Changes, Author, Date, Hash int
	ShowChanges, ShowAuthor, ShowDate, ShowHash        bool
}

// Visibility thresholds on the total width: Changes hides first, then Date,
// then Author, then Hash.
const (
	minTotalChanges = 120
	minTotalDate    = 100
	minTotalAuthor  = 80
	minTotalHash    = 50
)

// ComputeWidths lays out the table in total cells for a graph that needs
// graphWidth cells, with a label that wants labelWant cells.
//
// The BRANCH/TAG column is adaptive: it grows to fit the widest branch name,
// clamped to 24..45 cells and to at most 35% of total (never below 24). The
// extra width comes from the message first, which may drop to 20 cells; the
// graph keeps the width it gets with the default 24-wide label and 40-cell
// message, and only gives up the cells the message cannot spare (its floor is
// 20). If even that does not fit, the label, message and graph shrink in
// tiers (down to omission) and graph lines are clipped on the right. Message
// is always at least 1.
//
// The AUTHOR column wants the widest visible author name (authorNames),
// clamped to authorMin..authorWant (6..18: "AUTHOR" stays readable). It gives
// up its cells first: it shrinks toward authorMin before the message drops
// below 30 cells or the graph is clipped, and is then hidden by the usual
// order.
//
// The CHANGES column (changesWidth cells) is shown from minTotalChanges on,
// and only if it takes its cells from the message alone, leaving it at least
// 30: it is dropped before the label, graph or author give up anything.
func ComputeWidths(total, graphWidth, labelWant, authorNames int) Widths {
	without := computeWidths(total, graphWidth, labelWant, authorNames, false)
	w := computeWidths(total, graphWidth, labelWant, authorNames, true)
	if w.ShowChanges && w.Message >= 30 && w.Graph == without.Graph &&
		w.Label == without.Label && w.Author == without.Author {
		return w
	}
	return without
}

func computeWidths(total, graphWidth, labelWant, authorNames int, changes bool) Widths {
	want := min(max(authorNames, authorMin), authorWant)
	var w Widths
	for a := want; a >= authorMin; a-- {
		w = layout(total, graphWidth, labelWant, a, changes)
		if !w.ShowAuthor || (w.Label >= 24 && w.Message >= 30 && w.Graph >= graphWidth) {
			return w
		}
	}
	return w
}

// changesWidth: a 10-cell bar, a space and two 5-cell counts ("+NNNN -NNNN").
const changesWidth = 22

// Author column widths: preferred maximum and minimum (the header's width).
const (
	authorWant = 18
	authorMin  = 6
)

func layout(total, graphWidth, labelWant, authorW int, changes bool) Widths {
	if total < 1 {
		total = 1
	}
	if graphWidth < 0 {
		graphWidth = 0
	}
	w := Widths{
		ShowChanges: changes && total >= minTotalChanges,
		ShowDate:    total >= minTotalDate,
		ShowAuthor:  total >= minTotalAuthor,
		ShowHash:    total >= minTotalHash,
	}
	if w.ShowChanges {
		w.Changes = changesWidth
	}
	if w.ShowDate {
		w.Date = 5
	}
	if w.ShowAuthor {
		w.Author = authorW
	}
	if w.ShowHash {
		w.Hash = 7
	}
	g := graphWidth
	// The label grows to fit the widest branch name (24..45, and at most ~35%
	// of the total); the message gives up space first, down to 20.
	want := min(max(labelWant, 24), 45)
	if want > 24 {
		want = max(min(want, total*35/100), 24)
	}
	if want > 24 {
		// Graph as allocated with the default label (24) and message (40).
		w.Label, w.Graph, w.Message = 24, 1, 1
		room := total - (w.sum() - 2) - 40
		if g0 := max(min(g, room), 0); g0 >= min(g, 16) {
			w.Label, w.Graph, w.Message = want, g0, 0
			w.Message = total - w.sum()
			if w.Message >= 20 {
				return w
			}
		}
	}
	firstMsg := 20 // a grown label takes from the message first
	if want == 24 {
		firstMsg = 40
	}
	tiers := []struct{ label, minMsg, minGraph int }{
		{want, firstMsg, min(g, 16)},
		{want, 20, min(g, 16)},
		{24, 40, min(g, 16)},
		{16, 40, min(g, 16)},
		{16, 20, min(g, 8)},
		{12, 10, min(g, 4)},
		{8, 10, min(g, 2)},
	}
	for _, t := range tiers {
		w.Label = t.label
		// Room for the graph with its separator, keeping minMsg for the message.
		w.Graph, w.Message = 1, 1
		room := total - (w.sum() - 2) - t.minMsg
		w.Graph = max(min(g, room), 0)
		if w.Graph < t.minGraph {
			continue
		}
		w.Message = 0
		w.Message = total - w.sum()
		if w.Message >= t.minMsg {
			return w
		}
	}
	w.Label, w.Graph, w.Message = 0, 0, 0
	w.Message = total - w.sum()
	return w
}

// columns counts the visible columns (Message always counts).
func (w Widths) columns() int {
	n := 1
	for _, c := range []int{w.Label, w.Graph, w.Changes, w.Author, w.Date, w.Hash} {
		if c > 0 {
			n++
		}
	}
	return n
}

// sum is the total rendered width: cells plus separators.
func (w Widths) sum() int {
	return w.Label + w.Graph + w.Message + w.Changes + w.Author + w.Date + w.Hash + sepWidth*(w.columns()-1)
}

// cells returns the visible column widths in order, with a matching key.
func (w Widths) cells() []cell {
	all := []cell{
		{"label", w.Label}, {"graph", w.Graph}, {"message", w.Message},
		{"changes", w.Changes}, {"author", w.Author}, {"date", w.Date}, {"hash", w.Hash},
	}
	var out []cell
	for _, c := range all {
		if c.width > 0 {
			out = append(out, c)
		}
	}
	return out
}

type cell struct {
	key   string
	width int
}

func sepStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Separator))
}

// RenderHeader renders the title line and the rule below it, each exactly as wide as the table.
func RenderHeader(w Widths) string {
	titles := map[string]string{
		"label": " BRANCH / TAG", "graph": " GRAPH", "message": " MESSAGE",
		"changes": "CHANGES", "author": "AUTHOR", "date": "DATE", "hash": "HASH",
	}
	// Titles that do not fit are clipped, without "…" (which reads as data).
	head := lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray))
	sep := sepStyle().Render(" ┊ ")
	rule := sepStyle().Render("─┼─")
	var titleParts, ruleParts []string
	for _, c := range w.cells() {
		titleParts = append(titleParts, head.Render(padTo(ansi.Truncate(titles[c.key], c.width, ""), c.width)))
		ruleParts = append(ruleParts, sepStyle().Render(strings.Repeat("─", c.width)))
	}
	return strings.Join(titleParts, sep) + "\n" + strings.Join(ruleParts, rule)
}
