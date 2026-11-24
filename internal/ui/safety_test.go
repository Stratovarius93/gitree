package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func failingHook(t *testing.T, r *testutil.Repo) {
	t.Helper()
	p := filepath.Join(r.Dir, ".git", "hooks", "post-checkout")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\necho 'git-lfs not found' >&2\nexit 1\n"), 0o755))
}

func TestCheckoutHookFailureIsReportedNotMisreported(t *testing.T) {
	r, _ := mergeRepo(t)
	failingHook(t, r)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c", "y")
	assert.Equal(t, "feature/login", r.Git("branch", "--show-current"))
	assert.Equal(t, ModeList, m.mode)
	assert.Contains(t, m.status, "post-checkout hook failed: git-lfs not found")
	assert.Equal(t, "feature/login", m.branch)
}

func TestCheckoutStashHookFailureRestoresOnNewBranch(t *testing.T) {
	r, _ := mergeRepo(t)
	failingHook(t, r)
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c", "s", "y")
	assert.Equal(t, "feature/login", r.Git("branch", "--show-current"))
	assert.Equal(t, "dirty", readFile(t, r, "a"))
	assert.Empty(t, r.Git("stash", "list"))
	assert.Equal(t, ModeList, m.mode)
	assert.Contains(t, m.status, "checked out feature/login")
	assert.Contains(t, m.status, "post-checkout hook failed")
	assert.NotContains(t, m.status, "nothing changed")
}

func TestQuitWhileActingAsksFirst(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m.acting = true
	for _, k := range []string{"q", "ctrl+c"} {
		m2, cmd := update(m, keyMsg(k))
		assert.Nil(t, cmd, k)
		require.Equal(t, ModeConfirm, m2.mode, k)
		assert.Equal(t, "An action is running. Quit anyway? Your changes may be left in the stash.", m2.confirm.text)
		m3, cmd := update(m2, keyMsg("n"))
		assert.Nil(t, cmd)
		assert.Equal(t, ModeList, m3.mode)
		_, cmd = update(m2, keyMsg("enter"))
		assert.Nil(t, cmd, "enter never confirms")
		_, cmd = update(m2, keyMsg("y"))
		require.NotNil(t, cmd)
		assert.Equal(t, tea.QuitMsg{}, cmd())
		_, cmd = update(m2, keyMsg("ctrl+c"))
		require.NotNil(t, cmd, "a second ctrl+c forces")
		assert.Equal(t, tea.QuitMsg{}, cmd())
	}
	// from another view too
	m.mode = ModeHelp
	m2, _ := update(m, keyMsg("q"))
	assert.Equal(t, ModeConfirm, m2.mode)

	m.acting, m.mode = false, ModeList
	_, cmd := update(m, keyMsg("q"))
	require.NotNil(t, cmd)
	assert.Equal(t, tea.QuitMsg{}, cmd())
}

func TestInterruptedCheckoutStashIsAnnouncedOnce(t *testing.T) {
	r, _ := mergeRepo(t)
	writeFile(t, r, "a", "lost")
	r.Git("stash", "push", "-q", "-m", "gitree: checkout feature/login")
	writeFile(t, r, "a", "other")
	r.Git("stash", "push", "-q", "-m", "unrelated")
	m := loaded(t, r)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "Changes from an interrupted checkout are in the stash")
	assert.Contains(t, v, "stash@{1}: gitree: checkout feature/login")
	assert.Contains(t, v, "L  Restore them in lazygit")
	assert.Contains(t, v, "esc close")
	m = press(t, m, "esc", "r")
	assert.Equal(t, ModeList, m.mode, "once per entry per session")
	m, _ = update(m, keyMsg("r"))
	_ = m

	m = loaded(t, r)
	m.execProcess = rec.exec
	m, _ = update(m, keyMsg("L"))
	assert.Len(t, rec.cmds, 1)
}

func TestCheckoutRestoredAlwaysOpensDialog(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m.acting = true
	plan := &checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutLocal, Name: "x"}, name: "x"}
	got, _ := m.onActionDone(actionDoneMsg{op: "checkout", plan: plan,
		err: &git.CheckoutRestoredError{Err: errors.New("checkout x: pathspec 'x' did not match")}})
	gm := got.(Model)
	require.Equal(t, ModeNotice, gm.mode)
	v := viewText(gm)
	assert.Contains(t, v, "Can't check out")
	assert.Contains(t, v, "pathspec")
	assert.Contains(t, v, "restored")
	assert.Contains(t, v, "What you can do")
}
