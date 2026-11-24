package ui

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergeRepo: main ← develop (current) and feature/login one commit ahead.
func mergeRepo(t *testing.T) (*testutil.Repo, string) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("develop")
	r.Git("checkout", "-q", "-b", "feature/login")
	tip := r.Commit("login form", map[string]string{"login": "1"})
	r.Checkout("develop")
	return r, tip
}

func headOf(r *testutil.Repo) string { return r.Git("rev-parse", "HEAD") }

// execRecorder replaces the model's tea.ExecProcess and records the commands.
type execRecorder struct{ cmds []*exec.Cmd }

func (e *execRecorder) exec(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	e.cmds = append(e.cmds, c)
	return tea.ExecProcess(c, fn)
}

func TestConfirmBeforeMerge(t *testing.T) {
	r, tip := mergeRepo(t)
	before := headOf(r)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))

	m = press(t, m, "x", "m")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Merge feature/login → develop?", m.confirm.text)
	assert.Contains(t, viewText(m), "Merge feature/login → develop?")

	m = press(t, m, "n")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(r))
	m = press(t, m, "x", "m", "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(r))

	m = press(t, m, "x", "m", "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, tip, headOf(r), "fast-forward merge")
	assert.Equal(t, tip, m.selectedHash(), "selects the result")
	assert.Equal(t, "develop", m.branch)

	// enter never confirms: a double enter (menu + dialog) must not act
	r.Git("reset", "-q", "--hard", before)
	m = press(t, m, "r")
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "enter", "enter")
	assert.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, before, headOf(r))
	assert.Contains(t, viewText(m), "y yes · n/esc no")
	assert.NotContains(t, viewText(m), "enter")
	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(r))
}

func TestDirtyShowsWarning(t *testing.T) {
	r, _ := mergeRepo(t)
	before := headOf(r)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Check out feature/login?", m.confirm.text)
	writeFile(t, r, "a", "dirty") // changed while the question was open
	m = press(t, m, "y")
	require.Equal(t, ModeNotice, m.mode, "a dialog, not a status flash")
	assert.Equal(t, "Can't check out", m.notice.title)
	require.Len(t, m.notice.files, 1)
	assert.Equal(t, "a", m.notice.files[0].Path)
	assert.Contains(t, viewText(m), "uncommitted changes")
	assert.Contains(t, viewText(m), "s  Stash changes, check out feature/login, then restore them")
	m = press(t, m, "enter")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(r))
	assert.Equal(t, "develop", r.Git("branch", "--show-current"))
}

func TestInProgressShowsWarning(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "base"})
	r.Branch("other")
	r.Checkout("other")
	r.Commit("other change", map[string]string{"a": "other"})
	r.Checkout("main")
	r.Commit("main change", map[string]string{"a": "main"})
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)
	require.Error(t, repo.Merge("other"))

	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("other"))
	m = press(t, m, "x", "c", "y")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't check out", m.notice.title)
	assert.Contains(t, m.notice.text, "in progress")
	assert.Equal(t, "main", r.Git("branch", "--show-current"))
}

func conflictModel(t *testing.T) (Model, *testutil.Repo, *execRecorder) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "base"})
	r.Branch("other")
	r.Checkout("other")
	r.Commit("other change", map[string]string{"a": "other"})
	r.Checkout("main")
	r.Commit("main change", map[string]string{"a": "main"})
	m := loaded(t, r)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	return m, r, rec
}

func TestConflictOffersLazygit(t *testing.T) {
	m, r, rec := conflictModel(t)
	m = selectWhere(t, m, hasLabel("other"))
	m = press(t, m, "x", "m", "y")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Conflict in merge. Open lazygit to resolve it?", m.confirm.text)
	assert.Contains(t, m.status, "CONFLICT")

	m, cmd := update(m, keyMsg("y"))
	assert.Equal(t, ModeList, m.mode)
	require.NotNil(t, cmd)
	require.Len(t, rec.cmds, 1)
	assert.Equal(t, []string{"lazygit", "-p", m.repo.Root()}, rec.cmds[0].Args)
	assert.Equal(t, "tea.execMsg", fmt.Sprintf("%T", cmd()))
	assert.Contains(t, r.Git("status"), "Unmerged", "the conflict is left for lazygit")
}

