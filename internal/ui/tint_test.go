package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// styledCell is one printed cell with the colors in effect ("" = default).
type styledCell struct {
	ch     string
	fg, bg string
}

// cellsOf decodes the SGR sequences lipgloss emits (resets, 24-bit fg/bg)
// into per-cell colors.
func cellsOf(s string) []styledCell {
	var out []styledCell
	var fg, bg string
	for len(s) > 0 {
		if strings.HasPrefix(s, "\x1b[") {
			end := strings.IndexByte(s, 'm')
			params := strings.Split(s[2:end], ";")
			s = s[end+1:]
			for i := 0; i < len(params); i++ {
				switch params[i] {
				case "", "0":
					fg, bg = "", ""
				case "39":
					fg = ""
				case "49":
					bg = ""
				case "38", "48":
					if i+4 < len(params) && params[i+1] == "2" {
						hex := "#"
						for _, p := range params[i+2 : i+5] {
							v, _ := strconv.Atoi(p)
							hex += fmt.Sprintf("%02x", v)
						}
						if params[i] == "38" {
							fg = hex
						} else {
							bg = hex
						}
						i += 4
					}
				}
			}
			continue
		}
		r := []rune(s)[0]
		out = append(out, styledCell{ch: string(r), fg: fg, bg: bg})
		s = s[len(string(r)):]
	}
	return out
}

// tintRows: main (HEAD) on lane 0 merges feature/login on lane 1.
func tintRows() []Row {
	return BuildRows(sampleCommits(), nil, nil, nil, "")
}

func graphStart(w Widths) int { return w.Label + sepWidth }

// tintOf is the tint as the terminal receives it (lipgloss may round a
// channel by one).
func tintOf(lane int, a float64) string {
	hex := colors.Mix(colors.HexForColumn(lane), colors.Background, a)
	return cellsOf(lipgloss.NewStyle().Background(lipgloss.Color(hex)).Render("x"))[0].bg
}

func fgOf(hex string) string {
	return cellsOf(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("x"))[0].fg
}

const nodeGlyphs = "●○◌"

func TestRowTintFromNode(t *testing.T) {
	rows := tintRows()
	w := ComputeWidths(160, 6, 0, 18)
	r := rows[1] // feature/login, lane 1
	require.Equal(t, 1, r.Lane)
	cells := cellsOf(RenderRow(r, w, false, testNow))
	tint := tintOf(1, 0.20)
	start, node := graphStart(w), graphStart(w)+2*r.Lane
	for i := start; i < node; i++ {
		assert.NotEqual(t, tint, cells[i].bg, "cell %d left of the node", i)
	}
	assert.Contains(t, nodeGlyphs, cells[node].ch)
	for i := node; i < start+w.Graph; i++ {
		assert.Equal(t, tint, cells[i].bg, "cell %d from the node on", i)
	}
}

func TestRowBarBeforeMessage(t *testing.T) {
	rows := tintRows()
	w := ComputeWidths(160, 6, 0, 18)
	r := rows[1]
	cells := cellsOf(RenderRow(r, w, false, testNow))
	tint := tintOf(1, 0.20)
	sep := graphStart(w) + w.Graph
	assert.Equal(t, " ", cells[sep].ch)
	assert.Equal(t, tint, cells[sep].bg, "the tint reaches the bar")
	assert.Equal(t, "▌", cells[sep+1].ch)
	assert.Equal(t, fgOf(colors.HexForColumn(1)), cells[sep+1].fg)
	assert.Equal(t, " ", cells[sep+2].ch)
	assert.Equal(t, "", cells[sep+2].bg)
}

func TestSelectedRowTintStronger(t *testing.T) {
	rows := tintRows()
	w := ComputeWidths(160, 6, 0, 18)
	r := rows[1]
	cells := cellsOf(RenderRow(r, w, true, testNow))
	tint := tintOf(1, 0.30)
	node := graphStart(w) + 2*r.Lane
	assert.Equal(t, tint, cells[node].bg)
	msg := graphStart(w) + w.Graph + sepWidth
	assert.Equal(t, fgOf(colors.DimBg), cells[msg].bg, "the rest of the row keeps the highlight")
}

func TestWIPRowTinted(t *testing.T) {
	rows := BuildRows(sampleCommits(), []git.FileChange{{Path: "a"}}, nil, nil, "")
	w := ComputeWidths(160, 6, 0, 18)
	cells := cellsOf(RenderRow(rows[0], w, false, testNow))
	node := graphStart(w) + 2*rows[0].Lane
	assert.Equal(t, "◌", cells[node].ch)
	assert.Equal(t, tintOf(rows[0].Lane, 0.20), cells[node].bg)
}

func TestNodeBeyondClippedGraphHasNoTint(t *testing.T) {
	r := tintRows()[1]
	w := ComputeWidths(160, 6, 0, 18)
	w.Graph = 2 // clips lane 1's node (cell 2)
	out := RenderRow(r, w, false, testNow)
	assert.NotContains(t, out, "▌")
	assert.Equal(t, w.sum(), len(cellsOf(out)))
}

