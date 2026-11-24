package ui

import (
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sized(m Model, w, h int) Model {
	m, _ = update(m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func assertFits(t *testing.T, m Model) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	assert.LessOrEqual(t, len(lines), m.height)
	for i, l := range lines {
		assert.LessOrEqual(t, lipgloss.Width(l), m.width, "line %d: %q", i, l)
	}
}

func TestEnterOpensDetail(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	hash := r.Commit("add login screen", map[string]string{"b.txt": "b"})
	m := sized(loaded(t, r), 120, 30)

	next, cmd := update(m, keyMsg("enter"))
	require.NotNil(t, cmd)
	assert.Equal(t, ModeDetail, next.mode)
	var sawLoad bool
	for _, msg := range collect(cmd) {
		if dm, ok := msg.(detailLoadedMsg); ok {
			sawLoad = true
			assert.Equal(t, hash, dm.hash)
		}
	}
	assert.True(t, sawLoad)
	m = settle(t, next, cmd)
	view := m.View()
	assert.Contains(t, view, hash)
	assert.Len(t, hash, 40)
	assert.Contains(t, view, "add login screen")
	assert.Contains(t, view, "b.txt")
	assert.Less(t, m.listWidth(), m.width)
	assertFits(t, m)
}

func TestDetailFollowsCursor(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first commit", map[string]string{"a.txt": "a"})
	second := r.Commit("second commit", map[string]string{"b.txt": "b"})
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	assert.Contains(t, m.View(), second)
	assert.NotContains(t, m.View(), first)

	m = press(t, m, "j")
	assert.Contains(t, m.View(), first)
	assert.NotContains(t, m.View(), second)

	m, _ = update(m, detailLoadedMsg{hash: second, header: "commit " + second})
	assert.NotContains(t, m.View(), second, "stale detail is ignored")
}

func TestTabSwitchesFocus(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Commit("long", map[string]string{"b.txt": strings.Repeat("line\n", 200)})
	m := press(t, sized(loaded(t, r), 120, 20), "enter")
	require.Equal(t, 0, m.detail.vp.YOffset)

	m = press(t, m, "tab")
	assert.True(t, m.detail.focus)
	m = press(t, m, "j", "j")
	assert.Equal(t, 0, m.cursor, "j scrolls the detail, not the list")
	assert.Equal(t, 2, m.detail.vp.YOffset)

	m = press(t, m, "tab", "j")
	assert.False(t, m.detail.focus)
	assert.Equal(t, 1, m.cursor)
}

func TestEscClosesDetail(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	require.Equal(t, ModeDetail, m.mode)
	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.NotContains(t, m.View(), "a.txt")
	assertFits(t, m)
}

func TestDetailWIP(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "original\n"})
	writeFile(t, r, "a.txt", "changed-line\n")
	m := sized(loaded(t, r), 120, 30)
	require.Equal(t, RowWIP, m.Selected().Kind)
	m = press(t, m, "enter")
	view := m.View()
	assert.Contains(t, view, "a.txt")
	assert.Contains(t, view, "changed-line")
	assertFits(t, m)
}

func TestDetailShowsPR(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	r.Git("checkout", "-q", "-b", "feature")
	r.Commit("feature work", map[string]string{"f.txt": "f"})
	m := loaded(t, r)
	m.prs = map[string]github.PR{"feature": {Number: 42, Title: "Add the feature", Head: "feature", Base: "develop", URL: "https://x/42"}}
	m.rebuild()
	m = press(t, sized(m, 120, 30), "enter")
	view := m.View()
	assert.Contains(t, view, "#42")
	assert.Contains(t, view, "Add the feature")
	assert.Contains(t, view, "develop")
	assert.Contains(t, view, "feature")
}

func TestDetailResizeFits(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	for _, s := range []tea.WindowSizeMsg{{Width: 60, Height: 12}, {Width: 20, Height: 5}, {Width: 1, Height: 1}} {
		var cmd tea.Cmd
		m, cmd = update(m, s)
		m = settle(t, m, cmd)
		assertFits(t, m)
	}
}

// TestEnterClosesDetail: enter toggles the detail (opens and closes it).
func TestEnterClosesDetail(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	require.Equal(t, ModeDetail, m.mode)
	m = press(t, m, "enter")
	assert.Equal(t, ModeList, m.mode)
	m = press(t, m, "tab", "enter")
	assert.Equal(t, ModeDetail, m.mode, "tab in the list does nothing; enter reopens")
	m = press(t, m, "tab", "enter")
	assert.Equal(t, ModeList, m.mode, "enter closes with the detail focused too")
}
