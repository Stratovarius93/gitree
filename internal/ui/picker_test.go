package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typeText sends each rune of s as its own key press.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = press(t, m, string(r))
	}
	return m
}

func pressKey(t *testing.T, m Model, k tea.KeyType) Model {
	t.Helper()
	var cmd tea.Cmd
	m, cmd = update(m, tea.KeyMsg{Type: k})
	return settle(t, m, cmd)
}

// pickerNames are the names the picker currently lists, in order.
func pickerNames(m Model) []string {
	var out []string
	for _, i := range m.picker.hits {
		out = append(out, m.picker.refs[i].Name)
	}
	return out
}

// remoteRepo is a clone whose upstream has main, master (one commit ahead of
// the fork point, never checked out locally) and feature/Login; the clone
// only has a local main tracking origin/main, plus tags.
func remoteRepo(t *testing.T) (clone *testutil.Repo, masterTip string) {
	t.Helper()
	up := testutil.NewRepo(t)
	up.Commit("base", nil)
	up.Git("checkout", "-q", "-b", "master")
	masterTip = up.Commit("only on master", nil)
	up.Git("checkout", "-q", "-b", "feature/Login", "main")
	up.Commit("login", nil)
	up.Checkout("main")
	up.Commit("main work", nil)

	clone = testutil.NewRepo(t)
	clone.Git("remote", "add", "origin", up.Dir)
	clone.Git("fetch", "-q", "origin")
	clone.Git("checkout", "-q", "-b", "main", "origin/main")
	clone.Git("branch", "--set-upstream-to=origin/main", "main")
	clone.Commit("local work", nil)
	clone.Tag("Mark")
	clone.Git("tag", "-a", "-m", "release", "v1.0", "HEAD~1")
	return clone, masterTip
}

func TestPickerFilterOrder(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := press(t, loaded(t, clone), "b")
	require.Equal(t, ModeBranchPicker, m.mode)
	assert.Equal(t, []string{"main", "origin/feature/Login", "origin/main", "origin/master", "Mark", "v1.0"},
		pickerNames(m), "empty filter lists everything: local, remote, tag; alphabetical inside each")

	m = typeText(t, m, "MA")
	assert.Equal(t, []string{"main", "origin/main", "origin/master", "Mark"}, pickerNames(m),
		"case-insensitive substring")
	m = typeText(t, m, "ster")
	assert.Equal(t, []string{"origin/master"}, pickerNames(m))
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "Find branch")
	assert.Contains(t, view, "origin/master")
	assert.Contains(t, view, iconRemote)
}

func TestPickerShowsLoadingAndIcons(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := sized(loaded(t, clone), 100, 30)
	next, cmd := update(m, keyMsg("b"))
	require.NotNil(t, cmd, "refs load in the background")
	assert.Contains(t, ansi.Strip(next.View()), "loading ⠋")
	m = settle(t, next, cmd)
	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "loading")
	for _, icon := range []string{iconLocal, iconRemote, iconTag} {
		assert.Contains(t, view, icon)
	}
}

func TestPickerShowsPRChip(t *testing.T) {
	clone, _ := remoteRepo(t)
	repo, err := git.Open(clone.Dir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	m.fetchPRs = func(string) (map[string]github.PR, error) {
		return map[string]github.PR{"feature/Login": {Number: 42, State: github.PROpen}}, nil
	}
	m = sized(settle(t, m, m.Init()), 100, 30)
	m = typeText(t, press(t, m, "b"), "login")
	assert.Contains(t, ansi.Strip(m.View()), "#42")
}

func TestPickerTypingGoesToInput(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := press(t, loaded(t, clone), "b")
	for _, k := range []string{"j", "q", "k", "a", "b", "/", "?", "G"} {
		next, cmd := update(m, keyMsg(k))
		if cmd != nil {
			_, quit := cmd().(tea.QuitMsg)
			require.False(t, quit, "%q must not quit", k)
		}
		m = settle(t, next, cmd)
	}
	assert.Equal(t, ModeBranchPicker, m.mode)
	assert.Equal(t, "jqkab/?G", m.picker.input.Value())
	assert.Equal(t, git.ScopeDefault, m.scope, "a typed in the picker does not toggle the scope")

	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
}

func TestPickerArrowsMoveSelection(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := press(t, loaded(t, clone), "b")
	m = pressKey(t, m, tea.KeyDown)
	m = pressKey(t, m, tea.KeyCtrlN)
	assert.Equal(t, 2, m.picker.sel)
	m = pressKey(t, m, tea.KeyUp)
	assert.Equal(t, 1, m.picker.sel)
	m = pressKey(t, m, tea.KeyCtrlP)
	m = pressKey(t, m, tea.KeyCtrlP)
	assert.Equal(t, 0, m.picker.sel, "stops at the top")
	m = typeText(t, m, "o")
	assert.Equal(t, 0, m.picker.sel, "a new filter selects the first hit")
}

func TestPickerJumpToLoadedBranch(t *testing.T) {
	r := testutil.NewRepo(t)
	old := r.Commit("old", nil)
	r.Branch("release")
	r.Commit("middle", nil)
	r.Commit("newest", nil)
	m := loaded(t, r)
	require.Equal(t, 0, m.cursor)

	m = typeText(t, press(t, m, "b"), "release")
	require.Equal(t, []string{"release"}, pickerNames(m))
	m = press(t, m, "enter")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, old, m.selectedHash())
	assert.Empty(t, m.jump.hash)
}