func TestHeadConnector(t *testing.T) {
	r := tintRows()[0] // main, IsHead, lane 0
	require.True(t, r.Labels[0].IsHead)
	w := ComputeWidths(160, 6, 0, 18)
	cells := cellsOf(RenderRow(r, w, false, testNow))
	hex := fgOf(colors.HexForColumn(r.Lane))
	label := ""
	for _, c := range cells[:w.Label] {
		label += c.ch
	}
	assert.True(t, strings.HasSuffix(label, "━━━"), "label column ends with the connector: %q", label)
	assert.Contains(t, label, "main")
	for i := w.Label; i < w.Label+sepWidth; i++ {
		assert.Equal(t, "━", cells[i].ch, "separator cell %d", i)
		assert.Equal(t, hex, cells[i].fg)
	}
}

func TestConnectorCrossesLanes(t *testing.T) {
	// HEAD on lane 1: the connector crosses lane 0's line.
	commits := []git.Commit{
		{Hash: "a1", Parents: []string{"c3"}, Subject: "on main", Refs: []git.Ref{{Name: "main", Kind: git.LocalBranch}}},
		{Hash: "b2", Parents: []string{"c3"}, Subject: "on topic", Refs: []git.Ref{{Name: "topic", Kind: git.LocalBranch, IsHead: true}}},
		{Hash: "c3", Subject: "root"},
	}
	rows := BuildRows(commits, nil, nil, nil, "")
	r := rows[1]
	require.Equal(t, 1, r.Lane)
	w := ComputeWidths(160, 6, 0, 18)
	cells := cellsOf(RenderRow(r, w, false, testNow))
	start := graphStart(w)
	assert.Equal(t, "┿", cells[start].ch, "lane 0's line is crossed, not cut")
	assert.Equal(t, fgOf(colors.HexForColumn(0)), cells[start].fg)
	assert.Equal(t, "━", cells[start+1].ch)
	assert.Contains(t, nodeGlyphs, cells[start+2].ch)
}

func TestNoConnectorWithoutHead(t *testing.T) {
	r := tintRows()[1]
	w := ComputeWidths(160, 6, 0, 18)
	for _, c := range cellsOf(RenderRow(r, w, false, testNow)) {
		assert.NotEqual(t, "━", c.ch)
	}
}

func TestConnectorWithFullLabel(t *testing.T) {
	r := tintRows()[0]
	r.Labels[0].Name = strings.Repeat("x", 60)
	w := ComputeWidths(160, 6, 0, 18)
	cells := cellsOf(RenderRow(r, w, false, testNow))
	label := ""
	for _, c := range cells[:w.Label] {
		label += c.ch
	}
	assert.NotContains(t, label, "━", "a pill that fills the column leaves no room")
	assert.Equal(t, "━", cells[w.Label].ch, "the separator still connects")
}

func graphText(g string, n int) string {
	s := ""
	for _, c := range cellsOf(g)[:n] {
		s += c.ch
	}
	return s
}

func TestConnectGraphStopsAtOtherEdges(t *testing.T) {
	plain := func(s string) string { return s } // graph lines as the renderer's plain glyphs
	cases := map[string]string{
		"│ │─│─○": "│ │─│─○", // the node's own edge starts right left of it: nothing to draw over
		"│ ╭─╯ ○": "│ ╭─╯━○", // stops at another lane's bend
		"╭─┴─○":   "╭─┴─○",
		"│ │ ○":   "┿━┿━○", // only spaces and crossings: drawn all the way
	}
	for in, want := range cases {
		got := connectGraph(plain(in), len([]rune(in))-1, "#7daea3")
		assert.Equal(t, want, graphText(got, len([]rune(in))), "for %q", in)
	}
}

func TestConnectorNotDrawnToClippedNode(t *testing.T) {
	commits := []git.Commit{
		{Hash: "a1", Parents: []string{"c3"}, Subject: "on main", Refs: []git.Ref{{Name: "main", Kind: git.LocalBranch}}},
		{Hash: "b2", Parents: []string{"c3"}, Subject: "on topic", Refs: []git.Ref{{Name: "topic", Kind: git.LocalBranch, IsHead: true}}},
		{Hash: "c3", Subject: "root"},
	}
	r := BuildRows(commits, nil, nil, nil, "")[1]
	w := ComputeWidths(160, 6, 0, 18)
	w.Graph = 2 // the node (cell 2) is clipped
	cells := cellsOf(RenderRow(r, w, false, testNow))
	assert.Equal(t, w.sum(), len(cells))
	for i := w.Label; i < w.Label+sepWidth+w.Graph; i++ {
		assert.NotEqual(t, "━", cells[i].ch, "cell %d: no connector toward a node off screen", i)
	}
}

func TestConnectorLeavesSpaceAfterSuffix(t *testing.T) {
	r := tintRows()[0]
	r.Labels = append(r.Labels, Label{Name: "v1", Kind: LabelTag})
	w := ComputeWidths(160, 6, 0, 18)
	label := graphText(RenderRow(r, w, false, testNow), w.Label)
	assert.Contains(t, label, "+1 ━")
}

func TestMixClamps(t *testing.T) {
	assert.Equal(t, "#ffffff", colors.Mix("#ffffff", "#000000", 2))
	assert.Equal(t, "#000000", colors.Mix("#ffffff", "#000000", -1))
}
