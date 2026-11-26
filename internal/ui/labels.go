package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// LabelKind classifies a label pill.
type LabelKind int

const (
	LabelBranch LabelKind = iota
	LabelRemote
	LabelTag
	LabelDetached
)

// Label is one branch/tag pill; a local branch and its upstream on the same
// commit are merged into one label with Local and Remote both set.
type Label struct {
	Name          string
	Kind          LabelKind
	Local, Remote bool
	IsHead        bool
	Ahead, Behind int
	PR            *github.PR
}

// Nerd Font glyphs.
const (
	iconLocal  = "\uf109"
	iconRemote = "\uf0c2"
	iconTag    = "\uf02b"
	headMark   = "●"
)

// BuildLabels turns a commit's refs into labels ordered HEAD > local > remote > tag.
func BuildLabels(refs []git.Ref, track map[string]git.Track, prs map[string]github.PR) []Label {
	present := map[string]bool{}
	for _, r := range refs {
		present[r.Name] = true
	}
	mergedRemote := map[string]bool{}
	var labels []Label
	for _, r := range refs {
		switch r.Kind {
		case git.LocalBranch:
			l := Label{Name: r.Name, Kind: LabelBranch, Local: true, IsHead: r.IsHead}
			if t, ok := track[r.Name]; ok {
				l.Ahead, l.Behind = t.Ahead, t.Behind
				if present[t.Upstream] {
					l.Remote = true
					mergedRemote[t.Upstream] = true
				}
			}
			l.PR = findPR(prs, r.Name, track[r.Name].UpstreamBranch)
			labels = append(labels, l)
		case git.Tag:
			labels = append(labels, Label{Name: r.Name, Kind: LabelTag, IsHead: r.IsHead})
		case git.DetachedHead:
			labels = append(labels, Label{Name: r.Name, Kind: LabelDetached, IsHead: true})
		}
	}
	for _, r := range refs {
		if r.Kind == git.RemoteBranch && !mergedRemote[r.Name] {
			pr := findPR(prs, strings.TrimPrefix(r.Name, r.Remote+"/"))
			labels = append(labels, Label{Name: r.Name, Kind: LabelRemote, Remote: true, IsHead: r.IsHead, PR: pr})
		}
	}
	rank := func(l Label) int {
		switch {
		case l.IsHead:
			return 0
		case l.Kind == LabelBranch:
			return 1
		case l.Kind == LabelRemote:
			return 2
		}
		return 3
	}
	sort.SliceStable(labels, func(i, j int) bool { return rank(labels[i]) < rank(labels[j]) })
	return labels
}

// findPR returns the PR of the first of branches (names on GitHub) that has one.
func findPR(prs map[string]github.PR, branches ...string) *github.PR {
	for _, b := range branches {
		if pr, ok := prs[b]; ok && b != "" {
			return &pr
		}
	}
	return nil
}

// PRIcon returns the Nerd Font glyph and color for a PR state.
func PRIcon(s github.PRState) (glyph string, hex string) {
	switch s {
	case github.PRDraft:
		return "\uf4dd", colors.Gray
	case github.PRApproved:
		return "\uf407\uf00c", "#b8bb26"
	case github.PRChangesRequested:
		return "\uf407\uf00d", colors.Red
	case github.PRMerged:
		return "\uf419", colors.Purple
	case github.PRClosed:
		return "\uf4dc", "#a05a56"
	}
	return "\uf407", colors.Green
}

// pillText is the plain text inside the lane-colored pill: padded name,
// ahead/behind and location icons. The PR lives in its own chip.
func pillText(l Label) string {
	text := l.Name
	if l.IsHead {
		text = headMark + " " + text
	}
	if l.Ahead > 0 {
		text += " ↑" + strconv.Itoa(l.Ahead)
	}
	if l.Behind > 0 {
		text += " ↓" + strconv.Itoa(l.Behind)
	}
	switch {
	case l.Kind == LabelTag:
		text += " " + iconTag
	case l.Local && l.Remote:
		text += " " + iconLocal + " " + iconRemote
	case l.Local:
		text += " " + iconLocal
	case l.Remote:
		text += " " + iconRemote
	}
	return " " + text + " "
}

func prChipText(pr *github.PR) (text, hex string) {
	g, hex := PRIcon(pr.State)
	return " " + g + " #" + strconv.Itoa(pr.Number) + " ", hex
}

// prChip renders a PR as a neutral chip (DimBg) so the state color stays
// readable whatever the lane color is.
func prChip(pr *github.PR, text string, hex string) string {
	return lipgloss.NewStyle().Background(lipgloss.Color(colors.DimBg)).Foreground(lipgloss.Color(hex)).Render(text)
}

// RenderLabels renders the most important label as a pill that hugs its text,
// then its PR chip, then " +N" when more labels exist; the rest is blank
// padding. The result is exactly width cells, truncating with "…".
func RenderLabels(labels []Label, laneHex string, width int) string {
	s, _ := renderLabelContent(labels, laneHex, width)
	return fit(s, width)
}

// renderLabelContent is RenderLabels without the padding: the content and
// how many cells it uses (at most width).
func renderLabelContent(labels []Label, laneHex string, width int) (string, int) {
	if width <= 0 || len(labels) == 0 {
		return "", 0
	}
	l := labels[0]
	bg, fg := laneHex, colors.DarkText
	switch {
	case l.Kind == LabelTag:
		bg = colors.Yellow
	case l.Kind == LabelRemote:
		bg, fg = colors.DimBg, laneHex
	}
	suffix := ""
	if n := len(labels) - 1; n > 0 {
		suffix = " +" + strconv.Itoa(n)
	}
	avail := width
	if sw := lipgloss.Width(suffix); avail-sw >= 4 {
		avail -= sw
	} else {
		suffix = ""
	}
	pill := pillText(l)
	if lipgloss.Width(pill) > avail {
		pill = ansi.Truncate(pill, avail, "…")
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Bold(l.IsHead).Render(pill))
	avail -= lipgloss.Width(pill)
	if l.PR != nil {
		text, hex := prChipText(l.PR)
		if lipgloss.Width(text) <= avail {
			b.WriteString(prChip(l.PR, text, hex))
			avail -= lipgloss.Width(text)
		}
	}
	if suffix != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray)).Render(suffix))
	}
	return b.String(), min(lipgloss.Width(b.String()), width)
}

// NaturalLabelWidth is the width RenderLabels needs to show a row's labels
// without truncation: the first pill, its PR chip and the " +N" suffix.
func NaturalLabelWidth(labels []Label) int {
	if len(labels) == 0 {
		return 0
	}
	n := lipgloss.Width(pillText(labels[0]))
	if pr := labels[0].PR; pr != nil {
		text, _ := prChipText(pr)
		n += lipgloss.Width(text)
	}
	if k := len(labels) - 1; k > 0 {
		n += lipgloss.Width(" +" + strconv.Itoa(k))
	}
	return n
}
