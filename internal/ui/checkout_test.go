package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckoutKeyOnBranchRow(t *testing.T) {
	r, tip := mergeRepo(t)
	before := headOf(r)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))

	m = press(t, m, "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Check out feature/login?", m.confirm.text)
	m = press(t, m, "enter")
	assert.Equal(t, ModeConfirm, m.mode, "enter never confirms")
	assert.Equal(t, before, headOf(r))
	m = press(t, m, "n")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, "develop", r.Git("branch", "--show-current"))

	m = press(t, m, "c", "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, "feature/login", r.Git("branch", "--show-current"))
	assert.Equal(t, tip, headOf(r))
	assert.Equal(t, "feature/login", m.branch)
}

func TestCheckoutKeyOnCommitRowAndWIP(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(first))
	m = press(t, m, "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Contains(t, m.confirm.text, "HEAD will be detached")
	assert.Contains(t, m.confirm.text, "Check out "+first[:7]+"?")
	m = press(t, m, "y")
	assert.Equal(t, first, headOf(r))
	assert.Equal(t, "", m.branch)

	writeFile(t, r, "untracked", "u")
	m = press(t, m, "r")
	m = selectWhere(t, m, func(row Row) bool { return row.Kind == RowWIP })
	m = press(t, m, "c")
	assert.Equal(t, ModeList, m.mode)
	assert.Contains(t, m.status, "check out")
	assert.Equal(t, first, headOf(r))
}

func TestCheckoutKeyFromDetailListFocus(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "enter")
	require.Equal(t, ModeDetail, m.mode)
	m = press(t, m, "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Check out feature/login?", m.confirm.text)
}

func TestCheckoutKeyChoosesAmongSeveralBranches(t *testing.T) {
	r, _ := mergeRepo(t)
	r.Git("branch", "feature/two", "feature/login")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c")
	require.Equal(t, ModeMenu, m.mode)
	require.Len(t, m.menu.choices, 2)
	i := 1
	if m.menu.choices[1].Name == "feature/two" {
		i = 2
	}
	m = press(t, m, fmt.Sprint(i))
	require.Equal(t, ModeConfirm, m.mode, "choosing a branch goes straight to the checkout question")
	assert.Equal(t, "Check out feature/two?", m.confirm.text)
	m = press(t, m, "y")
	assert.Equal(t, "feature/two", r.Git("branch", "--show-current"))
}

func TestCheckoutKeyOnCurrentBranch(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("develop"))
	m = press(t, m, "c")
	require.Equal(t, ModeMenu, m.mode, "develop and main share the row")
	i := 1
	if m.menu.choices[1].Name == "develop" {
		i = 2
	}
	m, _ = update(m, keyMsg(fmt.Sprint(i)))
	assert.Equal(t, ModeList, m.mode)
	assert.Contains(t, m.status, "already on develop")
}

func TestCheckoutDirtyDialogOffersStash(t *testing.T) {
	r, tip := mergeRepo(t)
	writeFile(t, r, "a", "dirty")
	writeFile(t, r, "new", "staged")
	r.Git("add", "new")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))

	m = press(t, m, "c")
	require.Equal(t, ModeNotice, m.mode, "refused before asking")
	assert.Equal(t, "Can't check out", m.notice.title)
	v := viewText(m)
	assert.Contains(t, v, "uncommitted changes")
	assert.Contains(t, v, "M a")
	assert.Contains(t, v, "A new")
	assert.Contains(t, v, "What you can do")
	assert.Contains(t, v, "s  Stash changes, check out feature/login, then restore them")
	assert.Contains(t, v, "L  Open lazygit to commit or stash them")
	assert.Contains(t, v, "esc close")

	m = press(t, m, "s")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Stash 2 changed files, check out feature/login, then restore your changes?", m.confirm.text)
	m = press(t, m, "enter")
	assert.Equal(t, ModeConfirm, m.mode, "enter never confirms")
	assert.Equal(t, "develop", r.Git("branch", "--show-current"))

	m = press(t, m, "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, "feature/login", r.Git("branch", "--show-current"))
	assert.Equal(t, tip, headOf(r))
	assert.Equal(t, "dirty", readFile(t, r, "a"))
	assert.Equal(t, "new", r.Git("diff", "--cached", "--name-only"), "staged state kept")
	assert.Equal(t, "a", r.Git("diff", "--name-only"))
	assert.Empty(t, r.Git("stash", "list"))
	assert.Contains(t, m.status, "checked out feature/login")
}

