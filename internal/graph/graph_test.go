// Adapted from github.com/jesseduffield/lazygit pkg/gui/presentation/graph (MIT). See LICENSE in this directory.

package graph

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/xo/terminfo"
)

var ansiRe = regexp.MustCompile(`\x1B\[([0-9]{1,3}(;[0-9]{1,3})*)?[mGK]`)

func decolorise(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestRenderCommitGraph(t *testing.T) {
	tests := []struct {
		name           string
		commitOpts     []CommitOpts
		expectedOutput string
	}{
		{
			name: "with some merges",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3"}},
				{Hash: "3", Parents: []string{"4"}},
				{Hash: "4", Parents: []string{"5", "7"}},
				{Hash: "7", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"8"}},
				{Hash: "8", Parents: []string{"9"}},
				{Hash: "9", Parents: []string{"A", "B"}},
				{Hash: "B", Parents: []string{"D"}},
				{Hash: "D", Parents: []string{"D"}},
				{Hash: "A", Parents: []string{"E"}},
				{Hash: "E", Parents: []string{"F"}},
				{Hash: "F", Parents: []string{"D"}},
				{Hash: "D", Parents: []string{"G"}},
			},
			expectedOutput: `
			1 ○
			2 ○
			3 ○
			4 ◎─╮
			7 │ ○
			5 ○─╯
			8 ○
			9 ◎─╮
			B │ ○
			D │ ○
			A ○ │
			E ○ │
			F ○ │
			D ○─╯`,
		},
		{
			name: "with a path that has room to move to the left",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "4", Parents: []string{"3", "5"}},
				{Hash: "3", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"6"}},
				{Hash: "6", Parents: []string{"7"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			4 │ ◎─╮
			3 ○─╯ │
			5 ○───╯
			6 ○`,
		},
		{
			name: "with a new commit",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "4", Parents: []string{"3", "5"}},
				{Hash: "Z", Parents: []string{"Z"}},
				{Hash: "3", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"6"}},
				{Hash: "6", Parents: []string{"7"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			4 │ ◎─╮
			Z │ │ │ ○
			3 ○─╯ │ │
			5 ○───╯ │
			6 ○ ╭───╯`,
		},
		{
			name: "with a root commit followed by an unrelated history",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2"},
				{Hash: "A", Parents: []string{"B"}},
				{Hash: "B"},
			},
			expectedOutput: `
			1 ○
			2 ○
			A ○
			B ○`,
		},
		{
			name: "with a merge of an unrelated history",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "A"}},
				{Hash: "2", Parents: []string{"3"}},
				{Hash: "A"},
				{Hash: "3"},
			},
			expectedOutput: `
			1 ◎─╮
			2 ○ │
			A │ ○
			3 ○`,
		},
		{
			name: "with a path that has room to move to the left and continues",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "3", Parents: []string{"5", "4"}},
				{Hash: "5", Parents: []string{"7", "8"}},
				{Hash: "4", Parents: []string{"7"}},
				{Hash: "7", Parents: []string{"11"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			3 ◎─│─╮
			5 ◎─│─│─╮
			4 │ ○─╯ │
			7 ○─╯ ╭─╯`,
		},
		{
			name: "with a path that has room to move to the left and continues",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "3", Parents: []string{"5", "4"}},
				{Hash: "5", Parents: []string{"7", "8"}},
				{Hash: "7", Parents: []string{"4", "A"}},
				{Hash: "4", Parents: []string{"B"}},
				{Hash: "B", Parents: []string{"C"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			3 ◎─│─╮
			5 ◎─│─│─╮
			7 ◎─│─│─│─╮
			4 ○─┴─╯ │ │
			B ○ ╭───╯ │`,
		},
		{
			name: "with a path that has room to move to the left and continues",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3"}},
				{Hash: "3", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"4", "5"}},
				{Hash: "4", Parents: []string{"6", "7"}},
				{Hash: "6", Parents: []string{"8"}},
			},
			expectedOutput: `
			1 ◎─╮
			3 │ ○
			2 ◎─│
			4 ◎─│─╮
			6 ○ │ │`,
		},
		{
			name: "new merge path fills gap before continuing path on right",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3", "4", "5"}},
				{Hash: "4", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"A"}},
				{Hash: "A", Parents: []string{"6", "B"}},
				{Hash: "B", Parents: []string{"C"}},
			},
			expectedOutput: `
			1 ◎─┬─┬─╮
			4 │ │ ○ │
			2 ○─│─╯ │
			A ◎─│─╮ │
			B │ │ ○ │`,
		},
		{
			name: "with a path that has room to move to the left and continues",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "3", Parents: []string{"5", "4"}},
				{Hash: "5", Parents: []string{"7", "8"}},
				{Hash: "7", Parents: []string{"4", "A"}},
				{Hash: "4", Parents: []string{"B"}},
				{Hash: "B", Parents: []string{"C"}},
				{Hash: "C", Parents: []string{"D"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			3 ◎─│─╮
			5 ◎─│─│─╮
			7 ◎─│─│─│─╮
			4 ○─┴─╯ │ │
			B ○ ╭───╯ │
			C ○ │ ╭───╯`,
		},
		{
			name: "with a path that has room to move to the left and continues",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "3", Parents: []string{"5", "4"}},
				{Hash: "5", Parents: []string{"7", "G"}},
				{Hash: "7", Parents: []string{"8", "A"}},
				{Hash: "8", Parents: []string{"4", "E"}},
				{Hash: "4", Parents: []string{"B"}},
				{Hash: "B", Parents: []string{"C"}},
				{Hash: "C", Parents: []string{"D"}},
				{Hash: "D", Parents: []string{"F"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			3 ◎─│─╮
			5 ◎─│─│─╮
			7 ◎─│─│─│─╮
			8 ◎─│─│─│─│─╮
			4 ○─┴─╯ │ │ │
			B ○ ╭───╯ │ │
			C ○ │ ╭───╯ │
			D ○ │ │ ╭───╯`,
		},
	}

	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hashPool := &StringPool{}

			colorFor := func(int) *Style { return &FgDefault }
			commits := lo.Map(test.commitOpts,
				func(opts CommitOpts, _ int) *Commit { return NewCommit(hashPool, opts) })
			lines := RenderCommitGraph(commits, hashPool.Add("blah"), colorFor, BoxDrawingSymbols)

			trimmedExpectedOutput := ""
			for line := range strings.SplitSeq(strings.TrimPrefix(test.expectedOutput, "\n"), "\n") {
				trimmedExpectedOutput += strings.TrimSpace(line) + "\n"
			}

			t.Log("\nexpected: \n" + trimmedExpectedOutput)

			output := ""
			for i, line := range lines {
				description := test.commitOpts[i].Hash
				output += strings.TrimSpace(description+" "+decolorise(line)) + "\n"
			}
			t.Log("\nactual: \n" + output)

			assert.Equal(t,
				trimmedExpectedOutput,
				output)
		})
	}
}

