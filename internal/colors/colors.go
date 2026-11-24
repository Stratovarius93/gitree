// Package colors holds the Gruvbox palette and the per-column graph colors.
package colors

import (
	"sync"

	"github.com/Stratovarius93/gitree/internal/graph"
)

const (
	Red       = "#ea6962"
	Orange    = "#e78a4e"
	Yellow    = "#d8a657"
	Green     = "#a9b665"
	Aqua      = "#89b482"
	Blue      = "#7daea3"
	Purple    = "#d3869b"
	Fg        = "#d4be98"
	Gray      = "#928374"
	DimBg     = "#3c3836"
	Separator = "#504945"
	DarkText  = "#282828"
)

// Floating-window colors, from the Gruvbox Flutter nvim theme
// (NormalFloat, FloatBorder, Visual and PmenuMatch).
const (
	Bg3          = "#3c3836" // float background
	Bg5          = "#504945" // float border and selected row
	FgFloat      = "#ebdbb2" // float text and title
	BrightOrange = "#fe8019" // filter matches
)

// Palette is the lane color cycle; column n uses Palette[n % len(Palette)].
var Palette = []string{"#7daea3", "#a9b665", "#d3869b", "#d8a657", "#89b482", "#e78a4e", "#ea6962"}

var (
	mu     sync.Mutex
	styles = make([]*graph.Style, len(Palette))
)

// HexForColumn returns the hex color of a graph column.
func HexForColumn(col int) string { return Palette[col%len(Palette)] }

// ForColumn returns the style of a graph column. Each palette slot is built
// once and reused, because the graph's render cache is keyed by style pointer.
func ForColumn(col int) *graph.Style {
	i := col % len(Palette)
	mu.Lock()
	defer mu.Unlock()
	if styles[i] == nil {
		styles[i] = graph.NewStyle(Palette[i])
	}
	return styles[i]
}