func TestConflictDeclinedLeavesRepo(t *testing.T) {
	m, r, rec := conflictModel(t)
	m = selectWhere(t, m, hasLabel("other"))
	m = press(t, m, "x", "m", "y", "n")
	assert.Equal(t, ModeList, m.mode)
	assert.Empty(t, rec.cmds)
	assert.Contains(t, r.Git("status"), "Unmerged", "conflict is not aborted")
}

func TestCherryPickConfirmAndSelect(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Git("checkout", "-q", "-b", "topic")
	pick := r.Commit("picked", map[string]string{"p": "1"})
	r.Commit("topic tip", map[string]string{"q": "1"})
	r.Checkout("main")
	r.Commit("main diverges", map[string]string{"m": "1"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(pick))
	m = press(t, m, "x", "p")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Cherry-pick "+pick[:7]+" → main?", m.confirm.text)
	m = press(t, m, "y")
	newHead := headOf(r)
	assert.NotEqual(t, pick, newHead)
	assert.Equal(t, "picked", r.Git("log", "-1", "--format=%s"))
	assert.Equal(t, newHead, m.selectedHash())
}

func TestCheckoutCommitWarnsDetached(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(first))
	m = press(t, m, "x", "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Contains(t, m.confirm.text, "Check out "+first[:7]+"?")
	assert.Contains(t, m.confirm.text, "HEAD will be detached")
	m = press(t, m, "y")
	assert.Equal(t, first, headOf(r))
	assert.Equal(t, "", m.branch)
	assert.Equal(t, first, m.selectedHash())
	// detached: the commit menu has no cherry-pick
	m = press(t, m, "x")
	assert.Equal(t, []string{"c", "b", "y", "Y", "M"}, itemKeys(m.menu.items))
}

func TestCheckoutRemoteBranch(t *testing.T) {
	up := testutil.NewRepo(t)
	up.Commit("base", map[string]string{"a": "1"})
	up.Git("checkout", "-q", "-b", "x")
	tip := up.Commit("x work", map[string]string{"x": "1"})
	up.Checkout("main")
	clone := testutil.NewRepo(t)
	clone.Git("remote", "add", "origin", up.Dir)
	clone.Git("fetch", "-q", "origin")
	clone.Git("checkout", "-q", "-b", "main", "origin/main")
	m := loaded(t, clone)
	m = press(t, m, "a")
	m = selectWhere(t, m, hasLabel("origin/x"))
	m = press(t, m, "x", "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Contains(t, m.confirm.text, "Check out origin/x as a new local branch x tracking it?")
	m = press(t, m, "y")
	assert.Equal(t, "x", clone.Git("branch", "--show-current"))
	assert.Equal(t, tip, m.selectedHash())
}

func TestCreateBranchFlow(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(first))

	m = press(t, m, "x", "b")
	require.Equal(t, ModeInput, m.mode)
	m = press(t, m, "b", "a", "d", ".", ".", "x", "enter")
	assert.Equal(t, ModeInput, m.mode, "invalid name keeps the input open")
	assert.Contains(t, viewText(m), "invalid branch name")
	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)

	m = press(t, m, "x", "b")
	for _, c := range "feature/new" {
		m = press(t, m, string(c))
	}
	m = press(t, m, "enter")
	assert.Equal(t, first, r.Git("rev-parse", "feature/new"))
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Check out feature/new?", m.confirm.text)
	assert.Equal(t, first, m.selectedHash(), "selects the new branch")
	assert.True(t, hasLabel("feature/new")(*m.Selected()))

	m = press(t, m, "y")
	assert.Equal(t, "feature/new", r.Git("branch", "--show-current"))
	assert.Equal(t, first, m.selectedHash())
}

func TestCreateBranchDeclineCheckout(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a": "1"})
	m := loaded(t, r)
	m = press(t, m, "x", "b", "t", "o", "p", "enter", "n")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, "main", r.Git("branch", "--show-current"))
	r.Git("rev-parse", "top")
}

