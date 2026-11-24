package graph

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestRenderRangeMatchesRenderAux(t *testing.T) {
	commits := NewCommits([]CommitOpts{
		{Hash: "1", Parents: []string{"2"}},
		{Hash: "2", Parents: []string{"3", "6"}},
		{Hash: "3", Parents: []string{"4"}},
		{Hash: "6", Parents: []string{"4"}},
		{Hash: "4", Parents: []string{"5", "7"}},
		{Hash: "7", Parents: []string{"5"}},
		{Hash: "5", Parents: []string{"8"}},
		{Hash: "8", Parents: []string{}},
	})
	pipeSets := GetPipeSets(commits, testColor)
	for _, sel := range []*string{nil, commits[1].HashPtr(), commits[4].HashPtr()} {
		all := RenderAux(pipeSets, commits, sel, BoxDrawingSymbols, testColor)
		for from := 0; from <= len(commits); from++ {
			for to := from; to <= len(commits); to++ {
				assert.Equal(t, all[from:to], append([]string{}, RenderRange(pipeSets, commits, sel, BoxDrawingSymbols, testColor, from, to)...))
			}
		}
	}
	assert.Nil(t, RenderRange(pipeSets, commits, nil, BoxDrawingSymbols, testColor, -5, 0))
	assert.Equal(t, RenderAux(pipeSets, commits, nil, BoxDrawingSymbols, testColor)[6:], RenderRange(pipeSets, commits, nil, BoxDrawingSymbols, testColor, 6, 99))
}

func testColor(int) *Style { return &FgDefault }

func TestPipeSetWidthMatchesRenderedLine(t *testing.T) {
	pool := &StringPool{}
	commits := generateCommits(pool, 300)
	pipeSets := GetPipeSets(commits, testColor)
	for _, sel := range []*string{nil, commits[7].HashPtr()} {
		lines := RenderAux(pipeSets, commits, sel, BoxDrawingSymbols, testColor)
		for i, l := range lines {
			assert.Equal(t, utf8.RuneCountInString(decolorise(l)), PipeSetWidth(pipeSets[i]), "row %d", i)
		}
	}
}
