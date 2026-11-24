package ui

import (
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestBuildLabelsMergesLocalAndRemote(t *testing.T) {
	refs := []git.Ref{
		{Name: "develop", Kind: git.LocalBranch, IsHead: true},
		{Name: "origin/develop", Kind: git.RemoteBranch},
	}
	track := map[string]git.Track{"develop": {Upstream: "origin/develop"}}
	got := BuildLabels(refs, track, nil)
	assert.Equal(t, []Label{{Name: "develop", Kind: LabelBranch, Local: true, Remote: true, IsHead: true}}, got)
}

func TestBuildLabelsSeparateRemote(t *testing.T) {
	got := BuildLabels([]git.Ref{{Name: "origin/develop", Kind: git.RemoteBranch}}, nil, nil)
	assert.Equal(t, []Label{{Name: "origin/develop", Kind: LabelRemote, Remote: true}}, got)
}

func TestBuildLabelsOrderAndPR(t *testing.T) {
	refs := []git.Ref{
		{Name: "v1", Kind: git.Tag},
		{Name: "origin/x", Kind: git.RemoteBranch},
		{Name: "y", Kind: git.LocalBranch},
		{Name: "z", Kind: git.LocalBranch, IsHead: true},
	}
	prs := map[string]github.PR{"y": {Number: 7}}
	got := BuildLabels(refs, nil, prs)
	var names []string
	for _, l := range got {
		names = append(names, l.Name)
	}
	assert.Equal(t, []string{"z", "y", "origin/x", "v1"}, names)
	if assert.NotNil(t, got[1].PR) {
		assert.Equal(t, 7, got[1].PR.Number)
	}
	assert.Nil(t, got[0].PR)
}

// TestBuildLabelsRemotePR: a remote branch shows the PR of its branch name
// (without the remote), and a local branch falls back to its upstream's.
func TestBuildLabelsRemotePR(t *testing.T) {
	prs := map[string]github.PR{"feature/x": {Number: 9}, "mine": {Number: 4}}
	got := BuildLabels([]git.Ref{{Name: "origin/feature/x", Kind: git.RemoteBranch, Remote: "origin"}}, nil, prs)
	if assert.NotNil(t, got[0].PR) {
		assert.Equal(t, 9, got[0].PR.Number)
	}
	track := map[string]git.Track{
		"local-x": {Upstream: "origin/feature/x", UpstreamBranch: "feature/x"},
		"mine":    {Upstream: "origin/other", UpstreamBranch: "other"},
	}
	got = BuildLabels([]git.Ref{{Name: "local-x", Kind: git.LocalBranch}, {Name: "mine", Kind: git.LocalBranch}}, track, prs)
	if assert.NotNil(t, got[0].PR, "upstream fallback") {
		assert.Equal(t, 9, got[0].PR.Number)
	}
	if assert.NotNil(t, got[1].PR, "own name wins") {
		assert.Equal(t, 4, got[1].PR.Number)
	}
	got = BuildLabels([]git.Ref{{Name: "origin/none", Kind: git.RemoteBranch, Remote: "origin"}}, nil, prs)
	assert.Nil(t, got[0].PR)
}

func TestBuildLabelsDetachedHeadFirst(t *testing.T) {
	got := BuildLabels([]git.Ref{{Name: "v1", Kind: git.Tag}, {Name: "HEAD", Kind: git.DetachedHead, IsHead: true}}, nil, nil)
	assert.Equal(t, "HEAD", got[0].Name)
	assert.Equal(t, LabelDetached, got[0].Kind)
}

func plain(s string) string { return strings.TrimSpace(ansi.Strip(s)) }

func TestRenderLabelsOverflow(t *testing.T) {
	ls := []Label{{Name: "main", Local: true}, {Name: "a", Local: true}, {Name: "b", Local: true}}
	out := RenderLabels(ls, colors.Blue, 24)
	assert.Equal(t, 24, lipgloss.Width(out))
	assert.True(t, strings.HasPrefix(plain(out), "main"), plain(out))
	assert.True(t, strings.HasSuffix(plain(out), "+2"), plain(out))
}

func TestRenderLabelsTruncates(t *testing.T) {
	out := RenderLabels([]Label{{Name: strings.Repeat("x", 40), Local: true}}, colors.Blue, 24)
	assert.Equal(t, 24, lipgloss.Width(out))
	assert.True(t, strings.HasSuffix(plain(out), "…"), plain(out))
}

func TestRenderLabelsAheadBehind(t *testing.T) {
	out := plain(RenderLabels([]Label{{Name: "m", Local: true, Ahead: 2, Behind: 1}}, colors.Blue, 24))
	assert.Contains(t, out, "↑2")
	assert.Contains(t, out, "↓1")
}

func TestRenderLabelsEmptyAndTiny(t *testing.T) {
	assert.Equal(t, 10, lipgloss.Width(RenderLabels(nil, colors.Blue, 10)))
	for w := 0; w < 30; w++ {
		ls := []Label{{Name: "feature/login", Local: true, Remote: true, IsHead: true, Ahead: 1, Behind: 2, PR: &github.PR{Number: 142}}, {Name: "b"}}
		assert.Equal(t, w, lipgloss.Width(RenderLabels(ls, colors.Blue, w)), w)
	}
}

func TestPRIcon(t *testing.T) {
	want := map[github.PRState]string{
		github.PROpen: colors.Green, github.PRDraft: colors.Gray,
		github.PRChangesRequested: colors.Red, github.PRMerged: colors.Purple,
	}
	for _, s := range []github.PRState{github.PROpen, github.PRDraft, github.PRApproved, github.PRChangesRequested, github.PRMerged, github.PRClosed} {
		g, hex := PRIcon(s)
		assert.NotEmpty(t, g)
		assert.NotEmpty(t, hex)
		if w, ok := want[s]; ok {
			assert.Equal(t, w, hex)
		}
	}
}

func TestPRChipUsesNeutralBackground(t *testing.T) {
	// Open PR is green, and lane 1 is also green: the chip must not use the lane background.
	pr := &github.PR{Number: 142, State: github.PROpen}
	text, hex := prChipText(pr)
	chip := prChip(pr, text, hex)
	assert.Contains(t, chip, "48;2;60;56;54", "DimBg background")
	assert.NotContains(t, chip, "48;2;169;182;101", "lane background")
	assert.Contains(t, chip, "38;2;169;182;101", "state foreground")

	out := RenderLabels([]Label{{Name: "feat", Local: true, PR: pr}}, colors.Green, 40)
	assert.Contains(t, out, chip)
	assert.Equal(t, 40, lipgloss.Width(out))
	assert.Contains(t, plain(out), "#142")
}

func TestPillHugsText(t *testing.T) {
	out := ansi.Strip(RenderLabels([]Label{{Name: "main", Local: true}, {Name: "x"}}, colors.Blue, 24))
	want := " main " + iconLocal + "  +1"
	assert.Equal(t, want+strings.Repeat(" ", 24-lipgloss.Width(want)), out)
}