func TestCopyAndOpen(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first\n\nwith a body", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	r.Git("checkout", "-q", "-b", "feature")
	r.Commit("third", map[string]string{"a": "3"})
	r.Checkout("main")
	m := loaded(t, r)
	url := "https://github.com/o/r/pull/7"
	m, _ = update(m, prsLoadedMsg{prs: map[string]github.PR{"feature": {Number: 7, URL: url, Head: "feature"}}})
	fs := m.sys.(*fakeSystem)

	m = selectWhere(t, m, hasLabel("feature"))
	m = press(t, m, "y")
	assert.Equal(t, []string{"feature"}, fs.copied)
	assert.Equal(t, "copied", m.status)

	m = selectWhere(t, m, atHash(first))
	m = press(t, m, "y")
	assert.Equal(t, first, fs.copied[1])
	m = press(t, m, "x", "Y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, first[:7], fs.copied[2])
	m = press(t, m, "x", "M")
	assert.Equal(t, "first\n\nwith a body", fs.copied[3])
	m = press(t, m, "o")
	assert.Empty(t, fs.opened, "no PR on this row")

	m = selectWhere(t, m, hasLabel("feature"))
	m = press(t, m, "x", "u")
	assert.Equal(t, url, fs.copied[4])
	m = press(t, m, "x", "h")
	assert.Len(t, m.selectedHash(), 40)
	assert.Equal(t, m.selectedHash(), fs.copied[5])
	m = press(t, m, "x", "y")
	assert.Equal(t, "feature", fs.copied[6])
	m = press(t, m, "x", "o")
	assert.Equal(t, []string{url}, fs.opened)
	m = press(t, m, "o")
	assert.Equal(t, []string{url, url}, fs.opened)
	assert.Equal(t, ModeList, m.mode)
}

func TestLazygitKey(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a": "1"})
	m := loaded(t, r)
	rec := &execRecorder{}
	m.execProcess = rec.exec

	m, cmd := update(m, keyMsg("L"))
	require.NotNil(t, cmd)
	require.Len(t, rec.cmds, 1)
	assert.Equal(t, []string{"lazygit", "-p", m.repo.Root()}, rec.cmds[0].Args)
	assert.Equal(t, "tea.execMsg", fmt.Sprintf("%T", cmd()))

	gen := m.gen
	m, cmd = update(m, lazygitDoneMsg{})
	require.NotNil(t, cmd, "returning from lazygit reloads")
	assert.Greater(t, m.gen, gen)
	m = settle(t, m, cmd)
	assert.True(t, m.loaded)

	m.sys.(*fakeSystem).missing = map[string]bool{"lazygit": true}
	m, _ = update(m, keyMsg("L"))
	assert.Len(t, rec.cmds, 1)
	assert.Equal(t, "lazygit is not installed", m.status)
}

func TestLazygitCmd(t *testing.T) {
	assert.Equal(t, []string{"lazygit", "-p", "/some/repo"}, lazygitCmd("/some/repo").Args)
	// The default runner is tea.ExecProcess.
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a": "1"})
	m := loaded(t, r)
	_, cmd := update(m, keyMsg("L"))
	require.NotNil(t, cmd)
	assert.Equal(t, "tea.execMsg", fmt.Sprintf("%T", cmd()))
}

func TestActionInFlightRefusesKeys(t *testing.T) {
	r, tip := mergeRepo(t)
	m := loaded(t, r)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "m")
	m, cmd := update(m, keyMsg("y")) // dispatched, not finished
	require.NotNil(t, cmd)
	assert.True(t, m.acting)

	m = press(t, m, "x")
	assert.Equal(t, ModeList, m.mode, "no menu while an action runs")
	assert.Equal(t, "action in progress…", m.status)
	m = press(t, m, "L")
	assert.Empty(t, rec.cmds, "no lazygit while an action runs")

	ran := false
	m.confirm = confirmState{text: "x", yes: func(m Model) (Model, tea.Cmd) { ran = true; return m, nil }}
	m.mode = ModeConfirm
	m = press(t, m, "y")
	assert.False(t, ran, "no confirm while an action runs")
	m.mode = ModeList

	m = settle(t, m, cmd)
	assert.False(t, m.acting)
	assert.Equal(t, tip, headOf(r))
	m = press(t, m, "x")
	assert.Equal(t, ModeMenu, m.mode)
}

