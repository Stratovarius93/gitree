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
	free := width - used
	if free <= 0 {
		return s
	}
	// Pills and PR chips end in their own padding; the gray " +N" does not,
	// so the line keeps a space away from it.
	if plain := ansi.Strip(s); plain != "" && !strings.HasSuffix(plain, " ") {
		s += " "
		free--
	}
	return s + lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(strings.Repeat("━", free))
}

// connectGraph continues the connector through the graph cells left of the
// node: walking left from the node, empty cells become ━ in the lane color and
// a crossing lane's │ becomes ┿ in that lane's own color, so neither line is
// cut. It stops at the first other glyph (an edge, a bend, a merge): drawing
// over those would recolor other lanes' lines or hide the node's own edge.
func connectGraph(g string, node int, hex string) string {
	line := lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("━")
	segs := make([]string, node)
	for i := range segs {
		segs[i] = ansi.Cut(g, i, i+1)
	}
	for i := node - 1; i >= 0; i-- {
		switch ansi.Strip(segs[i]) {
		case " ", "":
			segs[i] = line
		case "│":
			segs[i] = strings.Replace(segs[i], "│", "┿", 1)
		default:
			i = -1 // stop
		}
	}
	return strings.Join(segs, "") + ansi.Cut(g, node, ansi.StringWidth(g))
}
