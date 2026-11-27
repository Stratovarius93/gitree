package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsLoadAfterLog(t *testing.T) {
	r := testutil.NewRepo(t)
	h := r.Commit("first", map[string]string{"a.txt": "1\n2\n3\n"})
	m := loaded(t, r)
	assert.Equal(t, git.Stat{Added: 3}, m.stats[h])
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 20})
	assert.Contains(t, ansi.Strip(m.View()), "+3")
}

func TestStatsPlaceholderBeforeLoad(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "1\n2\n3\n"})
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 20})
	msg := m.loadLog(0, false)()
	m, cmd := update(m, msg) // the stats command is not run
	require.NotNil(t, cmd)
	v := ansi.Strip(m.View())
	assert.Contains(t, v, "CHANGES")
	assert.Contains(t, v, "first")
	assert.NotContains(t, v, "+3")
}

func TestStatsCachedAcrossReload(t *testing.T) {
	r := testutil.NewRepo(t)
	h1 := r.Commit("first", map[string]string{"a.txt": "1\n"})
	m := loaded(t, r)
	h2 := r.Commit("second", map[string]string{"b.txt": "1\n2\n"})
	m.gen++
	m = settle(t, m, m.loadLog(0, false))
	require.Len(t, m.commits, 2)
	assert.Equal(t, git.Stat{Added: 1}, m.stats[h1])
	assert.Equal(t, git.Stat{Added: 2}, m.stats[h2])
	assert.Nil(t, m.loadStats(m.commits), "every hash is cached")
}

func TestStatsMissingOnlyNew(t *testing.T) {
	m := Model{stats: map[string]git.Stat{"a": {Added: 1}}}
	cs := []git.Commit{{Hash: "a"}, {Hash: "b"}}
	assert.Equal(t, []string{"b"}, m.missingStats(cs))
}

func TestStatsErrorIsSilent(t *testing.T) {
	r := testutil.NewRepo(t)
	h := r.Commit("first", map[string]string{"a.txt": "1\n"})
	m := loaded(t, r)
	before := m.Status()
	m, cmd := update(m, statsLoadedMsg{err: errors.New("boom")})
	assert.Nil(t, cmd)
	assert.Equal(t, before, m.Status())
	assert.False(t, strings.Contains(m.Status(), "boom"))
	assert.Equal(t, git.Stat{Added: 1}, m.stats[h])
}

// TestStatsNotRefetchedWhileInFlight: a page arriving while its hashes are
// being fetched does not request them again; a failed fetch frees them.
func TestStatsNotRefetchedWhileInFlight(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "1\n"})
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	msg := m.loadLog(0, false)().(logLoadedMsg)
	m, _ = update(m, msg) // stats command issued but not run
	hashes := m.missingStats(msg.commits)
	assert.Empty(t, hashes, "in flight: not missing")
	m.gen++
	msg.gen = m.gen
	m, _ = update(m, msg)
	assert.Empty(t, m.missingStats(msg.commits))
	m, _ = update(m, statsLoadedMsg{hashes: []string{msg.commits[0].Hash}, err: errors.New("boom")})
	assert.Equal(t, []string{msg.commits[0].Hash}, m.missingStats(msg.commits), "a failed fetch can be retried")
}
