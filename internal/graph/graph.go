// Adapted from github.com/jesseduffield/lazygit pkg/gui/presentation/graph (MIT). See LICENSE in this directory.

package graph

import (
	"cmp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/samber/lo"
)

type PipeKind uint8

const (
	TERMINATES PipeKind = iota
	STARTS
	CONTINUES
)

type Pipe struct {
	fromHash *string
	toHash   *string
	style    *Style
	fromPos  int16
	toPos    int16
	kind     PipeKind
}

var (
	highlightStyle      = FgLightWhite.Bold()
	EmptyTreeCommitHash = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	StartCommitHash     = "START"
)

func (self Pipe) left() int16 {
	return min(self.fromPos, self.toPos)
}

func (self Pipe) right() int16 {
	return max(self.fromPos, self.toPos)
}

// ColorFunc returns the style for graph column col. It must return the same
// *Style for the same column on every call.
type ColorFunc func(col int) *Style

func RenderCommitGraph(commits []*Commit, selectedCommitHashPtr *string, colorFor ColorFunc, symbolSet SymbolSet) []string {
	pipeSets := GetPipeSets(commits, colorFor)
	if len(pipeSets) == 0 {
		return nil
	}

	lines := RenderAux(pipeSets, commits, selectedCommitHashPtr, symbolSet, colorFor)

	return lines
}

func GetPipeSets(commits []*Commit, colorFor ColorFunc) [][]Pipe {
	if len(commits) == 0 {
		return nil
	}

	pipes := []Pipe{{fromPos: 0, toPos: 0, fromHash: &StartCommitHash, toHash: commits[0].HashPtr(), kind: STARTS, style: colorFor(0)}}

	return lo.Map(commits, func(commit *Commit, _ int) []Pipe {
		pipes = getNextPipes(pipes, commit, colorFor)
		return pipes
	})
}

func RenderAux(pipeSets [][]Pipe, commits []*Commit, selectedCommitHashPtr *string, symbolSet SymbolSet, colorFor ColorFunc) []string {
	maxProcs := runtime.GOMAXPROCS(0)

	// splitting up the rendering of the graph into multiple goroutines allows us to render the graph in parallel
	chunks := make([][]string, maxProcs)
	perProc := len(pipeSets) / maxProcs

	wg := sync.WaitGroup{}
	wg.Add(maxProcs)

	for i := range maxProcs {
		go func() {
			from := i * perProc
			to := (i + 1) * perProc
			if i == maxProcs-1 {
				to = len(pipeSets)
			}
			innerLines := make([]string, 0, to-from)
			for j, pipeSet := range pipeSets[from:to] {
				k := from + j
				var prevCommit *Commit
				if k > 0 {
					prevCommit = commits[k-1]
				}
				line := renderPipeSet(pipeSet, selectedCommitHashPtr, prevCommit, symbolSet, colorFor)
				innerLines = append(innerLines, line)
			}
			chunks[i] = innerLines
			wg.Done()
		}()
	}

	wg.Wait()

	return lo.Flatten(chunks)
}

// RenderRange renders only the lines for rows [from, to), clamped to the
// available rows. Each line depends only on its own pipe set, the previous
// commit and the selection, so the result equals RenderAux(...)[from:to]; a
// caller can compute the pipe sets once and re-render just the visible window
// when the selection moves.
func RenderRange(pipeSets [][]Pipe, commits []*Commit, selectedCommitHashPtr *string, symbolSet SymbolSet, colorFor ColorFunc, from, to int) []string {
	from, to = max(from, 0), min(to, len(pipeSets))
	if from >= to {
		return nil
	}
	lines := make([]string, 0, to-from)
	for k := from; k < to; k++ {
		var prevCommit *Commit
		if k > 0 {
			prevCommit = commits[k-1]
		}
		lines = append(lines, renderPipeSet(pipeSets[k], selectedCommitHashPtr, prevCommit, symbolSet, colorFor))
	}
	return lines
}

