package ui

import (
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGreenCells(t *testing.T) {
	cases := []struct{ a, d, want int }{
		{42, 18, 7}, {6, 0, 10}, {0, 125, 0}, {310, 47, 9}, {1, 1, 5},
		{300, 2, 9}, // a deleted line still gets a red cell
		{2, 300, 1}, // an added line still gets a green cell
		{0, 0, 0},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, greenCells(c.a, c.d), "%+v", c)
	}
}

func TestFmtCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 9999: "9999", 10000: "10k", 125000: "125k", 999999: "999k", 1000000: "1M", 25000000: "25M"} {
		assert.Equal(t, want, fmtCount(n), "%d", n)
	}
}

func TestChangesCell(t *testing.T) {
	s := changesCell(&git.Stat{Added: 42, Deleted: 18}, lipgloss.NewStyle())
	assert.Equal(t, 22, lipgloss.Width(s))
	assert.Equal(t, "▬▬▬▬▬▬▬▬▬▬ +42   -18  ", ansi.Strip(s))
	cells := cellsOf(s)
	for i := 0; i < 7; i++ {
		assert.Equal(t, fgOf(colors.Green), cells[i].fg, "bar cell %d", i)
	}
	for i := 7; i < 10; i++ {
		assert.Equal(t, fgOf(colors.Red), cells[i].fg, "bar cell %d", i)
	}
	assert.Equal(t, fgOf(colors.Green), cells[11].fg, "+42")
	assert.Equal(t, fgOf(colors.Red), cells[17].fg, "-18")
}

func TestChangesCellZero(t *testing.T) {
	s := changesCell(&git.Stat{Added: 6}, lipgloss.NewStyle())
	assert.Equal(t, "▬▬▬▬▬▬▬▬▬▬ +6    -0   ", ansi.Strip(s))
	assert.Equal(t, fgOf(colors.Gray), cellsOf(s)[17].fg, "-0 is gray")

	s = changesCell(&git.Stat{}, lipgloss.NewStyle())
	assert.Equal(t, 22, lipgloss.Width(s))
	assert.Equal(t, "▬▬▬▬▬▬▬▬▬▬ +0    -0   ", ansi.Strip(s))
	for i, c := range cellsOf(s) {
		if c.ch != " " {
			assert.Equal(t, fgOf(colors.Gray), c.fg, "cell %d", i)
		}
	}
}

func TestChangesCellLoading(t *testing.T) {
	s := changesCell(nil, lipgloss.NewStyle())
	assert.Equal(t, 22, lipgloss.Width(s))
	assert.Equal(t, strings.Repeat("▬", 10)+strings.Repeat(" ", 12), ansi.Strip(s))
	assert.Equal(t, fgOf(colors.Separator), cellsOf(s)[0].fg)
}

func TestChangesCellLargeCounts(t *testing.T) {
	s := changesCell(&git.Stat{Added: 125000, Deleted: 3}, lipgloss.NewStyle())
	assert.Equal(t, 22, lipgloss.Width(s))
	assert.Equal(t, "▬▬▬▬▬▬▬▬▬▬ +125k -3   ", ansi.Strip(s))
}

func TestRenderRowChangesSelected(t *testing.T) {
	r := tintRows()[1]
	r.Stat = &git.Stat{Added: 42, Deleted: 18}
	w := ComputeWidths(160, 6, 0, 18)
	require.True(t, w.ShowChanges)
	out := RenderRow(r, w, true, testNow)
	assert.Equal(t, w.sum(), lipgloss.Width(out))
	plain := ansi.Strip(out)
	assert.Less(t, strings.Index(plain, "login form"), strings.Index(plain, "+42"))
	assert.Less(t, strings.Index(plain, "+42"), strings.Index(plain, "Sam Rivera"))
	start := w.Label + w.Graph + w.Message + 3*sepWidth
	cells := cellsOf(out)
	for i := start; i < start+w.Changes; i++ {
		assert.Equal(t, cellsOf(lipgloss.NewStyle().Background(lipgloss.Color(colors.SelBg)).Render("x"))[0].bg, cells[i].bg, "cell %d", i)
	}
}

func TestRenderRowWIPChangesBlank(t *testing.T) {
	rows := BuildRows(sampleCommits(), []git.FileChange{{Path: "a"}}, nil, nil, "")
	require.Equal(t, RowWIP, rows[0].Kind)
	w := ComputeWidths(160, 6, 0, 18)
	out := RenderRow(rows[0], w, false, testNow)
	assert.Equal(t, w.sum(), lipgloss.Width(out))
	assert.NotContains(t, ansi.Strip(out), "▬")
}
