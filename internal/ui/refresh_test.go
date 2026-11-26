package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchKey(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	m, cmd := update(m, keyMsg("f"))
	assert.Contains(t, m.Status(), "fetching ⠋")
	require.NotNil(t, cmd)

	at := time.Date(2026, 1, 2, 14, 5, 0, 0, time.UTC)
	m, cmd = update(m, fetchDoneMsg{at: at})
	assert.Contains(t, m.Status(), "last fetch 14:05")
	assert.NotContains(t, m.Status(), "fetching")
	require.NotNil(t, cmd, "a successful fetch reloads")
	gen := m.gen
	assert.True(t, m.loading)
	m = settle(t, m, cmd)
	assert.Greater(t, gen, 1)
	assert.Contains(t, m.Status(), "last fetch 14:05", "the reload keeps the last-fetch time")
}

func TestFetchErrorDoesNotBlock(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Commit("second", map[string]string{"b.txt": "b"})
	m := loaded(t, r)

	m, _ = update(m, fetchDoneMsg{err: errors.New("boom")})
	assert.Contains(t, m.Status(), "boom")
	assert.Equal(t, ModeList, m.mode)
	m = press(t, m, "j")
	assert.Equal(t, 1, m.cursor)
	// fetch can be retried
	_, cmd := update(m, keyMsg("f"))
	assert.NotNil(t, cmd)
}

func TestAutoFetchTick(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r).EnableAutoFetch()

	m, cmd := update(m, autoFetchTickMsg{})
	require.NotNil(t, cmd)
	assert.Contains(t, m.Status(), "fetching ⠋")
	assert.True(t, m.fetching)
	// the batch holds the fetch and the next tick (tea.Tick would block, so
	// only check the batch size)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	assert.Len(t, batch, 2)
}

func TestFetchNotRunTwice(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m, _ = update(m, keyMsg("f"))
	_, cmd := update(m, keyMsg("f"))
	assert.Nil(t, cmd)
}

func TestReloadKeepsSelection(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	x := r.Commit("two", map[string]string{"b.txt": "b"})
	r.Commit("three", map[string]string{"c.txt": "c"})
	m := loaded(t, r)
	m = press(t, m, "j")
	require.Equal(t, x, m.selectedHash())

	r.Commit("four", map[string]string{"d.txt": "d"})
	m = press(t, m, "r")
	assert.Equal(t, x, m.selectedHash())
	assert.Equal(t, 2, m.cursor)
	assert.Len(t, m.rows, 4)
}

func TestReloadKeepsSelectionBeyondFirstPage(t *testing.T) {
	r := linearRepo(t, PageSize+20)
	m := loaded(t, r)
	m = press(t, m, "G")
	require.Len(t, m.commits, PageSize+20)
	last := m.selectedHash()

	m = press(t, m, "r")
	assert.Equal(t, last, m.selectedHash())
	assert.Len(t, m.commits, PageSize+20)
}

func TestReloadClampsWhenSelectionDisappears(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	r.Commit("two", map[string]string{"b.txt": "b"})
	m := loaded(t, r)
	m = press(t, m, "j")
	r.Git("reset", "-q", "--hard", "HEAD~1")
	m = press(t, m, "r")
	assert.Len(t, m.rows, 1)
	assert.Equal(t, 0, m.cursor)
}

func TestReloadRefreshesOpenDetailForWIP(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644))
	m := loaded(t, r)
	require.Equal(t, RowWIP, m.Selected().Kind)
	m = press(t, m, "enter")
	require.Equal(t, ModeDetail, m.mode)
	assert.NotContains(t, m.detail.vp.View(), "new-file.txt")

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "new-file.txt"), []byte("n"), 0o644))
	r.Git("add", "new-file.txt")
	m = press(t, m, "r")
	assert.Equal(t, ModeDetail, m.mode)
	assert.Contains(t, m.detail.vp.View(), "new-file.txt")
}

func TestReloadRefreshesOpenPreviewForWIP(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644))
	m := loaded(t, r)
	m = press(t, m, "space")
	require.Equal(t, ModePreviewFiles, m.mode)
	require.Len(t, m.preview.files, 1)

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "b.txt"), []byte("n"), 0o644))
	r.Git("add", "b.txt")
	m = settle(t, m, m.reloadAll()) // e.g. an auto-fetch landing while the preview is open
	assert.Equal(t, ModePreviewFiles, m.mode)
	assert.Len(t, m.preview.files, 2)
}

func TestReloadWithWIPAppearingKeepsCommitSelected(t *testing.T) {
	r := testutil.NewRepo(t)
	one := r.Commit("one", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m = press(t, m, "enter")
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644))
	m = press(t, m, "r")
	assert.Equal(t, one, m.selectedHash())
	assert.Equal(t, 1, m.cursor, "WIP row is on top, the selection stays on the commit")
	assert.Equal(t, one, m.detail.key)
}

