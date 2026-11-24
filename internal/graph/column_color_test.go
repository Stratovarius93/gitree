package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type markerSprinter struct{ tag string }

func (m markerSprinter) Sprint(a ...any) string {
	var sb strings.Builder
	for _, v := range a {
		sb.WriteString(v.(string))
	}
	return "<" + m.tag + ">" + sb.String() + "</" + m.tag + ">"
}

func (m markerSprinter) Sprintf(format string, a ...any) string { return m.Sprint(format) }

// markerColors returns a goroutine-safe ColorFunc: styles are created up
// front, because RenderAux calls colorFor from several goroutines.
func markerColors() ColorFunc {
	const maxCols = 32
	styles := make([]*Style, maxCols)
	for i := range styles {
		styles[i] = &Style{Style: markerSprinter{fmt.Sprintf("c%d", i)}}
	}
	return func(col int) *Style { return styles[col] }
}

func mergeCommits() []*Commit {
	return NewCommits([]CommitOpts{
		{Hash: "1", Parents: []string{"2"}},
		{Hash: "2", Parents: []string{"3", "4"}},
		{Hash: "4", Parents: []string{"3"}},
		{Hash: "3"},
	})
}

func TestPipeColorIsRightmostColumn(t *testing.T) {
	colorFor := markerColors()
	sets := GetPipeSets(mergeCommits(), colorFor)
	assert.Len(t, sets, 4)
	for row, set := range sets {
		for _, pipe := range set {
			assert.Same(t, colorFor(int(max(pipe.fromPos, pipe.toPos))), pipe.style,
				"row %d pipe %d->%d", row, pipe.fromPos, pipe.toPos)
		}
	}
	// the fork in row 1 leaves column 0 for column 1 and so takes column 1's color
	var sawFork bool
	for _, pipe := range sets[1] {
		if pipe.fromPos == 0 && pipe.toPos == 1 {
			sawFork = true
			assert.Same(t, colorFor(1), pipe.style)
		}
	}
	assert.True(t, sawFork)
}

// Asserts the behaviour (the dot is styled with its own column), not the
// renderPipeSet override specifically.
func TestCommitDotUsesOwnColumn(t *testing.T) {
	colorFor := markerColors()
	commits := mergeCommits()
	lines := RenderCommitGraph(commits, nil, colorFor, BoxDrawingSymbols)
	assert.Contains(t, lines[1], "<c0>◎</c0>")
	assert.Contains(t, lines[1], "<c1>")
	assert.Equal(t, 0, CommitColumn(GetPipeSets(commits, colorFor)[1]))
}

func TestCommitColumn(t *testing.T) {
	colorFor := markerColors()
	sets := GetPipeSets(mergeCommits(), colorFor)
	assert.Equal(t, []int{0, 0, 1, 0}, []int{
		CommitColumn(sets[0]), CommitColumn(sets[1]), CommitColumn(sets[2]), CommitColumn(sets[3]),
	})
}