// Box drawing look-alikes for the branch drawing symbols, so that the expected
// output in tests is readable. The box drawing characters of the graph never
// use ┤ and ┼; here they stand for the symbols that have two bends. Commit
// symbols are shown as ○ and ◎, whichever lines they connect to.
var branchDrawingLookAlikes = func() map[rune]rune {
	lookAlikes := map[rune]rune{
		'\uf5d0': '─',
		'\uf5d1': '│',
		'\uf5d6': '╭',
		'\uf5d7': '╮',
		'\uf5d8': '╰',
		'\uf5d9': '╯',
		'\uf5e0': '┬', // ╮ on ─
		'\uf5e3': '┴', // ╯ on ─
		'\uf5df': '┤', // ╯ and ╮
		'\uf5e8': '┼', // ╯ and ╮ on ─
	}
	for _, symbols := range branchDrawingCommitSymbols {
		lookAlikes[[]rune(symbols.commit)[0]] = CommitSymbol
		lookAlikes[[]rune(symbols.merge)[0]] = MergeSymbol
	}
	return lookAlikes
}()

func TestRenderCommitGraphWithBranchDrawingSymbols(t *testing.T) {
	tests := []struct {
		name           string
		commitOpts     []CommitOpts
		selectedHash   string
		expectedOutput string
	}{
		{
			name: "branch forked off a merge commit",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3"}},
				{Hash: "3", Parents: []string{"5"}},
				{Hash: "2", Parents: []string{"5", "4"}},
				{Hash: "4", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"6", "7"}},
				{Hash: "7", Parents: []string{"6"}},
				{Hash: "6", Parents: []string{"8"}},
			},
			expectedOutput: `
			1 ◎─╮
			3 │ ○
			2 ◎─│─╮
			4 │ │ ○
			5 ◎─┼─╯
			7 │ ○
			6 ○─╯`,
		},
		{
			name: "branch forked off a merge commit, with the merge commit selected",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3"}},
				{Hash: "3", Parents: []string{"5"}},
				{Hash: "2", Parents: []string{"5", "4"}},
				{Hash: "4", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"6", "7"}},
				{Hash: "7", Parents: []string{"6"}},
				{Hash: "6", Parents: []string{"8"}},
			},
			selectedHash: "5",
			expectedOutput: `
			1 ◎─╮
			3 │ ○
			2 ◎─│─╮
			4 │ │ ○
			5 ◎─┼─╯
			7 │ ○
			6 ○─╯`,
		},
		{
			name: "branch forked off a merge commit, with the merge commit above it selected",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3"}},
				{Hash: "3", Parents: []string{"5"}},
				{Hash: "2", Parents: []string{"5", "4"}},
				{Hash: "4", Parents: []string{"5"}},
				{Hash: "5", Parents: []string{"6", "7"}},
				{Hash: "7", Parents: []string{"6"}},
				{Hash: "6", Parents: []string{"8"}},
			},
			selectedHash: "2",
			expectedOutput: `
			1 ◎─╮
			3 │ ○
			2 ◎───╮
			4 │ │ ○
			5 ◎─┼─╯
			7 │ ○
			6 ○─╯`,
		},
		{
			name: "branch forked off a merge commit, with no line passing through",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2", "3"}},
				{Hash: "3", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"4", "5"}},
				{Hash: "4", Parents: []string{"6", "7"}},
				{Hash: "6", Parents: []string{"8"}},
			},
			expectedOutput: `
			1 ◎─╮
			3 │ ○
			2 ◎─┤
			4 ◎─│─╮
			6 ○ │ │`,
		},
		{
			name: "several lines ending in a commit",
			commitOpts: []CommitOpts{
				{Hash: "1", Parents: []string{"2"}},
				{Hash: "2", Parents: []string{"3", "4"}},
				{Hash: "3", Parents: []string{"5", "4"}},
				{Hash: "5", Parents: []string{"7", "8"}},
				{Hash: "7", Parents: []string{"4", "A"}},
				{Hash: "4", Parents: []string{"B"}},
				{Hash: "B", Parents: []string{"C"}},
			},
			expectedOutput: `
			1 ○
			2 ◎─╮
			3 ◎─│─╮
			5 ◎─│─│─╮
			7 ◎─│─│─│─╮
			4 ○─┴─╯ │ │
			B ○ ╭───╯ │`,
		},
	}

	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hashPool := &StringPool{}

			colorFor := func(int) *Style { return &FgDefault }
			commits := lo.Map(test.commitOpts,
				func(opts CommitOpts, _ int) *Commit { return NewCommit(hashPool, opts) })
			lines := RenderCommitGraph(commits, hashPool.Add(test.selectedHash), colorFor, BranchDrawingSymbols)

			trimmedExpectedOutput := ""
			for line := range strings.SplitSeq(strings.TrimPrefix(test.expectedOutput, "\n"), "\n") {
				trimmedExpectedOutput += strings.TrimSpace(line) + "\n"
			}

			output := ""
			for i, line := range lines {
				lookAlikes := strings.Map(func(r rune) rune {
					return lo.ValueOr(branchDrawingLookAlikes, r, r)
				}, decolorise(line))
				output += strings.TrimSpace(test.commitOpts[i].Hash+" "+lookAlikes) + "\n"
			}

			assert.Equal(t, trimmedExpectedOutput, output)
		})
	}
}

