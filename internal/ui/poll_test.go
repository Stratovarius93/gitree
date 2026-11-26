package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Polling is not enabled on these models, so a tick runs its check without
// scheduling the next one and settle terminates.

func TestRefsPollReloadsOnExternalCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	gen := m.gen

	r.Commit("made elsewhere", map[string]string{"b.txt": "b"})
	m, cmd := update(m, refsPollTickMsg{})
	m = settle(t, m, cmd)

	assert.Greater(t, m.gen, gen, "a changed fingerprint reloads")
	require.NotEmpty(t, m.commits)
	assert.Equal(t, "made elsewhere", m.commits[0].Subject)
}

func TestRefsPollReloadsOnExternalCheckout(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Branch("other")
	m := loaded(t, r)

	r.Checkout("other")
	m, cmd := update(m, refsPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, "other", m.branch)
}

func TestRefsPollNoChangeNoReload(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	gen := m.gen

	m, cmd := update(m, refsPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, gen, m.gen)
}

func TestRefsPollAfterOwnReloadNoSecondReload(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	r.Commit("second", map[string]string{"b.txt": "b"})
	m = press(t, m, "r") // the reload takes the new fingerprint
	gen := m.gen
	m, cmd := update(m, refsPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, gen, m.gen)
}

func TestRefsPollSkippedWhileBusy(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	base := loaded(t, r)
	r.Commit("second", map[string]string{"b.txt": "b"})

	for name, set := range map[string]func(*Model){
		"acting":   func(m *Model) { m.acting = true },
		"external": func(m *Model) { m.external = true },
		"fetching": func(m *Model) { m.fetching = true },
		"loading":  func(m *Model) { m.loading = true },
	} {
		m := base
		set(&m)
		_, cmd := update(m, refsPollTickMsg{})
		assert.Nil(t, cmd, name)
		_, cmd = update(m, wipPollTickMsg{})
		assert.Nil(t, cmd, name)
		_, cmd = update(m, tea.FocusMsg{})
		assert.Nil(t, cmd, name)
	}
}

func TestPollTicksRescheduleWhenEnabled(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r).EnablePolling()

	// idle: the check and the next tick
	_, cmd := update(m, refsPollTickMsg{})
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	assert.Len(t, batch, 2)

	// busy: only the next tick, so the chain survives
	m.acting = true
	_, cmd = update(m, refsPollTickMsg{})
	require.NotNil(t, cmd)
	_, cmd = update(m, wipPollTickMsg{})
	require.NotNil(t, cmd)
}

func TestWIPPollRebuildsOnlyOnChange(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	require.Equal(t, RowCommit, m.rows[0].Kind)

	m, cmd := update(m, wipPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, RowCommit, m.rows[0].Kind, "nothing changed")

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("edited"), 0o644))
	m, cmd = update(m, wipPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, RowWIP, m.rows[0].Kind)
}

func TestWIPUnchangedDoesNotRebuild(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	rows := m.rows
	m, _ = update(m, wipLoadedMsg{files: m.wip})
	assert.Same(t, &rows[0], &m.rows[0], "rows were not rebuilt")
}

func TestFocusChecksRefs(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	r.Commit("made elsewhere", map[string]string{"b.txt": "b"})
	m, cmd := update(m, tea.FocusMsg{})
	m = settle(t, m, cmd)
	assert.Equal(t, "made elsewhere", m.commits[0].Subject)
}

func TestRefsPollWorksInUnbornRepo(t *testing.T) {
	r := testutil.NewRepo(t)
	m := loaded(t, r)

	r.Commit("first", map[string]string{"a.txt": "a"})
	m, cmd := update(m, refsPollTickMsg{})
	m = settle(t, m, cmd)
	require.NotEmpty(t, m.commits, "the first commit shows up")
	assert.Equal(t, "first", m.commits[0].Subject)
}

func TestWIPPollRefreshesOpenWIPPreview(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a", "b.txt": "b"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("edited"), 0o644))
	m := loaded(t, r)
	require.Equal(t, RowWIP, m.rows[m.cursor].Kind)
	m = press(t, m, " ")
	require.Equal(t, ModePreviewFiles, m.mode)
	require.Len(t, m.preview.files, 1)

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "b.txt"), []byte("edited"), 0o644))
	m, cmd := update(m, wipPollTickMsg{})
	m = settle(t, m, cmd)
	assert.Len(t, m.preview.files, 2, "the open preview follows the working tree")
}

func TestWIPPollKeepsCursorAndDetail(t *testing.T) {
	r := testutil.NewRepo(t)
	head := r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m = press(t, m, "enter")
	require.Equal(t, ModeDetail, m.mode)
	require.Equal(t, head, m.selectedHash())

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("edited"), 0o644))
	m, cmd := update(m, wipPollTickMsg{})
	m = settle(t, m, cmd)
	require.Equal(t, RowWIP, m.rows[0].Kind)
	assert.Equal(t, head, m.selectedHash(), "the WIP row appearing does not steal the cursor")
	assert.Equal(t, head, m.detail.key)
}

func TestFetchReloadsOnlyWhenRefsChanged(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	gen := m.gen

	m, cmd := update(m, fetchDoneMsg{at: time.Now()})
	m = settle(t, m, cmd)
	assert.Equal(t, gen, m.gen, "a fetch that brought nothing does not reload")

	r.Commit("second", map[string]string{"b.txt": "b"})
	m, cmd = update(m, fetchDoneMsg{at: time.Now()})
	m = settle(t, m, cmd)
	assert.Greater(t, m.gen, gen)
}
