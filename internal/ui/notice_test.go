package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirtyBehind is a clone one commit behind origin (fetched) whose base.txt has
// a local modification; remote is what the new commit writes to base.txt
// ("" leaves base.txt alone and adds another file).
func dirtyBehind(t *testing.T, remote string) (*testutil.Repo, string) {
	t.Helper()
	c, seed := cloneWithOrigin(t)
	files := map[string]string{"other.txt": "1"}
	if remote != "" {
		files = map[string]string{"base.txt": remote}
	}
	tip := seed.Commit("remote", files)
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	writeFile(t, c, "base.txt", "mine")
	writeFile(t, c, "untracked.txt", "u")
	return c, tip
}

func TestPullDirtyShowsDialogWithFiles(t *testing.T) {
	c, _ := dirtyBehind(t, "")
	before := headOf(c)
	m := loaded(t, c)

	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't pull", m.notice.title)
	require.Len(t, m.notice.files, 1, "tracked files only")
	assert.Equal(t, "base.txt", m.notice.files[0].Path)
	assert.Empty(t, m.status, "not a status flash")
	v := viewText(m)
	assert.Contains(t, v, "Can't pull")
	assert.Contains(t, v, "M base.txt")
	assert.NotContains(t, v, "untracked.txt")
	assert.Contains(t, v, "uncommitted changes")
	assert.Contains(t, v, "Stash changes, pull, and restore them")
	assert.Contains(t, v, "esc close")

	for _, k := range []string{"esc", "enter", "n"} {
		m = press(t, m, k)
		assert.Equal(t, ModeList, m.mode, k)
		m = press(t, m, "p")
		require.Equal(t, ModeNotice, m.mode)
	}
	assert.Equal(t, before, headOf(c))
}

func TestPullDirtyDialogListsAtMostEightFiles(t *testing.T) {
	c, seed := cloneWithOrigin(t)
	files := map[string]string{}
	for i := range 11 {
		files["f"+strconv.Itoa(i)] = "1"
	}
	seed.Commit("many", files)
	seed.Git("push", "-q")
	c.Git("pull", "-q")
	seed.Commit("two", nil)
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	for i := range 11 {
		writeFile(t, c, "f"+strconv.Itoa(i), "x")
	}
	m := loaded(t, c)
	m.height = 40
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "+3 more")
	assert.Contains(t, v, "Stash changes, pull, and restore them")
}

func TestPullAutostashFromDialog(t *testing.T) {
	c, tip := dirtyBehind(t, "")
	m := loaded(t, c)
	m = press(t, m, "p", "s")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Stash 1 changed file, pull 1 commit from origin/main, then restore your changes?", m.confirm.text)
	m = press(t, m, "enter")
	assert.Equal(t, ModeConfirm, m.mode, "enter never confirms")
	assert.NotEqual(t, tip, headOf(c))

	m = press(t, m, "y")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, tip, headOf(c))
	assert.Equal(t, "pulled 1 commit", m.status)
	assert.Equal(t, "mine", readFile(t, c, "base.txt"), "local change restored")
	assert.Equal(t, "u", readFile(t, c, "untracked.txt"))
	assert.Empty(t, c.Git("stash", "list"))

	// n on the second question does nothing
	c2, _ := dirtyBehind(t, "")
	before := headOf(c2)
	m = loaded(t, c2)
	m = press(t, m, "p", "s", "n")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(c2))
}

func TestPullAutostashRestoreConflictOffersLazygit(t *testing.T) {
	c, tip := dirtyBehind(t, "theirs")
	m := loaded(t, c)
	rec := &execRecorder{}
	m.execProcess = rec.exec
	m = press(t, m, "p", "s", "y")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Pulled, but your changes conflicted when restored. They are safe in the stash — resolve in lazygit (L)?", m.confirm.text)
	assert.Equal(t, tip, headOf(c), "pull applied")
	assert.NotEmpty(t, c.Git("stash", "list"), "changes are safe in the stash")
	assert.Contains(t, m.status, "stash")

	m, cmd := update(m, keyMsg("y"))
	assert.Equal(t, ModeList, m.mode)
	require.NotNil(t, cmd)
	require.Len(t, rec.cmds, 1)
	assert.Equal(t, []string{"lazygit", "-p", m.repo.Root()}, rec.cmds[0].Args)
	assert.Equal(t, "tea.execMsg", fmt.Sprintf("%T", cmd()))
}

