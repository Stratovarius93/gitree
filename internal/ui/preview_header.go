package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Preview header layout: key column width, and the file-list rows the header
// always leaves on small terminals.
const (
	headerKeyWidth     = 11
	previewMinListRows = 3
)

// previewHeader is the summary block above the file list, ending with a
// separator, truncated so the list keeps at least previewMinListRows rows
// (nil when even that does not fit).
func (m Model) previewHeader(iw int) []string {
	bodyH := m.previewBodyHeight()
	budget := bodyH - min(previewMinListRows, bodyH)
	if budget < 2 {
		return nil
	}
	lines := m.previewHeaderLines(iw)
	lines = lines[:min(len(lines), budget-1)]
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Bg5)).Render(strings.Repeat("─", max(iw, 0)))
	return append(lines, rule)
}

// previewListRows is how many file rows fit below the header.
func (m Model) previewListRows() int {
	iw, _ := m.previewInner()
	return max(m.previewBodyHeight()-len(m.previewHeader(iw)), 1)
}

// previewHeaderLines is the full header, each line at most iw cells: the
// subject and key/value rows for a commit, "Working tree" for the WIP row,
// and the Changes row once the files are loaded.
func (m Model) previewHeaderLines(iw int) []string {
	p := m.preview
	var lines []string
	add := func(s string) { lines = append(lines, ansi.Truncate(s, iw, "…")) }
	kv := func(k, v string) { add(" " + floatMuted.Render(padTo(k, headerKeyWidth)) + v) }

	if p.commit == nil {
		add(floatBold.Render(" Working tree"))
	} else {
		c := p.commit
		subject, name, email, date := c.Subject, c.AuthorName, c.AuthorEmail, c.Time
		if mt := p.meta; mt != nil {
			subject, name, email, date = mt.Subject, mt.AuthorName, mt.AuthorEmail, mt.AuthorDate
		}
		add(floatBold.Render(" " + subject))
		kv("Author", person(name, email))
		if mt := p.meta; mt != nil {
			if len(mt.CoAuthors) > 0 {
				kv("Co-authors", strings.Join(mt.CoAuthors, ", "))
			}
			if mt.CommitterDiffers() {
				kv("Committer", person(mt.CommitterName, mt.CommitterEmail))
			}
		}
		kv("Date", date.Local().Format("Jan 2, 2006 15:04")+"  ("+relativeWords(date, m.now())+")")
		if len(p.labels) > 0 {
			hex := colors.HexForColumn(p.lane)
			pills := make([]string, len(p.labels))
			for i, l := range p.labels {
				one := []Label{l}
				pills[i] = RenderLabels(one, hex, NaturalLabelWidth(one))
			}
			kv("Refs", strings.Join(pills, " "))
		}
	}
	if !p.loading && p.err == "" {
		kv("Changes", changesSummary(p.files))
	}
	return lines
}

func person(name, email string) string {
	if email == "" {
		return name
	}
	return name + " <" + email + ">"
}

// changeOrder is the order of the per-status counts in the Changes row.
const changeOrder = "MADR?"

// changesSummary is "+58 −21   4 files: 2 M · 1 A · 1 D", colored, without
// zero groups.
func changesSummary(files []git.FileChange) string {
	fg := func(hex, s string) string { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(s) }
	added, deleted := previewTotals(files)
	counts := map[byte]int{}
	order := []byte(changeOrder)
	for _, f := range files {
		if counts[f.Status] == 0 && !strings.ContainsRune(changeOrder, rune(f.Status)) {
			order = append(order, f.Status)
		}
		counts[f.Status]++
	}
	var groups []string
	for _, s := range order {
		if n := counts[s]; n > 0 {
			groups = append(groups, fg(statusColor(s), strconv.Itoa(n)+" "+string(s)))
		}
	}
	noun := "files"
	if len(files) == 1 {
		noun = "file"
	}
	out := fg(colors.Green, "+"+strconv.Itoa(added)) + " " + fg(colors.Red, "−"+strconv.Itoa(deleted)) +
		fmt.Sprintf("   %d %s", len(files), noun)
	if len(groups) > 0 {
		out += ": " + strings.Join(groups, " · ")
	}
	return out
}

// relativeWords is t relative to now in words: "just now", "5 minutes ago",
// "3 days ago", "2 weeks ago", "1 year ago"…
func relativeWords(t, now time.Time) string {
	d := now.Sub(t)
	const day = 24 * time.Hour
	unit := func(n int, name string) string {
		if n != 1 {
			name += "s"
		}
		return strconv.Itoa(n) + " " + name + " ago"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return unit(int(d/time.Minute), "minute")
	case d < day:
		return unit(int(d/time.Hour), "hour")
	case d < 7*day:
		return unit(int(d/day), "day")
	case d < 30*day:
		return unit(int(d/(7*day)), "week")
	case d < 365*day:
		return unit(int(d/(30*day)), "month")
	}
	return unit(int(d/(365*day)), "year")
}