func TestFollowUpConfirmNotForced(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a": "1"})
	m := loaded(t, r)
	m = press(t, m, "x", "b", "t", "o", "p")
	m, cmd := update(m, keyMsg("enter"))
	m = press(t, m, "?")
	require.Equal(t, ModeHelp, m.mode)
	m = settle(t, m, cmd)
	assert.Equal(t, ModeHelp, m.mode, "help stays open")
	assert.Equal(t, "branch top created", m.status)
	m = press(t, m, "esc", "y") // a later y is a plain copy, not a hidden checkout
	assert.Equal(t, "main", r.Git("branch", "--show-current"))

	cm, cr, rec := conflictModel(t)
	cm = selectWhere(t, cm, hasLabel("other"))
	cm = press(t, cm, "x", "m")
	cm, cmd = update(cm, keyMsg("y"))
	cm = press(t, cm, "enter")
	require.Equal(t, ModeDetail, cm.mode)
	cm = settle(t, cm, cmd)
	assert.Equal(t, ModeDetail, cm.mode, "detail stays open")
	assert.Contains(t, cm.status, "conflict in merge: resolve it in lazygit (L)")
	assert.Empty(t, rec.cmds)
	assert.Contains(t, cr.Git("status"), "Unmerged")
}

func TestStaleBranchMergeRefused(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "m")
	require.Equal(t, "Merge feature/login → develop?", m.confirm.text)
	r.Checkout("main") // the current branch changes behind gitree's back
	before := headOf(r)
	m = press(t, m, "y")
	assert.Contains(t, m.status, "current branch is main, not develop")
	assert.Equal(t, before, headOf(r))
	assert.Equal(t, "main", r.Git("branch", "--show-current"))
}

func TestStaleBranchCherryPickRefused(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Git("checkout", "-q", "-b", "topic")
	pick := r.Commit("picked", map[string]string{"p": "1"})
	r.Commit("topic tip", map[string]string{"q": "1"})
	r.Checkout("main")
	r.Commit("main diverges", map[string]string{"m": "1"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(pick))
	m = press(t, m, "x", "p")
	r.Checkout("topic")
	before := headOf(r)
	m = press(t, m, "y")
	assert.Contains(t, m.status, "current branch is topic, not main")
	assert.Equal(t, before, headOf(r))
}

func TestCheckoutRemoteWithLocalCopy(t *testing.T) {
	up := testutil.NewRepo(t)
	up.Commit("base", map[string]string{"a": "1"})
	up.Branch("x")
	clone := testutil.NewRepo(t)
	clone.Git("remote", "add", "origin", up.Dir)
	clone.Git("fetch", "-q", "origin")
	clone.Git("checkout", "-q", "-b", "main", "origin/main")
	clone.Git("branch", "--track", "mine", "origin/x")
	local := clone.Git("rev-parse", "mine")
	up.Checkout("x")
	up.Commit("x moves", map[string]string{"x": "1"})
	clone.Git("fetch", "-q", "origin") // origin/x is now ahead of mine

	m := loaded(t, clone)
	m = selectWhere(t, m, hasLabel("origin/x"))
	m = press(t, m, "x", "c")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Check out mine (tracks origin/x)?", m.confirm.text)
	m = press(t, m, "y")
	assert.Empty(t, m.status)
	assert.Equal(t, "mine", clone.Git("branch", "--show-current"))
	assert.Equal(t, local, headOf(clone), "the local copy, not a new branch")
	assert.Equal(t, local, m.selectedHash())
}

func TestCreateBranchExisting(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a": "1"})
	m := loaded(t, r)
	m = press(t, m, "x", "b", "m", "a", "i", "n", "enter")
	assert.Equal(t, ModeInput, m.mode)
	assert.Contains(t, viewText(m), "branch already exists")
}

func TestEmptyCherryPickOffersLazygit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Git("checkout", "-q", "-b", "topic")
	pick := r.Commit("change", map[string]string{"a": "2"})
	r.Commit("topic tip", map[string]string{"b": "1"})
	r.Checkout("main")
	r.Commit("same change", map[string]string{"a": "2"})
	m := loaded(t, r)
	m = selectWhere(t, m, atHash(pick))
	m = press(t, m, "x", "p", "y")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Operation in progress", m.confirm.title)
	assert.Equal(t, "cherry-pick is unfinished. Open lazygit to finish it?", m.confirm.text)
}
