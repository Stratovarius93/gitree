package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timer runs cmd, which must be a single timer, and returns what it delivers.
func timer(t *testing.T, cmd tea.Cmd) deferredMsg {
	t.Helper()
	require.NotNil(t, cmd)
	d, ok := cmd().(deferredMsg)
	require.True(t, ok, "expected a timer")
	return d
}

// detailLoads counts the detail loads among the messages of cmds.
func detailLoads(cmds ...tea.Cmd) (n int, last detailLoadedMsg) {
	for _, c := range cmds {
		for _, msg := range collect(c) {
			if d, ok := msg.(detailLoadedMsg); ok {
				n, last = n+1, d
			}
		}
	}
	return n, last
}

// TestDetailMoveDebounced: moving through the list with the detail open
// loads only the row the cursor rests on, after detailDebounce.
func TestDetailMoveDebounced(t *testing.T) {
	m := sized(loaded(t, linearRepo(t, 10)), 120, 30)
	m = press(t, m, "enter")
	require.Equal(t, ModeDetail, m.mode)

	var timers []deferredMsg
	for range 3 {
		var cmd tea.Cmd
		m, cmd = update(m, keyMsg("j"))
		d := timer(t, cmd)
		assert.Equal(t, detailDebounce, d.d)
		timers = append(timers, d)
	}
	assert.Contains(t, m.detail.vp.View(), "loading…")

	var cmds []tea.Cmd
	for _, d := range timers {
		var cmd tea.Cmd
		m, cmd = update(m, d.msg)
		cmds = append(cmds, cmd)
	}
	n, last := detailLoads(cmds...)
	assert.Equal(t, 1, n, "only the last move loads")
	assert.Equal(t, m.selectedHash(), last.hash)
}

// TestReloadRefreshesDetailOnce: a reload refreshes the open detail once,
// after the log arrives, not once per WIP / tracking result.
func TestReloadRefreshesDetailOnce(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644))
	m := press(t, loaded(t, r), "enter")
	require.Equal(t, RowWIP, m.Selected().Kind)

	m, cmd := update(m, keyMsg("r"))
	var cmds []tea.Cmd
	for _, msg := range collect(cmd) {
		var next tea.Cmd
		m, next = update(m, msg)
		cmds = append(cmds, next)
	}
	n, _ := detailLoads(cmds...)
	assert.Equal(t, 1, n)
}

// TestPRsRefreshDetailWithoutReload: PRs arriving while the detail is open
// show up in it without reloading the commit.
func TestPRsRefreshDetailWithoutReload(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	require.NotContains(t, m.detail.vp.View(), "#5")
	m, cmd := update(m, prsLoadedMsg{prs: map[string]github.PR{"main": {Number: 5, Title: "t"}}})
	n, _ := detailLoads(cmd)
	assert.Equal(t, 0, n)
	assert.Contains(t, m.detail.vp.View(), "#5 t")
}

// TestTransientStatusExpires: "copied" and friends clear after statusTTL,
// unless a newer message replaced them.
func TestTransientStatusExpires(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	m, cmd := update(m, keyMsg("y"))
	assert.Equal(t, statusCopied, m.status)
	first := timer(t, cmd)
	assert.Equal(t, statusTTL, first.d)

	m, cmd = update(m, keyMsg("o"))
	assert.Equal(t, "no PR", m.status)
	second := timer(t, cmd)

	m, _ = update(m, first.msg)
	assert.Equal(t, "no PR", m.status, "an older timer does not clear a newer message")
	m, _ = update(m, second.msg)
	assert.Empty(t, m.status)
}

// TestFetchHasItsOwnSegment: a copy during a fetch does not hide "fetching ⠋".
func TestFetchHasItsOwnSegment(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m, _ = update(m, keyMsg("f"))
	m, _ = update(m, keyMsg("y"))
	assert.Contains(t, m.Status(), "fetching ⠋")
	assert.Contains(t, m.Status(), statusCopied)
}

// TestAutoFetchSkippedDuringLazygit: the tick chain stays alive but no fetch
// starts while lazygit has the terminal.
func TestAutoFetchSkippedDuringLazygit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	m := loaded(t, r).EnableAutoFetch()
	rec := &execRecorder{}
	m.execProcess = rec.exec
	m, _ = update(m, keyMsg("L"))
	require.Len(t, rec.cmds, 1)

	m, cmd := update(m, autoFetchTickMsg{})
	assert.False(t, m.fetching)
	assert.NotNil(t, cmd, "the next tick is scheduled")

	m, _ = update(m, lazygitDoneMsg{})
	m, _ = update(m, autoFetchTickMsg{})
	assert.True(t, m.fetching)
}
