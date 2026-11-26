// Package colors holds the Gruvbox palette and the per-column graph colors.
package colors

import (
	"fmt"
	"math"
	"strconv"
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
	SelBg     = "#504945" // selected row: stands out over the lane tint
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

// Background is the terminal background the theme assumes (Gruvbox bg0).
const Background = "#282828"

// Mix blends fg into bg with weight a (0: bg, 1: fg), per RGB channel,
// rounding to the nearest value. Malformed colors yield bg.
func Mix(fg, bg string, a float64) string {
	f, okF := parseHex(fg)
	b, okB := parseHex(bg)
	if !okF || !okB {
		return bg
	}
	a = min(max(a, 0), 1)
	out := "#"
	for i := range 3 {
		v := float64(f[i])*a + float64(b[i])*(1-a)
		out += fmt.Sprintf("%02x", int(math.Round(v)))
	}
	return out
}

func parseHex(s string) ([3]uint8, bool) {
	var c [3]uint8
	if len(s) != 7 || s[0] != '#' {
		return c, false
	}
	for i := range 3 {
		v, err := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return c, false
		}
		c[i] = uint8(v)
	}
	return c, true
}