func TestCheckoutDirtyDialogLazygit(t *testing.T) {
	r, _ := mergeRepo(t)
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c")
	require.Equal(t, ModeNotice, m.mode)
	m, cmd := update(m, keyMsg("L"))
	assert.Equal(t, ModeList, m.mode)
	require.NotNil(t, cmd)
	require.Len(t, rec.cmds, 1)
	assert.Equal(t, []string{"lazygit", "-p", m.repo.Root()}, rec.cmds[0].Args)
}

func TestCheckoutStashFailedCheckoutRestores(t *testing.T) {
	r, _ := mergeRepo(t)
	r.Checkout("feature/login")
	r.Commit("adds c", map[string]string{"c": "theirs"})
	r.Checkout("develop")
	writeFile(t, r, "c", "untracked mine")
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c", "s", "y")
	assert.Equal(t, "develop", r.Git("branch", "--show-current"))
	assert.Equal(t, "dirty", readFile(t, r, "a"))
	assert.Equal(t, "untracked mine", readFile(t, r, "c"))
	assert.Empty(t, r.Git("stash", "list"))
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't check out", m.notice.title)
	v := viewText(m)
	assert.Contains(t, v, "Untracked files would be overwritten")
	assert.Contains(t, v, "? c")
	assert.Contains(t, v, "restored")
	assert.Contains(t, v, "L  Open lazygit to move or remove them")
}

func TestCheckoutBlockedByUntrackedShowsDialog(t *testing.T) {
	r, _ := mergeRepo(t)
	r.Checkout("feature/login")
	r.Commit("adds c", map[string]string{"c": "theirs"})
	r.Checkout("develop")
	writeFile(t, r, "c", "untracked mine")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c", "y")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "Untracked files would be overwritten")
	assert.Contains(t, v, "? c")
	assert.Contains(t, v, "L  Open lazygit to move or remove them")
}

func TestCheckoutStashRestoreConflictOffersLazygit(t *testing.T) {
	m, r, rec := conflictModel(t)
	writeFile(t, r, "a", "mine")
	m = selectWhere(t, m, hasLabel("other"))
	m = press(t, m, "c", "s", "y")
	assert.Equal(t, "other", r.Git("branch", "--show-current"))
	assert.Contains(t, r.Git("stash", "list"), "gitree: checkout other", "changes are safe in the stash")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Checked out other, but your changes conflicted when restored. They are safe in the stash — resolve in lazygit (L)?", m.confirm.text)
	m, cmd := update(m, keyMsg("y"))
	assert.Equal(t, ModeList, m.mode)
	require.NotNil(t, cmd)
	require.Len(t, rec.cmds, 1)
}

func TestCheckoutInProgressDialog(t *testing.T) {
	m, r, rec := conflictModel(t)
	m = selectWhere(t, m, hasLabel("other"))
	m = press(t, m, "x", "m", "y", "n") // leave the merge conflict
	m = press(t, m, "c")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "in progress")
	assert.Contains(t, v, "L  Finish or abort it in lazygit")
	assert.NotContains(t, v, "Stash changes")
	m = press(t, m, "s")
	assert.Equal(t, ModeNotice, m.mode)
	m, _ = update(m, keyMsg("L"))
	assert.Len(t, rec.cmds, 1)
	assert.Contains(t, r.Git("status"), "Unmerged")
}

func TestMergeDirtyRecommendsLazygitOnly(t *testing.T) {
	r, _ := mergeRepo(t)
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "m")
	require.Equal(t, ModeNotice, m.mode, "refused before asking")
	v := viewText(m)
	assert.Contains(t, v, "L  Open lazygit to commit or stash them")
	assert.NotContains(t, v, "Stash changes")
}

