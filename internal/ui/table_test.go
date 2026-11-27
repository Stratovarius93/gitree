package ui

import (
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeWidthsWide(t *testing.T) {
	w := ComputeWidths(160, 6, 0, 18)
	assert.True(t, w.ShowAuthor && w.ShowDate && w.ShowHash)
	assert.Equal(t, 24, w.Label)
	assert.Equal(t, 18, w.Author)
	assert.Equal(t, 5, w.Date)
	assert.Equal(t, 7, w.Hash)
	assert.Equal(t, 22, w.Changes)
	assert.Equal(t, 160-24-6-22-18-5-7-6*3, w.Message)
}

func TestComputeWidthsNarrow(t *testing.T) {
	w := ComputeWidths(90, 6, 0, 18)
	assert.False(t, w.ShowDate)
	assert.True(t, w.ShowAuthor && w.ShowHash)
	w = ComputeWidths(70, 6, 0, 18)
	assert.False(t, w.ShowDate || w.ShowAuthor)
	assert.True(t, w.ShowHash)
	w = ComputeWidths(40, 6, 0, 18)
	assert.False(t, w.ShowDate || w.ShowAuthor || w.ShowHash)
}

func TestComputeWidthsInvariants(t *testing.T) {
	for _, g := range []int{0, 1, 6, 30, 100, 260} {
		for _, lw := range []int{0, 24, 30, 45, 60} {
			for total := 1; total <= 200; total++ {
				w := ComputeWidths(total, g, lw, 18)
				assert.GreaterOrEqual(t, w.Message, 1, "total %d graph %d label %d", total, g, lw)
				for _, f := range []int{w.Label, w.Graph, w.Changes, w.Author, w.Date, w.Hash} {
					assert.GreaterOrEqual(t, f, 0, "total %d graph %d label %d", total, g, lw)
				}
				assert.LessOrEqual(t, w.Label, 45, "total %d graph %d label %d", total, g, lw)
				assert.Equal(t, total, w.sum(), "total %d graph %d label %d", total, g, lw)
			}
		}
	}
}

func TestRenderHeaderWidth(t *testing.T) {
	for _, total := range []int{1, 15, 40, 70, 90, 160} {
		w := ComputeWidths(total, 6, 0, 18)
		h := RenderHeader(w)
		lines := strings.Split(h, "\n")
		assert.Len(t, lines, 2)
		for _, l := range lines {
			assert.Equal(t, total, lipgloss.Width(l), "total %d", total)
		}
	}
}

// TestComputeWidthsWideGraph: a very wide graph never squeezes the label or
// the message; it is clipped instead.
func TestComputeWidthsWideGraph(t *testing.T) {
	w := ComputeWidths(200, 260, 0, 18)
	assert.GreaterOrEqual(t, w.Label, 16)
	assert.GreaterOrEqual(t, w.Graph, 40)
	assert.GreaterOrEqual(t, w.Message, 40)
	assert.Equal(t, 200, w.sum())
	for _, total := range []int{120, 160, 250} {
		w := ComputeWidths(total, 260, 0, 18)
		assert.Equal(t, 24, w.Label, "total %d", total)
		assert.GreaterOrEqual(t, w.Message, 20, "total %d", total)
		assert.Greater(t, w.Graph, 4, "total %d", total)
	}
}

// TestRenderHeaderNoEllipsis: column titles fit or are clipped, never "AUTH…".
func TestRenderHeaderNoEllipsis(t *testing.T) {
	for _, total := range []int{60, 90, 160} {
		for _, g := range []int{2, 4, 6, 20} {
			h := ansi.Strip(strings.Split(RenderHeader(ComputeWidths(total, g, 0, 18)), "\n")[0])
			assert.NotContains(t, h, "…", "total %d graph %d: %q", total, g, h)
		}
	}
	h := ansi.Strip(RenderHeader(ComputeWidths(160, 6, 0, 18)))
	assert.Contains(t, h, "AUTHOR", "18-cell author column fits the full title")
	assert.Contains(t, h, "GRAPH")
}

func TestComputeWidthsAdaptiveLabel(t *testing.T) {
	assert.Equal(t, 30, ComputeWidths(200, 6, 30, 18).Label)
	assert.Equal(t, 24, ComputeWidths(200, 6, 10, 18).Label, "min 24")
	assert.Equal(t, 45, ComputeWidths(200, 6, 80, 18).Label, "max 45")
	// A wide graph never squeezes the label; the message keeps its floor.
	w := ComputeWidths(200, 260, 45, 18)
	assert.Equal(t, 45, w.Label)
	assert.GreaterOrEqual(t, w.Message, 20)
	// Narrow terminals: at most ~35% for the label, hiding order unchanged.
	w = ComputeWidths(90, 6, 45, 18)
	assert.LessOrEqual(t, w.Label, 90*35/100)
	assert.GreaterOrEqual(t, w.Label, 24)
	assert.False(t, w.ShowDate)
	assert.True(t, w.ShowAuthor && w.ShowHash)
	w = ComputeWidths(70, 6, 45, 18)
	assert.Equal(t, 24, w.Label)
	assert.False(t, w.ShowDate || w.ShowAuthor)
	assert.True(t, w.ShowHash)
	// With a wide graph the message shrinks first; the graph keeps what it
	// would get with the default 24-wide label.
	base := ComputeWidths(200, 260, 24, 18)
	w = ComputeWidths(200, 260, 45, 18)
	assert.Equal(t, 45, w.Label)
	assert.Equal(t, 20, w.Message, "message cedes down to its floor first")
	// The graph only loses what the message could not give (21 cells wanted,
	// 20 available).
	assert.GreaterOrEqual(t, w.Graph, base.Graph-(45-24-(base.Message-20)))
	// When the message can absorb it all, the graph is untouched.
	base = ComputeWidths(220, 260, 24, 18)
	w = ComputeWidths(220, 260, 40, 18)
	assert.Equal(t, base.Graph, w.Graph)
	assert.Equal(t, 40, w.Label)
	// Narrow graph: the label still gets its width.
	w = ComputeWidths(200, 6, 45, 18)
	assert.Equal(t, 45, w.Label)
	assert.Equal(t, 6, w.Graph)
	// The message gives up space (down to 20) before the label shrinks.
	w = ComputeWidths(120, 6, 45, 18)
	assert.Equal(t, 42, w.Label)
	assert.GreaterOrEqual(t, w.Message, 20)
}

func TestNaturalLabelWidth(t *testing.T) {
	assert.Equal(t, 0, NaturalLabelWidth(nil))
	l := Label{Name: strings.Repeat("a", 30), Kind: LabelBranch, Local: true, Behind: 3}
	assert.Equal(t, lipgloss.Width(pillText(l)), NaturalLabelWidth([]Label{l}))
	two := NaturalLabelWidth([]Label{l, {Name: "x", Kind: LabelTag}})
	assert.Equal(t, lipgloss.Width(pillText(l))+len(" +1"), two)
	// PR chip + " +N" at natural width: both visible, nothing truncated.
	pl := Label{Name: strings.Repeat("b", 20), Kind: LabelBranch, Local: true,
		PR: &github.PR{Number: 123, State: github.PROpen}}
	ls := []Label{pl, {Name: "x", Kind: LabelTag}, {Name: "y", Kind: LabelTag}}
	out := ansi.Strip(RenderLabels(ls, "#83a598", NaturalLabelWidth(ls)))
	assert.Contains(t, out, "#123")
	assert.Contains(t, out, "+2")
	assert.NotContains(t, out, "…")
	// Rendering at the natural width never truncates.
	assert.NotContains(t, ansi.Strip(RenderLabels([]Label{l}, "#83a598", NaturalLabelWidth([]Label{l}))), "…")
}

func TestLongBranchNotTruncatedAt200(t *testing.T) {
	cs := sampleCommits()
	cs[1].Refs = []git.Ref{{Name: "ABC-1234-WISHLIST-ABCDEFGH12", Kind: git.LocalBranch}}
	track := map[string]git.Track{"ABC-1234-WISHLIST-ABCDEFGH12": {Behind: 3}}
	rows := BuildRows(cs, nil, track, nil, "")
	want := 0
	for _, r := range rows {
		want = max(want, NaturalLabelWidth(r.Labels))
	}
	require.Greater(t, want, 30)
	w := ComputeWidths(200, 6, want, 18)
	assert.LessOrEqual(t, w.Label, 45)
	out := ansi.Strip(RenderRow(rows[1], w, false, testNow))
	assert.Contains(t, out, "ABC-1234-WISHLIST-ABCDEFGH12 ↓3")
	labelCell, _, _ := strings.Cut(out, " ┊ ")
	assert.NotContains(t, labelCell, "…")
	assert.Equal(t, 200, lipgloss.Width(RenderRow(rows[1], w, false, testNow)))
}

func TestAuthorFullName(t *testing.T) {
	rows := BuildRows(sampleCommits(), nil, nil, nil, "")
	w := ComputeWidths(200, 6, 0, 18)
	out := ansi.Strip(RenderRow(rows[0], w, false, testNow))
	assert.Contains(t, out, "Ana Perez")
	assert.Equal(t, 200, lipgloss.Width(RenderRow(rows[0], w, false, testNow)))

	long := sampleCommits()
	long[0].AuthorName = "Maximiliano Bartolomeo Fernandez"
	rows = BuildRows(long, nil, nil, nil, "")
	out = ansi.Strip(RenderRow(rows[0], w, false, testNow))
	assert.Contains(t, out, "Maximiliano Barto…")
	assert.Equal(t, 200, lipgloss.Width(RenderRow(rows[0], w, true, testNow)))
}

func TestAuthorWidthAdapts(t *testing.T) {
	assert.Equal(t, 18, ComputeWidths(200, 6, 0, 18).Author)
	for _, total := range []int{80, 90} {
		w := ComputeWidths(total, 6, 0, 18)
		assert.True(t, w.ShowAuthor, "total %d", total)
		assert.GreaterOrEqual(t, w.Author, 6, "total %d", total)
		assert.GreaterOrEqual(t, w.Message, 20, "total %d", total)
		assert.Equal(t, total, w.sum())
	}
	assert.False(t, ComputeWidths(70, 6, 0, 18).ShowAuthor)
	assert.False(t, ComputeWidths(50, 6, 0, 18).ShowAuthor)
	for _, total := range []int{80, 90, 100, 160} {
		w := ComputeWidths(total, 6, 0, 18)
		if w.Author >= 6 {
			assert.Contains(t, ansi.Strip(RenderHeader(w)), "AUTHOR", "total %d", total)
		}
	}
}

func TestAuthorWidthFollowsVisibleNames(t *testing.T) {
	assert.Equal(t, 9, ComputeWidths(200, 6, 0, 9).Author)
	assert.Equal(t, 6, ComputeWidths(200, 6, 0, 3).Author, "AUTHOR stays readable")
	assert.Equal(t, 6, ComputeWidths(200, 6, 0, 0).Author)
	assert.Equal(t, 18, ComputeWidths(200, 6, 0, 40).Author)
	assert.Contains(t, ansi.Strip(RenderHeader(ComputeWidths(200, 6, 0, 3))), "AUTHOR")
}

// TestAuthorShrinksFirst: the author column gives up its cells before the
// message drops below 30 or the graph is clipped.
func TestAuthorShrinksFirst(t *testing.T) {
	w := ComputeWidths(100, 6, 0, 18)
	assert.GreaterOrEqual(t, w.Message, 30)
	assert.Less(t, w.Author, 18)
	for _, g := range []int{0, 6, 30, 60} {
		for _, aw := range []int{0, 6, 12, 18, 30} {
			for total := 1; total <= 220; total++ {
				w := ComputeWidths(total, g, 24, aw)
				assert.Equal(t, total, w.sum(), "total %d graph %d author %d", total, g, aw)
				assert.GreaterOrEqual(t, w.Message, 1)
				if !w.ShowAuthor {
					continue
				}
				assert.GreaterOrEqual(t, w.Author, authorMin, "total %d graph %d author %d", total, g, aw)
				assert.LessOrEqual(t, w.Author, max(min(aw, authorWant), authorMin))
				if w.Message < 30 || w.Graph < g {
					assert.Equal(t, authorMin, w.Author, "total %d graph %d author %d: author shrinks first", total, g, aw)
				}
			}
		}
	}
}

func TestComputeWidthsChanges(t *testing.T) {
	w := ComputeWidths(160, 6, 0, 18)
	assert.True(t, w.ShowChanges)
	assert.Equal(t, 22, w.Changes)
	assert.Equal(t, 160-24-6-22-18-5-7-6*3, w.Message)
	assert.Contains(t, ansi.Strip(RenderHeader(w)), "CHANGES")
	var keys []string
	for _, c := range w.cells() {
		keys = append(keys, c.key)
	}
	assert.Equal(t, []string{"label", "graph", "message", "changes", "author", "date", "hash"}, keys)
}

func TestComputeWidthsChangesHidesFirst(t *testing.T) {
	w := ComputeWidths(119, 6, 0, 18)
	assert.False(t, w.ShowChanges)
	assert.True(t, w.ShowDate)
	for total := 120; total <= 200; total++ {
		for _, g := range []int{6, 30, 60} {
			w := ComputeWidths(total, g, 0, 18)
			if w.ShowChanges {
				assert.GreaterOrEqual(t, w.Message, 30, "total %d graph %d", total, g)
				assert.Equal(t, g, w.Graph, "total %d graph %d", total, g)
			}
		}
	}
}
