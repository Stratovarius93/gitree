package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func barModel(t *testing.T) Model {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := sized(loaded(t, r), 140, 20)
	m.repoName = "demo"
	return m
}

// iconFg is the foreground of the first cell showing glyph in bar.
func iconFg(t *testing.T, bar, glyph string) string {
	t.Helper()
	for _, c := range cellsOf(bar) {
		if c.ch == glyph {
			return c.fg
		}
	}
	t.Fatalf("%q not in bar %q", glyph, ansi.Strip(bar))
	return ""
}

func TestStatusBarSegments(t *testing.T) {
	m := barModel(t).WithVersion("v1.2.3")
	m.lastFetch = time.Date(2026, 1, 2, 10, 25, 0, 0, time.UTC)
	m.track = map[string]git.Track{m.branch: {Ahead: 2, Behind: 1}}
	bar := m.statusBar()
	plain := ansi.Strip(bar)
	assert.Equal(t, m.width, lipgloss.Width(bar))
	for _, want := range []string{iconRepo + " demo", iconBranch + " " + m.branch + " ↑2 ↓1", iconLocal + " local", iconFetch + " fetched 10:25", iconVersion + " v1.2.3", plArrowR, plArrowL} {
		assert.Contains(t, plain, want)
	}
	assert.True(t, strings.HasSuffix(strings.TrimRight(plain, " "), "v1.2.3"), "the version is on the right: %q", plain)
	assert.Less(t, strings.Index(plain, "fetched"), strings.Index(plain, "v1.2.3"))
}

func TestStatusBarDefaultVersion(t *testing.T) {
	assert.Contains(t, ansi.Strip(barModel(t).statusBar()), iconVersion+" dev")
}

func TestStatusBarAllScope(t *testing.T) {
	m := barModel(t)
	m.scope = git.ScopeAll
	assert.Contains(t, ansi.Strip(m.statusBar()), iconAll+" all")
}

func TestStatusBarMessageKinds(t *testing.T) {
	m := barModel(t)

	m.setStatus("action", "error: boom")
	assert.Equal(t, fgOf(colors.Red), iconFg(t, m.statusBar(), iconError))

	m.setStatus("action", "")
	_ = m.flash("action", "pulled 2 commits")
	assert.Equal(t, fgOf(colors.Green), iconFg(t, m.statusBar(), iconOK))

	_ = m.flash("action", statusBusy)
	assert.Equal(t, fgOf(colors.Yellow), iconFg(t, m.statusBar(), iconWarn))

	m.setStatus("action", statusDirty)
	assert.Equal(t, fgOf(colors.Yellow), iconFg(t, m.statusBar(), iconWarn))

	m.acting = true
	m.setStatus("action", "pulling…")
	assert.Contains(t, ansi.Strip(m.statusBar()), "pulling ⠋")
}

func TestStatusBarFetchAndNotes(t *testing.T) {
	m := barModel(t)
	m.fetching = true
	assert.Contains(t, ansi.Strip(m.statusBar()), "fetching ⠋")
	m.fetching = false
	m.fetchErr = "error: fetch: offline"
	assert.Equal(t, fgOf(colors.Red), iconFg(t, m.statusBar(), iconError))
	m.prNote = "gh not installed: no PRs"
	assert.Contains(t, ansi.Strip(m.statusBar()), iconPR+" gh not installed")
}

func TestStatusBarWidthAlwaysExact(t *testing.T) {
	m := barModel(t).WithVersion("v1.2.3")
	m.lastFetch = time.Now()
	m.branch = "feature/" + strings.Repeat("very-long-name-", 8)
	m.setStatus("action", "error: "+strings.Repeat("something went wrong ", 10))
	for w := 1; w <= 200; w++ {
		m.width = w
		assert.Equal(t, w, lipgloss.Width(m.statusBar()), "width %d", w)
	}
}

func TestStatusBarNarrowDropsFetchFirst(t *testing.T) {
	m := barModel(t).WithVersion("v1.2.3")
	m.lastFetch = time.Now()
	m.width = 45
	plain := ansi.Strip(m.statusBar())
	assert.NotContains(t, plain, "fetched")
	assert.Contains(t, plain, "v1.2.3")
	assert.Contains(t, plain, "demo")
}

func TestViewUsesStatusBar(t *testing.T) {
	m := barModel(t).WithVersion("v1.2.3")
	lines := strings.Split(m.View(), "\n")
	require.NotEmpty(t, lines)
	assert.Contains(t, ansi.Strip(lines[len(lines)-1]), iconVersion+" v1.2.3")
}

func TestStatusBarKeepsRoomForMessages(t *testing.T) {
	m := barModel(t).WithVersion("v1.2.3")
	m.lastFetch = time.Now()
	m.width = 70
	m.setStatus("action", "error: push rejected by the remote")
	plain := ansi.Strip(m.statusBar())
	assert.Contains(t, plain, iconError+" error: push", "a message is not squeezed out by optional segments")
	assert.NotContains(t, plain, "fetched")
}