func getNextPipes(prevPipes []Pipe, commit *Commit, colorFor ColorFunc) []Pipe {
	// a pipe that terminated in the previous line has no bearing on the current line
	// so we'll filter those out. The same goes for the pipe from a root commit to
	// the empty tree, because no commit comes after a root commit.
	currentPipes := lo.Filter(prevPipes, func(pipe Pipe, _ int) bool {
		return pipe.kind != TERMINATES && !equalHashes(pipe.toHash, &EmptyTreeCommitHash)
	})

	maxPos := int16(-1)
	for _, pipe := range currentPipes {
		maxPos = max(maxPos, pipe.toPos)
	}

	newPipes := make([]Pipe, 0, len(currentPipes)+len(commit.ParentPtrs()))
	// start by assuming that we've got a brand new commit not related to any preceding commit.
	// (this only happens when we're doing `git log --all`). These will be tacked onto the far end.
	pos := maxPos + 1
	for _, pipe := range currentPipes {
		if equalHashes(pipe.toHash, commit.HashPtr()) {
			// turns out this commit does have a descendant so we'll place it right under the first instance
			pos = pipe.toPos
			break
		}
	}

	// a taken spot is one where a current pipe is ending on
	// Note: this set and similar ones below use int instead of int16 because
	// that's much more efficient. We cast the int16 values we store in these
	// sets to int on every access.
	takenSpots := intSet{}
	// a traversed spot is one where a current pipe is starting on, ending on, or passing through
	traversedSpots := intSet{}

	var toHash *string
	if commit.IsFirstCommit() {
		toHash = &EmptyTreeCommitHash
	} else {
		toHash = commit.ParentPtrs()[0]
	}
	newPipes = append(newPipes, Pipe{
		fromPos:  pos,
		toPos:    pos,
		fromHash: commit.HashPtr(),
		toHash:   toHash,
		kind:     STARTS,
		style:    nil,
	})

	traversedSpotsForContinuingPipes := intSet{}
	for _, pipe := range currentPipes {
		if !equalHashes(pipe.toHash, commit.HashPtr()) {
			traversedSpotsForContinuingPipes.add(int(pipe.toPos))
		}
	}

	getNextAvailablePosForContinuingPipe := func() int16 {
		i := int16(0)
		for {
			if !traversedSpots.has(int(i)) {
				return i
			}
			i++
		}
	}

	getNextAvailablePosForNewPipe := func() int16 {
		i := int16(0)
		for {
			// a newly created pipe is not allowed to end on a spot that's already taken,
			// nor on a spot that's been traversed by a continuing pipe.
			if !takenSpots.has(int(i)) && !traversedSpotsForContinuingPipes.has(int(i)) {
				return i
			}
			i++
		}
	}

	traverse := func(from, to int16) {
		left, right := from, to
		if left > right {
			left, right = right, left
		}
		for i := left; i <= right; i++ {
			traversedSpots.add(int(i))
		}
		takenSpots.add(int(to))
	}

	for _, pipe := range currentPipes {
		if equalHashes(pipe.toHash, commit.HashPtr()) {
			// terminating here
			newPipes = append(newPipes, Pipe{
				fromPos:  pipe.toPos,
				toPos:    pos,
				fromHash: pipe.fromHash,
				toHash:   pipe.toHash,
				kind:     TERMINATES,
				style:    pipe.style,
			})
			traverse(pipe.toPos, pos)
		} else if pipe.toPos < pos {
			// continuing here
			availablePos := getNextAvailablePosForContinuingPipe()
			newPipes = append(newPipes, Pipe{
				fromPos:  pipe.toPos,
				toPos:    availablePos,
				fromHash: pipe.fromHash,
				toHash:   pipe.toHash,
				kind:     CONTINUES,
				style:    pipe.style,
			})
			traverse(pipe.toPos, availablePos)
		}
	}

	if commit.IsMerge() {
		for _, parent := range commit.ParentPtrs()[1:] {
			availablePos := getNextAvailablePosForNewPipe()
			// need to act as if continuing pipes are going to continue on the same line.
			newPipes = append(newPipes, Pipe{
				fromPos:  pos,
				toPos:    availablePos,
				fromHash: commit.HashPtr(),
				toHash:   parent,
				kind:     STARTS,
				style:    nil,
			})

			takenSpots.add(int(availablePos))
		}
	}

	for _, pipe := range currentPipes {
		if !equalHashes(pipe.toHash, commit.HashPtr()) && pipe.toPos > pos {
			// continuing on, potentially moving left to fill in a blank spot
			last := pipe.toPos
			for i := pipe.toPos; i > pos; i-- {
				if takenSpots.has(int(i)) || traversedSpots.has(int(i)) {
					break
				}
				last = i
			}
			newPipes = append(newPipes, Pipe{
				fromPos:  pipe.toPos,
				toPos:    last,
				fromHash: pipe.fromHash,
				toHash:   pipe.toHash,
				kind:     CONTINUES,
				style:    pipe.style,
			})
			traverse(pipe.toPos, last)
		}
	}

	// not efficient but doing it for now: sorting my pipes by toPos, then by kind
	slices.SortFunc(newPipes, func(a, b Pipe) int {
		if a.toPos == b.toPos {
			return cmp.Compare(a.kind, b.kind)
		}
		return cmp.Compare(a.toPos, b.toPos)
	})

	// each line takes the color of the rightmost column it touches: straight
	// lines their own column, forks and merges the column of the branch that
	// leaves or arrives.
	for i := range newPipes {
		newPipes[i].style = colorFor(int(newPipes[i].right()))
	}

	return newPipes
}

