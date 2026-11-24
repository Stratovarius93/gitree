package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func sampleCommits() []git.Commit {
	return []git.Commit{
		{Hash: "aaaaaaa1111", Parents: []string{"bbbbbbb2222", "ccccccc3333"}, AuthorName: "Ana Perez", Time: testNow.Add(-5 * time.Minute),
			Subject: "Merge login", Refs: []git.Ref{{Name: "main", Kind: git.LocalBranch, IsHead: true}}},
		{Hash: "ccccccc3333", Parents: []string{"bbbbbbb2222"}, AuthorName: "Sam Rivera", Time: testNow.Add(-2 * time.Hour),
			Subject: "login form " + strings.Repeat("long ", 30), Refs: []git.Ref{{Name: "feature/login", Kind: git.LocalBranch}}},
		{Hash: "bbbbbbb2222", AuthorName: "", Time: testNow.Add(-48 * time.Hour), Subject: "root"},
	}
}

func TestBuildRowsWIP(t *testing.T) {
	wip := []git.FileChange{{Path: "a"}, {Path: "b"}, {Path: "c"}}
	rows := BuildRows(sampleCommits(), wip, nil, nil, "")
	assert.Len(t, rows, 4)
	assert.Equal(t, RowWIP, rows[0].Kind)
	assert.Equal(t, 3, rows[0].WIPCount)
	assert.Equal(t, RowCommit, rows[1].Kind)

	rows = BuildRows(sampleCommits(), nil, nil, nil, "")
	assert.Len(t, rows, 3)
	assert.Equal(t, RowCommit, rows[0].Kind)
	assert.Equal(t, "aaaaaaa1111", rows[0].Commit.Hash)
}

func TestBuildRowsEmpty(t *testing.T) {
	assert.Empty(t, BuildRows(nil, nil, nil, nil, ""))
	rows := BuildRows(nil, []git.FileChange{{Path: "a"}}, nil, nil, "")
	assert.Len(t, rows, 1)
	assert.Equal(t, RowWIP, rows[0].Kind)
}

func TestBuildRowsLanesAndLabels(t *testing.T) {
	rows := BuildRows(sampleCommits(), nil, map[string]git.Track{}, map[string]github.PR{"feature/login": {Number: 3}}, "ccccccc3333")
	assert.Equal(t, 0, rows[0].Lane)
	assert.Equal(t, 1, rows[1].Lane)
	assert.Equal(t, "feature/login", rows[1].Labels[0].Name)
	assert.NotNil(t, rows[1].Labels[0].PR)
	assert.NotEmpty(t, rows[0].Graph)
}

func TestBuildRowsWIPLaneFollowsHead(t *testing.T) {
	cs := sampleCommits()
	cs[0].Refs = nil
	cs[1].Refs = append(cs[1].Refs, git.Ref{Name: "HEAD", Kind: git.DetachedHead, IsHead: true})
	rows := BuildRows(cs, []git.FileChange{{Path: "a"}}, nil, nil, "")
	assert.Equal(t, rows[2].Lane, rows[0].Lane)
	assert.Equal(t, 1, rows[0].Lane)
}

func TestRenderRowWidth(t *testing.T) {
	rows := BuildRows(sampleCommits(), []git.FileChange{{Path: "a"}}, nil, nil, "aaaaaaa1111")
	for _, total := range []int{1, 8, 20, 40, 70, 90, 120, 160} {
		w := ComputeWidths(total, 6, 0, 18)
		for i, r := range rows {
			for _, sel := range []bool{false, true} {
				out := RenderRow(r, w, sel, testNow)
				assert.Equal(t, total, lipgloss.Width(out), "total %d row %d", total, i)
				assert.NotContains(t, out, "\n")
			}
		}
	}
}

func TestRenderRowContent(t *testing.T) {
	rows := BuildRows(sampleCommits(), []git.FileChange{{Path: "a"}}, nil, nil, "")
	w := ComputeWidths(160, 6, 0, 18)
	wip := ansi.Strip(RenderRow(rows[0], w, false, testNow))
	assert.Contains(t, wip, "// WIP")
	assert.Contains(t, wip, "1 file changed")
	assert.NotContains(t, wip, "files")
	c := ansi.Strip(RenderRow(rows[1], w, false, testNow))
	assert.Contains(t, c, "Merge login")
	assert.Contains(t, c, "Ana Perez")
	assert.Contains(t, c, "5m")
	assert.Contains(t, c, "aaaaaaa")
	assert.Contains(t, ansi.Strip(RenderRow(rows[3], w, false, testNow)), "?")
}

func TestSelectedRowBackgroundHasNoHoles(t *testing.T) {
	rows := BuildRows(sampleCommits(), []git.FileChange{{Path: "a"}}, nil, nil, "aaaaaaa1111")
	reset := regexp.MustCompile(`\x1b\[0?m`)
	for _, total := range []int{70, 160} {
		w := ComputeWidths(total, 6, 0, 18)
		for i, r := range rows {
			out := RenderRow(r, w, true, testNow)
			locs := reset.FindAllStringIndex(out, -1)
			for _, loc := range locs {
				rest := out[loc[1]:]
				if rest == "" {
					continue
				}
				end := strings.Index(rest, "m")
				assert.True(t, strings.HasPrefix(rest, "\x1b[") && strings.Contains(rest[:end+1], "48;2;60;56;54"),
					"total %d row %d: reset followed by %q", total, i, rest[:min(len(rest), 20)])
			}
			assert.True(t, strings.HasPrefix(out, "\x1b["), "row starts with an escape")
		}
	}
}