func TestPullPushRefusalRecommendations(t *testing.T) {
	// no upstream → P push
	c, _ := cloneWithOrigin(t)
	c.Git("checkout", "-q", "-b", "topic")
	m := loaded(t, c)
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, viewText(m), "P  Push to create origin/topic")
	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Contains(t, m.confirm.text, "Push topic to origin/topic")
	m = press(t, m, "esc")

	// nothing to push → f fetch
	c.Git("checkout", "-q", "main")
	m = press(t, m, "r", "P")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, viewText(m), "f  Fetch to check for remote changes")
	m, cmd := update(m, keyMsg("f"))
	assert.Equal(t, ModeList, m.mode)
	assert.True(t, m.fetching)
	m = settle(t, m, cmd)

	// fetching → wait
	m.fetching = true
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, viewText(m), "Wait a moment and try again")
	m = press(t, m, "esc")
	m.fetching = false

	// detached → b / c
	c.Git("checkout", "-q", "--detach")
	m = press(t, m, "r", "P")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "b  Find a branch")
	assert.Contains(t, v, "c  Check out a branch")
	m = press(t, m, "b")
	assert.Equal(t, ModeBranchPicker, m.mode)
	m = press(t, m, "esc", "p", "c")
	assert.Contains(t, []Mode{ModeConfirm, ModeMenu}, m.mode, "c checks out the selected row")
}

func TestPullDivergedAndGoneRecommendations(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	c.Commit("local", map[string]string{"l": "1"})
	c.Git("fetch", "-q")
	m := loaded(t, c)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, viewText(m), "L  Rebase or merge in lazygit")
	m, _ = update(m, keyMsg("L"))
	assert.Len(t, rec.cmds, 1)

	c2, seed2 := cloneWithOrigin(t)
	c2.Git("checkout", "-q", "-b", "feature/x")
	c2.Commit("feat", map[string]string{"f": "1"})
	c2.Git("push", "-q", "-u", "origin", "feature/x")
	seed2.Git("push", "-q", "origin", "--delete", "feature/x")
	c2.Git("fetch", "-q", "--prune")
	m = loaded(t, c2)
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "origin/feature/x was deleted")
	assert.Contains(t, v, "P  Push to recreate origin/feature/x")
	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Contains(t, m.confirm.text, "recreate")
}

func TestPullDirtyRecommendations(t *testing.T) {
	c, _ := dirtyBehind(t, "")
	m := loaded(t, c)
	m = press(t, m, "p")
	v := viewText(m)
	assert.Contains(t, v, "s  Stash changes, pull, and restore them")
	assert.Contains(t, v, "L  Open lazygit to commit or stash them")
}

func TestRefusalRecommendationsForOtherErrors(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	for _, op := range []string{"merge", "cherry-pick"} {
		got := m.refuse(op, fmt.Errorf("cannot %s: %w", op, git.ErrDetached))
		v := viewText(got)
		assert.Contains(t, v, "b  Find a branch", op)
		assert.Contains(t, v, "c  Check out a branch", op)
	}
	got := m.refuse("checkout", git.ErrControlName)
	assert.Contains(t, viewText(got), "Use git directly")
	assert.Contains(t, viewText(got), "What you can do")

	// a refusal git reports after running (merge blocked by local changes)
	got = m.refuseWith("merge", &git.ActionError{Op: "merge", Blocked: []string{"a"}}, nil)
	assert.Equal(t, ModeNotice, got.mode)
	assert.Contains(t, viewText(got), "L  Open lazygit to commit or stash them")
}

func TestHelpListsCheckoutAndRecommendations(t *testing.T) {
	text := ansi.Strip(strings.Join(helpLines(), "\n"))
	assert.Contains(t, text, "check out branch (or commit, detached)")
	assert.Contains(t, text, "Stash changes, check out, then restore them")
	assert.Contains(t, text, "Open lazygit")
}