func TestRenderPipeSetWithBranchDrawingSymbols(t *testing.T) {
	cyan := FgCyan
	red := FgRed
	green := FgGreen
	yellow := FgYellow
	magenta := FgMagenta
	nothing := Nothing

	hashPool := &StringPool{}
	pool := func(s string) *string { return hashPool.Add(s) }

	tests := []struct {
		name           string
		pipes          []Pipe
		prevCommit     *Commit
		expectedStr    string
		expectedStyles []Style
	}{
		{
			name: "commit with lines above and below",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "\uf5fb", // ○ with lines up and down
			expectedStyles: []Style{green},
		},
		{
			name: "first commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: &StartCommitHash, toHash: pool("b"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: STARTS, style: &green},
			},
			expectedStr:    "\uf5f7", // ○ with a line down
			expectedStyles: []Style{green},
		},
		{
			name: "root commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: &EmptyTreeCommitHash, kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "\uf5f9", // ○ with a line up
			expectedStyles: []Style{green},
		},
		{
			name: "selected root commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("selected"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: &EmptyTreeCommitHash, kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "\uf5f9", // ○ with a line up
			expectedStyles: []Style{highlightStyle},
		},
		{
			name: "selected commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("selected"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("c"), kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "\uf5fb", // ○ with lines up and down
			expectedStyles: []Style{highlightStyle},
		},
		{
			name: "commit whose previous commit is selected and is a merge commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &green},
				{fromPos: 1, toPos: 1, fromHash: pool("selected"), toHash: pool("b3"), kind: CONTINUES, style: &red},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "selected"}),
			expectedStr: "\uf5fb \uf5d1", // ○ with lines up and down, │
			expectedStyles: []Style{
				highlightStyle, nothing, highlightStyle,
			},
		},
		{
			name: "merge commit that a branch forks off",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 0, fromHash: pool("b1"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
				{fromPos: 2, toPos: 0, fromHash: pool("c1"), toHash: pool("a2"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "\uf604\uf5d0\uf5e8\uf5d0\uf5d9", // ◎ with lines up, down and right, ─, ╯ and ╮ on ─, ─, ╯
			expectedStyles: []Style{
				green, green, magenta, cyan, cyan,
			},
		},
		{
			name: "merge commit that a branch forks off, with the branch's first commit selected",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
				{fromPos: 2, toPos: 0, fromHash: pool("c1"), toHash: pool("a2"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "\uf604\uf5d0\uf5e8\uf5d0\uf5d9", // ◎ with lines up, down and right, ─, ╯ and ╮ on ─, ─, ╯
			expectedStyles: []Style{
				highlightStyle, highlightStyle, highlightStyle, cyan, cyan,
			},
		},
		{
			name: "line of the selected commit crossing another line",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("selected"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 1, fromHash: pool("b1"), toHash: pool("b2"), kind: CONTINUES, style: &magenta},
				{fromPos: 3, toPos: 0, fromHash: pool("e1"), toHash: pool("selected"), kind: TERMINATES, style: &green},
				{fromPos: 0, toPos: 2, fromHash: pool("selected"), toHash: pool("c3"), kind: STARTS, style: &yellow},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "\uf604\uf5d0\uf5d0\uf5d0\uf5e0\uf5d0\uf5d9", // ◎ with lines up, down and right, ─, ─, ─, ╮ on ─, ─, ╯
			expectedStyles: []Style{
				highlightStyle, highlightStyle, highlightStyle, highlightStyle, highlightStyle, green, green,
			},
		},
		{
			name: "line of the selected commit passing through a cell in which other lines bend",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 0, fromHash: pool("b1"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
				{fromPos: 2, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "\uf604\uf5d0\uf5e8\uf5d0\uf5d9", // ◎ with lines up, down and right, ─, ╯ and ╮ on ─, ─, ╯
			expectedStyles: []Style{
				highlightStyle, highlightStyle, magenta, highlightStyle, highlightStyle,
			},
		},
	}

	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualStr := renderPipeSet(test.pipes, pool("selected"), test.prevCommit, BranchDrawingSymbols, nil)
			if len([]rune(test.expectedStr)) != len(test.expectedStyles) {
				t.Fatalf("Error in test setup: you have %d characters in the expected output but have specified %d styles", len([]rune(test.expectedStr)), len(test.expectedStyles))
			}
			expectedStr := ""
			for i, char := range []rune(test.expectedStr) {
				expectedStr += test.expectedStyles[i].Sprint(string(char))
			}
			expectedStr += " "

			assert.Equal(t, expectedStr, actualStr)
		})
	}
}

func TestBranchDrawingSymbolsCoverAllCells(t *testing.T) {
	hashPool := &StringPool{}
	commits := generateCommits(hashPool, 1000)
	colorFor := func(int) *Style { return &FgDefault }
	isBoxDrawingChar := func(r rune) bool { return r >= '\u2500' && r <= '\u257f' }

	for _, selectedHash := range []string{"none", commits[10].Hash(), commits[500].Hash()} {
		lines := RenderCommitGraph(commits, hashPool.Add(selectedHash), colorFor, BranchDrawingSymbols)
		for i, line := range lines {
			line = decolorise(line)
			assert.False(t, strings.ContainsFunc(line, isBoxDrawingChar),
				"row %d falls back to box drawing characters: %q", i, line)
		}
	}
}

func TestRenderPipeSet(t *testing.T) {
	cyan := FgCyan
	red := FgRed
	green := FgGreen
	// blue := FgBlue
	yellow := FgYellow
	magenta := FgMagenta
	nothing := Nothing

	hashPool := &StringPool{}
	pool := func(s string) *string { return hashPool.Add(s) }

	tests := []struct {
		name           string
		pipes          []Pipe
		commit         *Commit
		prevCommit     *Commit
		expectedStr    string
		expectedStyles []Style
	}{
		{
			name: "single cell",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "○",
			expectedStyles: []Style{green},
		},
		{
			name: "single cell, selected",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("selected"), kind: TERMINATES, style: &cyan},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("c"), kind: STARTS, style: &green},
			},
			prevCommit:     NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr:    "○",
			expectedStyles: []Style{highlightStyle},
		},
		{
			name: "terminating hook and starting hook, selected",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("selected"), kind: TERMINATES, style: &cyan},
				{fromPos: 1, toPos: 0, fromHash: pool("c"), toHash: pool("selected"), kind: TERMINATES, style: &yellow},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("d"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("selected"), toHash: pool("e"), kind: STARTS, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr: "◎─╮",
			expectedStyles: []Style{
				highlightStyle, highlightStyle, highlightStyle,
			},
		},
		{
			name: "terminating hook and starting hook, prioritise the terminating one",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 0, fromHash: pool("c"), toHash: pool("b"), kind: TERMINATES, style: &magenta},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("d"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("b"), toHash: pool("e"), kind: STARTS, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a"}),
			expectedStr: "◎─│",
			expectedStyles: []Style{
				green, green, magenta,
			},
		},
		{
			name: "starting and terminating pipe sharing some space",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 1, fromHash: pool("b1"), toHash: pool("b2"), kind: CONTINUES, style: &magenta},
				{fromPos: 3, toPos: 0, fromHash: pool("e1"), toHash: pool("a2"), kind: TERMINATES, style: &green},
				{fromPos: 0, toPos: 2, fromHash: pool("a2"), toHash: pool("c3"), kind: STARTS, style: &yellow},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "◎─│─┬─╯",
			expectedStyles: []Style{
				yellow, yellow, magenta, yellow, yellow, green, green,
			},
		},
		{
			name: "starting and terminating pipe sharing some space, with selection",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("selected"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 1, fromHash: pool("b1"), toHash: pool("b2"), kind: CONTINUES, style: &magenta},
				{fromPos: 3, toPos: 0, fromHash: pool("e1"), toHash: pool("selected"), kind: TERMINATES, style: &green},
				{fromPos: 0, toPos: 2, fromHash: pool("selected"), toHash: pool("c3"), kind: STARTS, style: &yellow},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "◎───╮ ╯",
			expectedStyles: []Style{
				highlightStyle, highlightStyle, highlightStyle, highlightStyle, highlightStyle, nothing, green,
			},
		},
		{
			name: "many terminating pipes",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 0, fromHash: pool("b1"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
				{fromPos: 2, toPos: 0, fromHash: pool("c1"), toHash: pool("a2"), kind: TERMINATES, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "○─┴─╯",
			expectedStyles: []Style{
				yellow, magenta, magenta, green, green,
			},
		},
		{
			name: "starting pipe passing through",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 0, toPos: 3, fromHash: pool("a2"), toHash: pool("d3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 1, fromHash: pool("b1"), toHash: pool("b3"), kind: CONTINUES, style: &magenta},
				{fromPos: 2, toPos: 2, fromHash: pool("c1"), toHash: pool("c3"), kind: CONTINUES, style: &green},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "◎─│─│─╮",
			expectedStyles: []Style{
				yellow, yellow, magenta, yellow, green, yellow, yellow,
			},
		},
		{
			name: "starting and terminating path crossing continuing path",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &yellow},
				{fromPos: 1, toPos: 1, fromHash: pool("b1"), toHash: pool("a2"), kind: CONTINUES, style: &green},
				{fromPos: 2, toPos: 0, fromHash: pool("c1"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "◎─│─╯",
			expectedStyles: []Style{
				yellow, yellow, green, magenta, magenta,
			},
		},
		{
			name: "another clash of starting and terminating paths",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &yellow},
				{fromPos: 2, toPos: 2, fromHash: pool("c1"), toHash: pool("c3"), kind: CONTINUES, style: &green},
				{fromPos: 3, toPos: 0, fromHash: pool("d1"), toHash: pool("a2"), kind: TERMINATES, style: &magenta},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "a1"}),
			expectedStr: "◎─┬─│─╯",
			expectedStyles: []Style{
				yellow, yellow, yellow, magenta, green, magenta, magenta,
			},
		},
		{
			name: "commit whose previous commit is selected",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &yellow},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "selected"}),
			expectedStr: "○",
			expectedStyles: []Style{
				yellow,
			},
		},
		{
			name: "commit whose previous commit is selected and is a merge commit",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 1, fromHash: pool("selected"), toHash: pool("b3"), kind: CONTINUES, style: &red},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "selected"}),
			expectedStr: "○ │",
			expectedStyles: []Style{
				highlightStyle, nothing, highlightStyle,
			},
		},
		{
			name: "commit whose previous commit is selected and is a merge commit, with continuing pipe inbetween",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 1, toPos: 1, fromHash: pool("z1"), toHash: pool("z3"), kind: CONTINUES, style: &green},
				{fromPos: 2, toPos: 2, fromHash: pool("selected"), toHash: pool("b3"), kind: CONTINUES, style: &red},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "selected"}),
			expectedStr: "○ │ │",
			expectedStyles: []Style{
				highlightStyle, nothing, green, nothing, highlightStyle,
			},
		},
		{
			name: "when previous commit is selected, not a merge commit, and spawns a continuing pipe",
			pipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a1"), toHash: pool("a2"), kind: TERMINATES, style: &red},
				{fromPos: 0, toPos: 0, fromHash: pool("a2"), toHash: pool("a3"), kind: STARTS, style: &green},
				{fromPos: 0, toPos: 1, fromHash: pool("a2"), toHash: pool("b3"), kind: STARTS, style: &green},
				{fromPos: 1, toPos: 0, fromHash: pool("selected"), toHash: pool("a2"), kind: TERMINATES, style: &yellow},
			},
			prevCommit:  NewCommit(hashPool, CommitOpts{Hash: "selected"}),
			expectedStr: "◎─╯",
			expectedStyles: []Style{
				highlightStyle, highlightStyle, highlightStyle,
			},
		},
	}

	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualStr := renderPipeSet(test.pipes, pool("selected"), test.prevCommit, BoxDrawingSymbols, nil)
			t.Log("actual cells:")
			t.Log(actualStr)
			expectedStr := ""
			if len([]rune(test.expectedStr)) != len(test.expectedStyles) {
				t.Fatalf("Error in test setup: you have %d characters in the expected output (%s) but have specified %d styles", len([]rune(test.expectedStr)), test.expectedStr, len(test.expectedStyles))
			}
			for i, char := range []rune(test.expectedStr) {
				expectedStr += test.expectedStyles[i].Sprint(string(char))
			}
			expectedStr += " "
			t.Log("expected cells:")
			t.Log(expectedStr)

			assert.Equal(t, expectedStr, actualStr)
		})
	}
}