// CommitColumn returns the column of the commit dot in a row's pipe set.
func CommitColumn(pipeSet []Pipe) int {
	for _, pipe := range pipeSet {
		if pipe.kind == STARTS {
			return int(pipe.fromPos)
		}
	}
	for _, pipe := range pipeSet {
		if pipe.kind == TERMINATES {
			return int(pipe.toPos)
		}
	}
	return 0
}

// PipeSetWidth is the width in cells of the line renderPipeSet draws for
// pipeSet (two cells per column), without rendering it.
func PipeSetWidth(pipeSet []Pipe) int {
	maxPos := int16(0)
	for _, pipe := range pipeSet {
		maxPos = max(maxPos, pipe.right())
	}
	return 2 * (int(maxPos) + 1)
}

func renderPipeSet(
	pipes []Pipe,
	selectedCommitHashPtr *string,
	prevCommit *Commit,
	symbolSet SymbolSet,
	colorFor ColorFunc,
) string {
	maxPos := int16(0)
	commitPos := int16(0)
	startCount := 0
	for _, pipe := range pipes {
		if pipe.kind == STARTS {
			startCount++
			commitPos = pipe.fromPos
		} else if pipe.kind == TERMINATES {
			commitPos = pipe.toPos
		}

		if pipe.right() > maxPos {
			maxPos = pipe.right()
		}
	}
	isMerge := startCount > 1

	cells := lo.Map(lo.Range(int(maxPos)+1), func(i int, _ int) *Cell {
		return &Cell{cellType: CONNECTION, style: &FgDefault}
	})

	renderPipe := func(pipe *Pipe, style *Style, overrideRightStyle bool) {
		left := pipe.left()
		right := pipe.right()

		if left != right {
			for i := left + 1; i < right; i++ {
				cells[i].setHorizontal(style, overrideRightStyle)
			}
			cells[left].setRight(style, overrideRightStyle)
			cells[right].setLeft(style)
		}

		if pipe.kind == STARTS || pipe.kind == CONTINUES {
			cells[pipe.toPos].setDown(style, lineTowards(pipe.toPos, pipe.fromPos))
		}
		if pipe.kind == TERMINATES || pipe.kind == CONTINUES {
			cells[pipe.fromPos].setUp(style, lineTowards(pipe.fromPos, pipe.toPos))
		}
	}

	// we don't want to highlight two commits if they're contiguous. We only want
	// to highlight multiple things if there's an actual visible pipe involved.
	highlight := true
	if prevCommit != nil && equalHashes(prevCommit.HashPtr(), selectedCommitHashPtr) {
		highlight = false
		for _, pipe := range pipes {
			if equalHashes(pipe.fromHash, selectedCommitHashPtr) && (pipe.kind != TERMINATES || pipe.fromPos != pipe.toPos) {
				highlight = true
			}
		}
	}

	// so we have our commit pos again, now it's time to build the cells.
	// we'll handle the one that's sourced from our selected commit last so that it can override the other cells.
	selectedPipes, nonSelectedPipes := lo.FilterReject(pipes, func(pipe Pipe, _ int) bool {
		return highlight && equalHashes(pipe.fromHash, selectedCommitHashPtr)
	})

	for _, pipe := range nonSelectedPipes {
		if pipe.kind == STARTS {
			renderPipe(&pipe, pipe.style, true)
		}
	}

	for _, pipe := range nonSelectedPipes {
		if pipe.kind == STARTS {
			continue
		}
		if pipe.kind == TERMINATES && pipe.fromPos == commitPos && pipe.toPos == commitPos {
			// Record this line without drawing it, so that the commit symbol
			// keeps the style of the commit itself. The branch drawing commit
			// symbols connect to it. The pipe from above the first commit
			// doesn't come from a commit, so it has no line.
			if !equalHashes(pipe.fromHash, &StartCommitHash) {
				cells[commitPos].up = straightLine
			}
			continue
		}
		renderPipe(&pipe, pipe.style, false)
	}

	// Safeguard that makes the rule explicit: the commit dot takes the color
	// of its own column, whatever lines meet in its cell (the pipes already
	// style it this way). The selected commit's highlight is applied below.
	if colorFor != nil {
		cells[commitPos].style = colorFor(int(commitPos))
	}

	// Box drawing characters can't show every combination of lines in a cell,
	// so the cells that the selected commit's lines run through show only
	// those lines. The branch drawing symbols can, so they keep the other
	// lines too.
	if symbolSet == BoxDrawingSymbols {
		for _, pipe := range selectedPipes {
			for i := pipe.left(); i <= pipe.right(); i++ {
				cells[i].reset()
			}
		}
	}
	for _, pipe := range selectedPipes {
		renderPipe(&pipe, &highlightStyle, true)
		if pipe.toPos == commitPos {
			cells[pipe.toPos].setStyle(&highlightStyle)
		}
	}

	// The pipe from a root commit to the empty tree gives the commit symbol the
	// style of the commit, but there is no line below a root commit
	for _, pipe := range pipes {
		if pipe.kind == STARTS && equalHashes(pipe.toHash, &EmptyTreeCommitHash) {
			cells[pipe.toPos].down = noLine
		}
	}

	// Where a line of the selected commit crosses the vertical line of another
	// commit, draw it over that line, so that it reads as one line. The
	// vertical line visibly continues above and below. A vertical line in a
	// cell that doesn't have the highlight style belongs to another commit.
	if symbolSet == BranchDrawingSymbols {
		for _, pipe := range selectedPipes {
			for i := pipe.left() + 1; i < pipe.right(); i++ {
				cell := cells[i]
				if cell.up == straightLine && cell.down == straightLine && cell.style != &highlightStyle {
					cell.horizontalOnTop = true
					cell.style = &highlightStyle
				}
			}
		}
	}

	cType := COMMIT
	if isMerge {
		cType = MERGE
	}

	cells[commitPos].setType(cType)

	// using a string builder here for the sake of performance
	writer := &strings.Builder{}
	writer.Grow(len(cells) * 2)
	for _, cell := range cells {
		cell.render(writer, symbolSet)
	}
	return writer.String()
}

// The course of a line through the top or bottom edge of the cell at pos,
// given the column of the pipe's other end
func lineTowards(pos, otherPos int16) verticalLine {
	switch {
	case otherPos < pos:
		return lineToLeft
	case otherPos > pos:
		return lineToRight
	default:
		return straightLine
	}
}

func equalHashes(a, b *string) bool {
	// if our selectedCommitHashPtr is nil, there is no selected commit
	if a == nil || b == nil {
		return false
	}

	// We know that all hashes are stored in the pool, so we can compare their addresses
	return a == b
}

// intSet is a minimal set of ints.
type intSet map[int]struct{}

func (s intSet) add(v int) { s[v] = struct{}{} }

func (s intSet) has(v int) bool {
	_, ok := s[v]
	return ok
}
