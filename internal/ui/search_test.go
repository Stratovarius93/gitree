package ui

import (
	"regexp"
	"testing"

	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchRepo has "login" in the subjects of three commits (rows 1, 3, 5 from
// the top, newest first) on a repo whose HEAD is the newest commit.
func searchRepo(t *testing.T) *testutil.Repo {
	r := testutil.NewRepo(t)
	r.Commit("c1 fix login bug", map[string]string{"a": "1"})
	r.Commit("c2 other", map[string]string{"b": "1"})
	r.Commit("c3 login form", map[string]string{"c": "1"})
	r.Commit("c4 other", map[string]string{"d": "1"})
	r.Git("checkout", "-q", "-b", "feature/x")
	r.Commit("c5 add LOGIN page", map[string]string{"e": "1"})
	r.Commit("c6 other", map[string]string{"f": "1"})
	return r
}

func typeQuery(t *testing.T, m Model, q string) Model {
	return press(t, m, "/", q, "enter")
}

func TestSearchJumps(t *testing.T) {
	m := loaded(t, searchRepo(t))
	require.Len(t, m.rows, 6)

	m = press(t, m, "/")
	assert.Equal(t, ModeSearch, m.mode)
	m = press(t, m, "login", "enter")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, 1, m.cursor)
	assert.Contains(t, m.Status(), "1/3")

	m = press(t, m, "n")
	assert.Equal(t, 3, m.cursor)
	assert.Contains(t, m.Status(), "2/3")
	m = press(t, m, "n")
	assert.Equal(t, 5, m.cursor)
	m = press(t, m, "n")
	assert.Equal(t, 1, m.cursor, "n wraps around")
	m = press(t, m, "N")
	assert.Equal(t, 5, m.cursor, "N wraps backwards")
	m = press(t, m, "N")
	assert.Equal(t, 3, m.cursor)
	assert.Contains(t, m.Status(), "2/3")
}

func TestSearchByBranchAndHash(t *testing.T) {
	r := searchRepo(t)
	m := loaded(t, r)

	m = typeQuery(t, m, "feature/")
	require.NotNil(t, m.Selected())
	assert.Equal(t, 0, m.cursor, "the only row labelled feature/x is the top one")
	assert.Contains(t, m.Status(), "1/1")

	hash := m.rows[4].Commit.Hash
	m = typeQuery(t, m, hash[:7])
	assert.Equal(t, 4, m.cursor)
	assert.Contains(t, m.Status(), "1/1")

	m = typeQuery(t, m, "AUTHOR-NOPE")
	assert.Contains(t, m.Status(), "no results")
}

func TestMatches(t *testing.T) {
	r := searchRepo(t)
	m := loaded(t, r)
	row := m.rows[0]
	assert.True(t, matches(row, "C6 OTHER"), "subject, case-insensitive")
	assert.True(t, matches(row, "test"), "author name")
	assert.True(t, matches(row, "feature/x"), "label name")
	assert.False(t, matches(row, "zzz"))
	assert.False(t, matches(Row{Kind: RowWIP}, "x"), "WIP rows never match")
}

func TestSearchNoMatch(t *testing.T) {
	m := loaded(t, searchRepo(t))
	m = press(t, m, "j", "j")
	m = typeQuery(t, m, "zzzzz")
	assert.Equal(t, 2, m.cursor)
	assert.Contains(t, m.Status(), "no results")
}

func TestSearchEscCancelsTyping(t *testing.T) {
	m := loaded(t, searchRepo(t))
	m = press(t, m, "/", "login", "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, 0, m.cursor)
	assert.NotContains(t, m.Status(), "/3")
	// q is text while typing, not quit.
	m = press(t, m, "/", "q")
	assert.Equal(t, ModeSearch, m.mode)
	assert.Contains(t, m.Status(), "q")
}

func TestHelpOverlay(t *testing.T) {
	m := loaded(t, searchRepo(t))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 60})
	m = press(t, m, "?")
	assert.Equal(t, ModeHelp, m.mode)
	view := ansi.Strip(m.View())
	for _, k := range []string{"j", "k", "g", "G", "ctrl+d", "ctrl+u", "/", "n", "N", "enter", "tab", "esc", "space", "x", "y", "o", "f", "a", "L", "r", "?", "q"} {
		assert.Regexp(t, `(^|[ │/])`+regexp.QuoteMeta(k)+`( |/|$|│)`, view, "key %q", k)
	}
	for _, s := range []github.PRState{github.PROpen, github.PRDraft, github.PRApproved, github.PRChangesRequested, github.PRMerged, github.PRClosed} {
		g, _ := PRIcon(s)
		assert.Contains(t, view, g)
	}
	for _, w := range []string{"Open", "Draft", "Approved", "Changes requested", "Merged", "Closed"} {
		assert.Contains(t, view, w)
	}
	for _, tc := range []string{"?", "esc"} {
		closed := press(t, m, tc)
		assert.Equal(t, ModeList, closed.mode, tc)
	}
}

// TestHelpScrollClamped: scrolling past the end of the help does not build
// up an offset that k then has to unwind.
func TestHelpScrollClamped(t *testing.T) {
	m := loaded(t, searchRepo(t))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = press(t, m, "?")
	for range 100 {
		m = press(t, m, "j")
	}
	bottom := m.View()
	m = press(t, m, "k")
	assert.NotEqual(t, bottom, m.View(), "one k scrolls back up at once")
}