func TestGetNextPipes(t *testing.T) {
	hashPool := &StringPool{}
	pool := func(s string) *string { return hashPool.Add(s) }

	tests := []struct {
		prevPipes []Pipe
		commit    *Commit
		expected  []Pipe
	}{
		{
			prevPipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: STARTS, style: &FgDefault},
			},
			commit: NewCommit(hashPool, CommitOpts{
				Hash:    "b",
				Parents: []string{"c"},
			}),
			expected: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &FgDefault},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: STARTS, style: &FgDefault},
			},
		},
		{
			prevPipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("b"), kind: TERMINATES, style: &FgDefault},
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: STARTS, style: &FgDefault},
				{fromPos: 0, toPos: 1, fromHash: pool("b"), toHash: pool("d"), kind: STARTS, style: &FgDefault},
			},
			commit: NewCommit(hashPool, CommitOpts{
				Hash:    "d",
				Parents: []string{"e"},
			}),
			expected: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("b"), toHash: pool("c"), kind: CONTINUES, style: &FgDefault},
				{fromPos: 1, toPos: 1, fromHash: pool("b"), toHash: pool("d"), kind: TERMINATES, style: &FgDefault},
				{fromPos: 1, toPos: 1, fromHash: pool("d"), toHash: pool("e"), kind: STARTS, style: &FgDefault},
			},
		},
		{
			prevPipes: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("a"), toHash: pool("root"), kind: TERMINATES, style: &FgDefault},
			},
			commit: NewCommit(hashPool, CommitOpts{
				Hash:    "root",
				Parents: []string{},
			}),
			expected: []Pipe{
				{fromPos: 0, toPos: 0, fromHash: pool("root"), toHash: pool("4b825dc642cb6eb9a060e54bf8d69288fbee4904"), kind: STARTS, style: &FgDefault},
			},
		},
	}

	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	for _, test := range tests {
		colorFor := func(int) *Style { return &FgDefault }
		pipes := getNextPipes(test.prevPipes, test.commit, colorFor)
		// rendering cells so that it's easier to see what went wrong
		actualStr := renderPipeSet(pipes, pool("selected"), nil, BoxDrawingSymbols, nil)
		expectedStr := renderPipeSet(test.expected, pool("selected"), nil, BoxDrawingSymbols, nil)
		t.Log("expected cells:")
		t.Log(expectedStr)
		t.Log("actual cells:")
		t.Log(actualStr)
		assert.EqualValues(t, test.expected, pipes)
	}
}

