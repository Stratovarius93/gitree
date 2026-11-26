package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tintAlpha is how much of the lane color goes into the tint.
func tintAlpha(selected bool) float64 {
	if selected {
		return 0.30
	}
	return 0.20
}

// tintGraph gives cells [node, width) of the graph line g a tint background.
func tintGraph(g string, node, width int, tint string) string {
	return ansi.Cut(g, 0, node) + withBg(ansi.Cut(g, node, width), tint)
}

// laneBar is the graph→message separator of a tinted row: the tint, a strong
// lane bar, then the row's own background.
func laneBar(base lipgloss.Style, hex, tint string) string {
	return lipgloss.NewStyle().Background(lipgloss.Color(tint)).Render(" ") +
		base.Foreground(lipgloss.Color(hex)).Render("▌") + base.Render(" ")
}

// connectLabel renders the labels of the current branch's row with the free
// space after them filled by a connector toward the graph.
func connectLabel(labels []Label, hex string, width int) string {
	s, used := renderLabelContent(labels, hex, width)
	if free := width - used; free > 0 {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(strings.Repeat("━", free))
	}
	return s
}

// connectGraph continues the connector through graph cells [0, node): empty
// cells and horizontal lines become ━ in the lane color, a crossing lane's │
// becomes ┿ in that lane's own color, so neither line is cut.
func connectGraph(g string, node int, hex string) string {
	line := lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("━")
	var b strings.Builder
	for i := 0; i < node; i++ {
		seg := ansi.Cut(g, i, i+1)
		switch ansi.Strip(seg) {
		case " ", "─", "":
			b.WriteString(line)
		case "│":
			b.WriteString(strings.Replace(seg, "│", "┿", 1))
		default:
			b.WriteString(seg)
		}
	}
	return b.String() + ansi.Cut(g, node, ansi.StringWidth(g))
}