func TestFetchKeepsSelectionWithoutCursorMove(t *testing.T) {
	origin := testutil.NewRepo(t)
	origin.Commit("first", map[string]string{"a.txt": "a"})
	cloneDir := t.TempDir()
	cmd := exec.Command("git", "clone", "-q", origin.Dir, cloneDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	repo, err := git.Open(cloneDir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	m.scope = git.ScopeAll
	m = settle(t, m, m.Init())
	before := m.selectedHash()
	require.NotEmpty(t, before)

	origin.Commit("second", map[string]string{"b.txt": "b"})
	require.NoError(t, repo.Fetch(context.Background()))
	m = settle(t, m, m.reloadAll())
	assert.Len(t, m.commits, 2)
	assert.Equal(t, before, m.selectedHash())
}

func TestFetchErrorIsSingleLine(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m.width, m.height = 250, 24
	m, _ = update(m, fetchDoneMsg{err: errors.New("fatal: Could not read from remote repository.\n\nPlease make sure\nyou have access")})
	assert.NotContains(t, m.Status(), "\n")
	assert.Contains(t, m.Status(), "error: fetch: fatal: Could not read from remote repository.")
	assert.Equal(t, m.height, len(strings.Split(m.View(), "\n")))
	// even a raw newline in a status set elsewhere cannot break the layout
	m.status = "a\nb"
	assert.Equal(t, m.height, len(strings.Split(m.View(), "\n")))
}

func TestFetchKeepsSearchCounterAndLastFetch(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("alpha", map[string]string{"a.txt": "a"})
	r.Commit("beta", map[string]string{"b.txt": "b"})
	m := loaded(t, r)
	m = press(t, m, "/", "b", "e", "t", "a", "enter")
	require.Contains(t, m.Status(), "1/1")

	m, cmd := update(m, fetchDoneMsg{at: time.Date(2026, 1, 2, 9, 7, 0, 0, time.UTC)})
	m = settle(t, m, cmd)
	m, _ = update(m, prsLoadedMsg{err: github.ErrGhMissing})
	assert.Contains(t, m.Status(), "1/1", "search counter still visible")
	assert.Contains(t, m.Status(), "last fetch 09:07")
	assert.Contains(t, m.Status(), "gh not installed: no PRs")
	// a later fetch's PR load must not erase the last-fetch time
	m, _ = update(m, prsLoadedMsg{err: github.ErrGhMissing})
	assert.Contains(t, m.Status(), "last fetch 09:07")
}

func prsMap() map[string]github.PR {
	return map[string]github.PR{"feature": {Number: 7, Title: "Add x", Head: "feature", State: github.PRApproved}}
}

func TestPRsArriveLater(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Branch("feature")
	m := loaded(t, r)
	prOf := func() *github.PR {
		for _, l := range m.rows[0].Labels {
			if l.Name == "feature" {
				return l.PR
			}
		}
		t.Fatal("no feature label")
		return nil
	}
	assert.Nil(t, prOf())

	m, _ = update(m, prsLoadedMsg{prs: prsMap()})
	require.NotNil(t, prOf())
	assert.Equal(t, 7, prOf().Number)
}

func TestPRsErrors(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	m, _ = update(m, prsLoadedMsg{err: github.ErrNoAccess})
	assert.Equal(t, "no access to PRs: check gh auth status", m.prNote)
	assert.Contains(t, m.Status(), m.prNote)
	m, _ = update(m, prsLoadedMsg{prs: prsMap()})
	assert.Empty(t, m.prNote, "a successful load clears the PR note")

	m, _ = update(m, prsLoadedMsg{err: github.ErrGhMissing})
	assert.Equal(t, "gh not installed: no PRs", m.prNote)

	m, _ = update(m, prsLoadedMsg{err: github.ErrNotGitHub})
	assert.Empty(t, m.prNote)
	assert.Empty(t, m.status)
	m, _ = update(m, prsLoadedMsg{err: errors.New("weird")})
	assert.Contains(t, m.status, "weird")
}

func TestStatusErrorClearsOnSuccessfulReload(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m, _ = update(m, wipLoadedMsg{err: errors.New("bad wip")})
	assert.Contains(t, m.status, "bad wip")
	m = press(t, m, "r")
	assert.Empty(t, m.status)
}

func TestStartupDoesNotWaitForPRs(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	block := make(chan struct{})
	defer close(block)
	m.fetchPRs = func(string) (map[string]github.PR, error) { <-block; return nil, nil }

	batch, ok := m.Init()().(tea.BatchMsg)
	require.True(t, ok)
	msgs := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func() { msgs <- c() }()
	}
	var logMsg tea.Msg
	timeout := time.After(5 * time.Second)
	for logMsg == nil {
		select {
		case msg := <-msgs:
			if _, ok := msg.(logLoadedMsg); ok {
				logMsg = msg
			}
		case <-timeout:
			t.Fatal("log load did not complete while PRs were pending")
		}
	}
	require.NotNil(t, logMsg, "log load must complete while PRs are pending")
	m, _ = update(m, logMsg)
	assert.Len(t, m.rows, 1)
	assert.False(t, strings.Contains(m.status, "error"))
}

func TestStatusStripsEscapes(t *testing.T) {
	assert.Equal(t, "error: hook said no", oneLine("error: hook \x1b[31msaid\x1b[0m\nno\x07"))
}