func TestPullAutostashGuards(t *testing.T) {
	c, _ := dirtyBehind(t, "")
	before := headOf(c)
	m := loaded(t, c)

	m = press(t, m, "p", "s")
	require.Equal(t, ModeConfirm, m.mode)
	m.acting = true
	m = press(t, m, "y")
	assert.Equal(t, statusBusy, m.status)
	assert.Equal(t, before, headOf(c))

	m.acting = false
	m = press(t, m, "p", "s")
	require.Equal(t, ModeConfirm, m.mode)
	m.fetching = true
	m = press(t, m, "y")
	assert.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "fetch")
	assert.Equal(t, before, headOf(c))
	assert.False(t, m.acting)

	// diverged is refused before the dirty dialog, with no stash offered
	m = press(t, m, "esc")
	m.fetching = false
	c.Git("commit", "-qam", "local")
	writeFile(t, c, "base.txt", "dirty again")
	m = press(t, m, "r", "p")
	require.Equal(t, ModeNotice, m.mode)
	assert.Contains(t, m.notice.text, "diverged")
	m = press(t, m, "s")
	assert.Equal(t, ModeNotice, m.mode, "s is only offered for a dirty tree")
}

func TestPushWithDirtyTreeSucceeds(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	c.Commit("one", map[string]string{"x": "1"})
	writeFile(t, c, "base.txt", "dirty")
	m := loaded(t, c)
	m = press(t, m, "P")
	require.Equal(t, ModeConfirm, m.mode)
	m = press(t, m, "y")
	assert.Equal(t, "pushed", m.status)
	assert.Equal(t, headOf(c), c.Git("rev-parse", "origin/main"))
	assert.Equal(t, "dirty", readFile(t, c, "base.txt"))
}

func TestMergeDirtyShowsDialog(t *testing.T) {
	r, _ := mergeRepo(t)
	before := headOf(r)
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "x", "m", "y")
	require.Equal(t, ModeNotice, m.mode)
	assert.Equal(t, "Can't merge", m.notice.title)
	require.Len(t, m.notice.files, 1)
	assert.NotContains(t, viewText(m), "Stash changes", "stash is offered for pull only")
	m = press(t, m, "s")
	assert.Equal(t, ModeNotice, m.mode)
	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.Equal(t, before, headOf(r))

}

func readFile(t *testing.T, r *testutil.Repo, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir, name))
	require.NoError(t, err)
	return string(b)
}

func TestNoticeFitsShortTerminal(t *testing.T) {
	c, _ := dirtyBehind(t, "")
	for i := range 11 {
		writeFile(t, c, "n"+strconv.Itoa(i), "x")
		c.Git("add", "n"+strconv.Itoa(i))
	}
	m := loaded(t, c)
	m.height = 12
	m = press(t, m, "p")
	require.Equal(t, ModeNotice, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "uncommitted changes")
	assert.Contains(t, v, "more")
	assert.Contains(t, v, "esc close")
	assert.LessOrEqual(t, len(strings.Split(strings.TrimRight(v, "\n"), "\n")), 12)
}

func TestHelpDocumentsPullStash(t *testing.T) {
	text := strings.Join(helpLines(), "\n")
	assert.Contains(t, text, "Stash changes, pull, and restore them")
	assert.Contains(t, text, "opt-in")
}

func TestPullAutostashConfirmWarnsAboutStaged(t *testing.T) {
	c, _ := dirtyBehind(t, "")
	c.Git("add", "base.txt")
	m := loaded(t, c)
	m = press(t, m, "p", "s")
	require.Equal(t, ModeConfirm, m.mode)
	assert.Equal(t, "Stash 1 changed file, pull 1 commit from origin/main, then restore your changes (staged changes will come back unstaged)?", m.confirm.text)
}

func TestAutostashKeptOffersLazygit(t *testing.T) {
	c, _ := cloneWithOrigin(t)
	m := loaded(t, c)
	m.acting = true
	err := &git.AutostashKeptError{Cause: errors.New("timed out")}
	got, _ := m.onNetActionDone(netActionDoneMsg{op: "pull", err: err})
	gm := got.(Model)
	require.Equal(t, ModeConfirm, gm.mode)
	assert.Equal(t, "The pull did not finish. Your local changes are safe in the stash — restore them in lazygit (L)?", gm.confirm.text)
	assert.Equal(t, err.Error(), gm.status)
}