func TestPickerFromDetailClosesIt(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", nil)
	m := press(t, sized(loaded(t, r), 120, 30), "enter")
	require.Equal(t, ModeDetail, m.mode)
	m = press(t, m, "b")
	assert.Equal(t, ModeBranchPicker, m.mode)
}

// deepBranchRepo is linearRepo(n) with branch "old" on the second commit.
func deepBranchRepo(t *testing.T, n int) (*testutil.Repo, string) {
	t.Helper()
	r := linearRepo(t, n)
	r.Git("branch", "old", "HEAD~"+strconv.Itoa(n-2))
	return r, r.Git("rev-parse", "old")
}

func TestPickerJumpLoadsMorePages(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := loaded(t, r)
	require.Len(t, m.commits, PageSize)

	m = typeText(t, press(t, m, "b"), "old")
	next, cmd := update(m, keyMsg("enter"))
	assert.Equal(t, ModeList, next.mode)
	assert.Contains(t, next.Status(), "searching old…")
	m = settle(t, next, cmd)

	assert.Equal(t, old, m.selectedHash())
	assert.Equal(t, git.ScopeDefault, m.scope)
	assert.GreaterOrEqual(t, m.cursor, m.offset)
	assert.Less(t, m.cursor, m.offset+m.listHeight(), "the selection is scrolled into view")
	assert.NotContains(t, m.Status(), "searching")
	assert.Empty(t, m.jump.hash)
	assert.Empty(t, m.pendingSelect)
}

func TestPickerJumpRemoteOnlySwitchesScope(t *testing.T) {
	clone, masterTip := remoteRepo(t)
	m := loaded(t, clone)
	for _, r := range m.rows {
		require.NotEqual(t, masterTip, r.Commit.Hash, "origin/master is not in the default view")
	}

	m = typeText(t, press(t, m, "b"), "origin/master")
	m = press(t, m, "enter")
	assert.Equal(t, git.ScopeAll, m.scope)
	assert.Equal(t, masterTip, m.selectedHash())
	assert.Contains(t, m.Status(), "switched to all branches to show origin/master")
	assert.Empty(t, m.jump.hash)
}

func TestPickerJumpUpstreamKeepsScope(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := typeText(t, press(t, loaded(t, clone), "b"), "origin/main")
	require.Equal(t, "origin/main", pickerNames(m)[0])
	m = press(t, m, "enter")
	assert.Equal(t, git.ScopeDefault, m.scope, "an upstream is part of the default view")
	assert.Equal(t, clone.Git("rev-parse", "origin/main"), m.selectedHash())
}

func TestPickerJumpTagOutsideScopeSwitches(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	r.Git("checkout", "-q", "--orphan", "other")
	orphan := r.Commit("orphan only", nil)
	r.Tag("orphan-tag")
	r.Checkout("main")
	r.Git("branch", "-D", "other")
	m := loaded(t, r)

	m = typeText(t, press(t, m, "b"), "orphan")
	m = press(t, m, "enter")
	assert.Equal(t, git.ScopeAll, m.scope, "exhausting the default view switches to all branches")
	assert.Equal(t, orphan, m.selectedHash())
}

func TestPickerJumpGivesUpAtCap(t *testing.T) {
	defer func(n int) { jumpMaxCommits = n }(jumpMaxCommits)
	jumpMaxCommits = 350
	r, old := deepBranchRepo(t, 700)
	m := typeText(t, press(t, loaded(t, r), "b"), "old")
	m = press(t, m, "enter")
	assert.NotEqual(t, old, m.selectedHash())
	assert.Contains(t, m.Status(), "could not find old")
	assert.Empty(t, m.jump.hash)
	assert.LessOrEqual(t, len(m.commits), 350)
}

func TestPickerJumpCanceledByMove(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := typeText(t, press(t, loaded(t, r), "b"), "old")
	m, pending := update(m, keyMsg("enter"))
	require.NotEmpty(t, m.jump.hash)

	m = press(t, m, "j")
	assert.Empty(t, m.jump.hash, "moving the cursor cancels the jump")
	assert.NotContains(t, m.Status(), "searching")
	m = settle(t, m, pending) // the page already requested still arrives
	assert.NotEqual(t, old, m.selectedHash())
	assert.Equal(t, 1, m.cursor)
}

