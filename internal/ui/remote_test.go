package ui

import (
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cloneWithOrigin returns a clone of a fresh origin (one commit on main) and
// the seed clone that can push to that origin.
func cloneWithOrigin(t *testing.T) (*testutil.Repo, *testutil.Repo) {
	t.Helper()
	bare, seed := testutil.NewOrigin(t)
	return testutil.Clone(t, bare), seed
}

func TestPullConfirmsThenFastForwards(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	seed.Commit("two", map[string]string{"b": "1"})
	tip := seed.Commit("three", map[string]string{"c": "1"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	before := headOf(c)
	m := loaded(t, c)
	m = selectWhere(t, m, func(r Row) bool { return r.Commit != nil && r.Commit.Hash == before })

	m = press(t, m, "p")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Pull 2 commits from origin/main into main?", m.confirm.text)
	m = press(t, m, "enter")
	assert.Equal(t, ModeConfirm, m.mode, "enter never confirms")
	assert.Equal(t, before, headOf(c))

	m = press(t, m, "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, tip, headOf(c))
	assert.Equal(t, "pulled 2 commits", m.status)
	assert.Equal(t, before, m.selectedHash(), "keeps the selection")
	assert.False(t, m.acting)
	assert.Equal(t, 0, m.track["main"].Behind, "reloaded")
}

func TestPullWithNothingKnown(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	m := loaded(t, c)
	m = press(t, m, "p")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Pull from origin/main into main (no new commits known — fetch with f)?", m.confirm.text)
	m = press(t, m, "y")
	assert.Equal(t, "already up to date", m.status)
}

func TestPullDivergedIsRefused(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	local := c.Commit("local", map[string]string{"l": "1"})
	m := loaded(t, c)
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't pull", m.notice.title)
	assert.Contains(t, m.notice.text, "Branches diverged")
	assert.Contains(t, viewText(m), "L  Rebase or merge in lazygit")
	assert.Equal(t, local, headOf(c))
}

func TestPushConfirmsThenPushes(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Commit("one", map[string]string{"x": "1"})
	c.Commit("two", map[string]string{"y": "1"})
	m := loaded(t, c)

	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Push 2 commits to origin/main?", m.confirm.text)
	m = press(t, m, "enter")
	assert.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, 2, m.track["main"].Ahead)

	m = press(t, m, "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, "pushed", m.status)
	assert.Equal(t, 0, m.track["main"].Ahead, "reloaded")
	assert.Equal(t, headOf(c), c.Git("rev-parse", "origin/main"))
}

func TestPushNewBranchSetsUpstream(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Git("checkout", "-q", "-b", "feature/x")
	c.Commit("feat", map[string]string{"f": "1"})
	m := loaded(t, c)

	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Push feature/x to origin/feature/x (new branch) and set upstream?", m.confirm.text)
	m = press(t, m, "y")
	assert.Equal(t, "pushed", m.status)
	assert.Equal(t, "origin/feature/x", m.track["feature/x"].Upstream)
}

func TestPushNothingToPush(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	m := loaded(t, c)
	m = press(t, m, "P")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't push", m.notice.title)
	assert.Contains(t, m.notice.text, "Nothing to push")
}

func TestPullPushRefusedWhenDetachedOrNoUpstream(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Git("checkout", "-q", "--detach")
	m := loaded(t, c)
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "detached")
	m = press(t, m, "esc", "P")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't push", m.notice.title)
	assert.Contains(t, m.notice.text, "detached")

	c.Git("checkout", "-q", "-b", "topic")
	m = press(t, m, "esc", "r", "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "No upstream: push first with P")
}

func TestPullPushRefusedWhileActing(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	seed.Commit("two", nil)
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	m := loaded(t, c)
	before := headOf(c)

	m = press(t, m, "p")
	require.Equal(t, ModeConfirm, m.mode)
	m.acting = true // another action is running
	m = press(t, m, "y")
	assert.Equal(t, statusBusy, m.status)
	assert.Equal(t, before, headOf(c))

	for _, k := range []string{"p", "P"} {
		m = press(t, m, k)
		assert.Equal(t, ModeList, m.mode, k)
		assert.Equal(t, statusBusy, m.status, k)
	}
}

func TestPullPushWorkFromDetailListFocus(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Commit("one", nil)
	m := loaded(t, c)
	m = press(t, m, "enter")
	require.Equal(t, ModeDetail, m.mode)
	m = press(t, m, "P")
	assert.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Push 1 commit to origin/main?", m.confirm.text)
}

func TestHelpListsPullAndPush(t *testing.T) {
	var found []string
	for _, b := range keys.helpBindings() {
		found = append(found, b.Help().Key+" "+b.Help().Desc)
	}
	assert.Contains(t, found, "p pull (fast-forward only)")
	assert.Contains(t, found, "P push")
}

func TestPushBranchTrackingOtherNameNeverHitsThatBranch(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	mainTip := c.Git("rev-parse", "origin/main")
	c.Git("checkout", "-q", "-b", "my-feature", "origin/main")
	tip := c.Commit("feat", map[string]string{"f": "1"})
	m := loaded(t, c)

	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Push my-feature to origin/my-feature (new branch) and set upstream?", m.confirm.text)
	m = press(t, m, "y")
	assert.Equal(t, "pushed", m.status)
	c.Git("fetch", "-q")
	assert.Equal(t, mainTip, c.Git("rev-parse", "origin/main"), "remote main unchanged")
	assert.Equal(t, tip, c.Git("rev-parse", "origin/my-feature"))
	assert.Equal(t, "origin/my-feature", m.track["my-feature"].Upstream)
}

func TestPushRefusedWhenBranchChangedAfterConfirm(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Commit("one", nil)
	before := c.Git("rev-parse", "origin/main")
	m := loaded(t, c)
	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	c.Git("checkout", "-q", "-b", "other")
	m = press(t, m, "y")
	assert.Equal(t, "branch changed — try again", m.status)
	c.Git("fetch", "-q")
	assert.Equal(t, before, c.Git("rev-parse", "origin/main"))
}

func TestGoneUpstreamPullRefusedPushRecreates(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	c.Git("checkout", "-q", "-b", "feature/x")
	c.Commit("feat", map[string]string{"f": "1"})
	c.Git("push", "-q", "-u", "origin", "feature/x")
	seed.Git("push", "-q", "origin", "--delete", "feature/x")
	c.Git("fetch", "-q", "--prune")
	m := loaded(t, c)
	require.True(t, m.track["feature/x"].Gone)

	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "Upstream origin/feature/x no longer exists")

	m = press(t, m, "esc", "P")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Push feature/x to origin/feature/x (remote branch was deleted — recreate it)?", m.confirm.text)
	m = press(t, m, "y")
	assert.Equal(t, "pushed", m.status)
	assert.False(t, m.track["feature/x"].Gone)
	assert.Equal(t, headOf(c), c.Git("rev-parse", "origin/feature/x"))
}

func TestPullPushRefusedWhileFetching(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	seed.Commit("two", nil)
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	m := loaded(t, c)
	before := headOf(c)

	m.fetching = true
	for _, k := range []string{"p", "P"} {
		m = press(t, m, k)
		require.Equal(t, ModeNotice, m.mode, k)
		assert.Contains(t, m.notice.text, "fetch is in progress", k)
		m = press(t, m, "esc")
	}

	m.fetching = false
	m = press(t, m, "p")
	require.Equal(t, ModeConfirm, m.mode)
	m.fetching = true // a fetch started while the question was open
	m = press(t, m, "y")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "fetch is in progress")
	assert.Equal(t, before, headOf(c))
	assert.False(t, m.acting)
	assert.Equal(t, "fetch in progress — try again", statusFetching)
}

func TestFetchSkippedWhileActing(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	m := loaded(t, c).EnableAutoFetch()
	m.acting = true

	m, _ = update(m, keyMsg("f"))
	assert.False(t, m.fetching, "f does not fetch during an action")
	assert.Equal(t, statusBusy, m.status)

	m, cmd := update(m, autoFetchTickMsg{})
	assert.False(t, m.fetching, "the tick skips the fetch")
	require.NotNil(t, cmd, "but keeps the tick chain alive")

	m.acting = false
	m, _ = update(m, autoFetchTickMsg{})
	assert.True(t, m.fetching, "the next tick fetches again")
}

func TestKnownRemoteErrorsAreShownPlain(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	for _, err := range []error{git.GoneError("origin/x"), git.ErrControlName} {
		m := loaded(t, c)
		m.acting = true
		got, _ := m.onNetActionDone(netActionDoneMsg{op: "pull", err: err})
		require.Equal(t, ModeNotice, got.(Model).mode, "refused before git ran: a dialog")
		assert.Contains(t, strings.ToLower(got.(Model).notice.text), strings.ToLower(err.Error()))

		m.mode = ModeHelp // another view is open: the reason goes to the status bar
		got, _ = m.onNetActionDone(netActionDoneMsg{op: "pull", err: err})
		assert.Equal(t, err.Error(), got.(Model).status)
	}
}

func TestCheckBranchRefusesControlNameBranch(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Git("branch", "ab")
	c.Git("checkout", "-q", "-b", "a\u0085b")
	m := loaded(t, c)
	assert.Equal(t, "ab", m.branch, "display name is sanitized")
	assert.ErrorIs(t, checkBranch(m.repo, "ab"), git.ErrControlName)
	c.Git("checkout", "-q", "ab")
	assert.NoError(t, checkBranch(m.repo, "ab"))
}
