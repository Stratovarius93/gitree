package ui

import (
	"strconv"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/lipgloss"
)

// The CHANGES cell: a fixed-length bar split green/red by the share of added
// and deleted lines, then the two counts in 5-cell slots.
const (
	barCells  = 10
	barGlyph  = "▬"
	countSlot = 5
)

// greenCells is how many of the bar's cells are green: the added share,
// rounded half up, but a side with any lines always keeps at least one cell.
func greenCells(added, deleted int) int {
	t := added + deleted
	if t == 0 {
		return 0
	}
	n := (barCells*added + t/2) / t
	if added > 0 && n == 0 {
		n = 1
	}
	if deleted > 0 && n == barCells {
		n = barCells - 1
	}
	return n
}

// fmtCount fits a line count in 4 characters: 9999, then 10k…999k, then 1M….
func fmtCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.Itoa(n/1_000_000) + "M"
	case n >= 10_000:
		return strconv.Itoa(n/1_000) + "k"
	}
	return strconv.Itoa(n)
}

// changesCell renders st in exactly changesWidth cells; nil (not loaded yet)
// is a dim bar without counts. Every segment is drawn with base so a
// selection background carries through.
func changesCell(st *git.Stat, base lipgloss.Style) string {
	paint := func(s, hex string) string { return base.Foreground(lipgloss.Color(hex)).Render(s) }
	bar := func(n int, hex string) string { return paint(strings.Repeat(barGlyph, n), hex) }
	if st == nil {
		return bar(barCells, colors.Separator) + base.Render(strings.Repeat(" ", changesWidth-barCells))
	}
	count := func(sign string, n int, hex string) string {
		if n == 0 {
			hex = colors.Gray
		}
		s := sign + fmtCount(n)
		return paint(s+strings.Repeat(" ", max(countSlot-len(s), 0)), hex)
	}
	var b strings.Builder
	if st.Added+st.Deleted == 0 {
		b.WriteString(bar(barCells, colors.Gray))
	} else {
		g := greenCells(st.Added, st.Deleted)
		if g > 0 {
			b.WriteString(bar(g, colors.Green))
		}
		if g < barCells {
			b.WriteString(bar(barCells-g, colors.Red))
		}
	}
	b.WriteString(base.Render(" "))
	b.WriteString(count("+", st.Added, colors.Green))
	b.WriteString(base.Render(" "))
	b.WriteString(count("-", st.Deleted, colors.Red))
	return b.String()
}