func TestPickerJumpCanceledByEsc(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := typeText(t, press(t, loaded(t, r), "b"), "old")
	m, pending := update(m, keyMsg("enter"))
	m = press(t, m, "esc")
	assert.Empty(t, m.jump.hash)
	m = settle(t, m, pending)
	assert.NotEqual(t, old, m.selectedHash())
}

func TestHelpListsBranchPicker(t *testing.T) {
	assert.Contains(t, ansi.Strip(strings.Join(helpLines(), "\n")), "find branch")
}

func TestPickerViewFits(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	for i := 0; i < 30; i++ {
		r.Branch(fmt.Sprintf("br-%02d", i))
	}
	r.Branch("feature/a-rather-long-branch-name-that-does-not-fit-at-forty")
	m := sized(loaded(t, r), 40, 12)
	next, _ := update(m, keyMsg("b"))
	assertFits(t, next) // still loading
	m = press(t, m, "b")
	assertFits(t, m)
	for i := 0; i < 25; i++ {
		m = pressKey(t, m, tea.KeyDown)
	}
	assert.Equal(t, 25, m.picker.sel)
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "br-25", "the list scrolls to the selection")
	assert.NotContains(t, view, "br-00")
	assertFits(t, m)
	m = typeText(t, m, "zzz")
	assert.Contains(t, ansi.Strip(m.View()), "no results")
	assertFits(t, m)

	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 200, Height: 3}, {Width: 12, Height: 5}} {
		m, _ = update(m, size)
		assert.NotPanics(t, func() { _ = m.View() })
	}
}

func TestPickerJumpYieldsToAction(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := loaded(t, r)
	result := m.rows[5].Commit.Hash
	m = typeText(t, press(t, m, "b"), "old")
	m, pending := update(m, keyMsg("enter"))
	require.NotEmpty(t, m.jump.hash)

	m, reload := update(m, actionDoneMsg{selectHash: result})
	assert.Empty(t, m.jump.hash, "an action's result wins over a jump in progress")
	m = settle(t, m, tea.Batch(pending, reload))
	assert.Equal(t, git.ScopeDefault, m.scope)
	assert.Equal(t, result, m.selectedHash())
	assert.NotEqual(t, old, m.selectedHash())
	assert.NotContains(t, m.Status(), "could not find")
	assert.NotContains(t, m.Status(), "searching")
}

func TestPickerJumpRefreshesDetail(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := sized(loaded(t, r), 160, 30)
	m = typeText(t, press(t, m, "b"), "old")
	m, pending := update(m, keyMsg("enter"))
	m, openDetail := update(m, keyMsg("enter")) // detail opened while pages load
	require.Equal(t, ModeDetail, m.mode)
	require.NotEmpty(t, m.jump.hash)

	m = settle(t, m, tea.Batch(openDetail, pending))
	require.Equal(t, old, m.selectedHash())
	assert.Equal(t, old, m.detail.key, "the detail follows the jump")
	assert.Contains(t, m.View(), old)
}

func TestPickerJumpCapCountsJumpPages(t *testing.T) {
	defer func(n int) { jumpMaxCommits = n }(jumpMaxCommits)
	jumpMaxCommits = 350 // 300 already loaded + 350 by the jump reaches commit 600
	r, old := deepBranchRepo(t, 600)
	m := typeText(t, press(t, loaded(t, r), "b"), "old")
	m = press(t, m, "enter")
	assert.Equal(t, old, m.selectedHash())
	assert.Equal(t, git.ScopeDefault, m.scope)
}

func TestPickerJumpSurvivesFetchReload(t *testing.T) {
	r, old := deepBranchRepo(t, 700)
	m := typeText(t, press(t, loaded(t, r), "b"), "old")
	m, pending := update(m, keyMsg("enter"))
	m, reload := update(m, fetchDoneMsg{at: time.Now()}) // supersedes the page in flight
	require.NotEmpty(t, m.jump.hash)
	m = settle(t, m, tea.Batch(pending, reload))
	assert.Equal(t, old, m.selectedHash())
	assert.Equal(t, git.ScopeDefault, m.scope)
	assert.Empty(t, m.jump.hash)
}

func TestPickerIgnoresStaleRefs(t *testing.T) {
	clone, _ := remoteRepo(t)
	m := loaded(t, clone)
	m, first := update(m, keyMsg("b"))
	m = press(t, m, "esc", "b") // closed and reopened
	before := pickerNames(m)
	stale := first().(refsLoadedMsg)
	stale.refs = []git.RefInfo{{Name: "stale", Kind: git.LocalBranch, Hash: "dead"}}
	m, _ = update(m, stale)
	assert.Equal(t, before, pickerNames(m))
	assert.False(t, m.picker.loading)
}