func BenchmarkRenderCommitGraph(b *testing.B) {
	oldColorLevel := color.ForceSetColorLevel(terminfo.ColorLevelMillions)
	defer color.ForceSetColorLevel(oldColorLevel)

	hashPool := &StringPool{}

	commits := generateCommits(hashPool, 50)
	colorFor := func(int) *Style { return &FgDefault }
	b.ResetTimer()
	for b.Loop() {
		RenderCommitGraph(commits, hashPool.Add("selected"), colorFor, BoxDrawingSymbols)
	}
}

func generateCommits(hashPool *StringPool, count int) []*Commit {
	rnd := rand.New(rand.NewSource(1234))
	pool := []*Commit{NewCommit(hashPool, CommitOpts{Hash: "a"})}
	commits := make([]*Commit, 0, count)
	for len(commits) < count {
		currentCommitIdx := rnd.Intn(len(pool))
		currentCommit := pool[currentCommitIdx]
		pool = append(pool[0:currentCommitIdx], pool[currentCommitIdx+1:]...)
		// I need to pick a random number of parents to add
		parentCount := rnd.Intn(2) + 1

		parentHashes := currentCommit.Parents()
		for j := range parentCount {
			reuseParent := rnd.Intn(6) != 1 && j <= len(pool)-1 && j != 0
			var newParent *Commit
			if reuseParent {
				newParent = pool[j]
			} else {
				newParent = NewCommit(hashPool, CommitOpts{
					Hash: fmt.Sprintf("%s%d", currentCommit.Hash(), j),
				})
				pool = append(pool, newParent)
			}
			parentHashes = append(parentHashes, newParent.Hash())
		}

		changedCommit := NewCommit(hashPool, CommitOpts{
			Hash:    currentCommit.Hash(),
			Parents: parentHashes,
		})
		commits = append(commits, changedCommit)
	}

	return commits
}
