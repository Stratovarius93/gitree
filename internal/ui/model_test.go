package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSystem records calls instead of touching the clipboard or browser.
type fakeSystem struct {
	copied  []string
	opened  []string
	missing map[string]bool // LookPath fails for these
}

func (f *fakeSystem) Copy(text string) error   { f.copied = append(f.copied, text); return nil }
func (f *fakeSystem) OpenURL(url string) error { f.opened = append(f.opened, url); return nil }
func (f *fakeSystem) LookPath(name string) (string, error) {
	if f.missing[name] {
		return "", exec.ErrNotFound
	}
	return "/usr/bin/" + name, nil
}

// collect executes cmd synchronously and returns its messages, flattening batches.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	if d, ok := msg.(deferredMsg); ok {
		if d.d >= time.Second {
			return nil
		}
		return []tea.Msg{d.msg}
	}
	return []tea.Msg{msg}
}

// settle runs cmd and feeds every resulting message back into the model until
// no commands remain.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := collect(cmd)
	for i := 0; len(queue) > 0; i++ {
		require.Less(t, i, 1000, "message loop did not settle")
		msg := queue[0]
		queue = queue[1:]
		if _, ok := msg.(tea.QuitMsg); ok {
			continue
		}
		var next tea.Cmd
		m, next = update(m, msg)
		queue = append(queue, collect(next)...)
	}
	return m
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	nm, cmd := m.Update(msg)
	return nm.(Model), cmd
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends keys and settles the commands each one returns.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(m, keyMsg(k))
		m = settle(t, m, cmd)
	}
	return m
}

func loaded(t *testing.T, r *testutil.Repo) Model {
	t.Helper()
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	return settle(t, m, m.Init())
}

// linearRepo builds n empty commits on main ("commit 1" oldest) with a single
// git fast-import; n separate `git commit --allow-empty` calls take seconds.
func linearRepo(t *testing.T, n int) *testutil.Repo {
	t.Helper()
	r := testutil.NewRepo(t)
	var stream strings.Builder
	for i := 1; i <= n; i++ {
		msg := "commit " + strconv.Itoa(i)
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter Test <test@example.com> %d +0000\ndata %d\n%s\n", 1700000000+i, len(msg), msg)
	}
	cmd := exec.Command("git", "-C", r.Dir, "fast-import", "--quiet")
	cmd.Stdin = strings.NewReader(stream.String())
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Equal(t, strconv.Itoa(n), r.Git("rev-list", "--count", "HEAD"))
	return r
}

func TestInitialLoadShowsCommits(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first commit", map[string]string{"a.txt": "a"})
	second := r.Commit("add login screen", map[string]string{"b.txt": "b"})
	m := loaded(t, r)

	require.Len(t, m.rows, 2)
	assert.Equal(t, second, m.rows[0].Commit.Hash)
	assert.Equal(t, first, m.rows[1].Commit.Hash)
	view := m.View()
	assert.Contains(t, view, "BRANCH / TAG")
	assert.Contains(t, view, "add login screen")
	assert.Contains(t, m.Status(), "main")
	require.NotNil(t, m.Selected())
	assert.Equal(t, second, m.Selected().Commit.Hash)
}

func TestInitialLoadShowsWIPRow(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Commit("second", map[string]string{"b.txt": "b"})
	r.Git("checkout", "-q", "-b", "feature")
	writeFile(t, r, "a.txt", "changed")
	m := loaded(t, r)

	require.Len(t, m.rows, 3)
	assert.Equal(t, RowWIP, m.rows[0].Kind)
	assert.Equal(t, 1, m.rows[0].WIPCount)
	assert.Contains(t, m.View(), "1 file changed")
	assert.Contains(t, m.Status(), "feature")
}

func TestNavigation(t *testing.T) {
	m := loaded(t, linearRepo(t, 30))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 13})
	require.Len(t, m.rows, 30)

	m = press(t, m, "j", "j", "k")
	assert.Equal(t, 1, m.cursor)
	m = press(t, m, "G")
	assert.Equal(t, 29, m.cursor)
	assert.Equal(t, "commit 1", m.Selected().Commit.Subject)
	assert.Contains(t, m.View(), "commit 1")
	m = press(t, m, "g")
	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, 0, m.offset)

	half := m.listHeight() / 2
	require.Positive(t, half)
	m = press(t, m, "ctrl+d")
	assert.Equal(t, half, m.cursor)
	m = press(t, m, "ctrl+d", "ctrl+u")
	assert.Equal(t, half, m.cursor)
	m = press(t, m, "k", "k", "k", "k", "k", "k", "k", "k", "k", "k")
	assert.Equal(t, 0, m.cursor, "k stops at the top")

	// The cursor stays inside the visible window.
	m = press(t, m, "ctrl+d", "ctrl+d", "ctrl+d")
	assert.GreaterOrEqual(t, m.cursor, m.offset)
	assert.Less(t, m.cursor, m.offset+m.listHeight())
}

func TestScopeToggle(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	r.Git("checkout", "-q", "--orphan", "other")
	r.Commit("orphan only", nil)
	r.Tag("orphan-tag") // reachable only through the tag once the branch is gone
	r.Checkout("main")
	r.Git("branch", "-D", "other")
	m := loaded(t, r)
	assert.Equal(t, git.ScopeDefault, m.scope)
	assert.Len(t, m.rows, 1)
	assert.Contains(t, m.Status(), "local branches")

	next, cmd := update(m, keyMsg("a"))
	assert.Equal(t, git.ScopeAll, next.scope)
	var sawLoad bool
	for _, msg := range collect(cmd) {
		if lm, ok := msg.(logLoadedMsg); ok {
			sawLoad = true
			assert.False(t, lm.append)
			assert.Equal(t, git.ScopeAll, lm.scope)
		}
	}
	assert.True(t, sawLoad, "toggling scope issues a log load")
	m = settle(t, next, cmd)
	assert.Contains(t, m.Status(), "all branches")
	assert.Contains(t, m.View(), "all branches")
	assert.Len(t, m.rows, 2)

	m = press(t, m, "a")
	assert.Equal(t, git.ScopeDefault, m.scope)
	assert.Contains(t, m.Status(), "local branches")
	assert.Len(t, m.rows, 1)
}

func TestStaleLogIgnored(t *testing.T) {
	m := loaded(t, linearRepo(t, 3))
	stale := logLoadedMsg{gen: m.gen - 1, commits: []git.Commit{{Hash: "dead", Subject: "stale"}}}
	m, _ = update(m, stale)
	assert.Len(t, m.rows, 3)
}

func TestPagingRequestsMore(t *testing.T) {
	m := loaded(t, linearRepo(t, 350))
	require.Len(t, m.rows, PageSize)

	next, cmd := update(m, keyMsg("G"))
	var page *logLoadedMsg
	for _, msg := range collect(cmd) {
		if lm, ok := msg.(logLoadedMsg); ok {
			page = &lm
		}
	}
	require.NotNil(t, page, "G near the end requests the next page")
	assert.True(t, page.append)
	assert.Equal(t, PageSize, page.skip)
	assert.Len(t, page.commits, 50)

	m = settle(t, next, cmd)
	assert.Len(t, m.rows, 350)
	assert.Equal(t, PageSize-1, m.cursor, "cursor stays on the commit it was on")

	m = press(t, m, "G")
	assert.Equal(t, 349, m.cursor)
	assert.Equal(t, "commit 1", m.Selected().Commit.Subject)
	_, cmd = update(m, keyMsg("k"))
	for _, msg := range collect(cmd) {
		_, isLog := msg.(logLoadedMsg)
		assert.False(t, isLog, "a short page means there is nothing more to load")
	}
}

func TestEmptyRepoView(t *testing.T) {
	r := testutil.NewRepo(t)
	writeFile(t, r, "untracked.txt", "x")
	m := loaded(t, r)
	assert.Empty(t, m.rows)
	assert.Nil(t, m.Selected())
	assert.Contains(t, m.View(), "no commits")
	assert.NotContains(t, strings.ToLower(m.Status()), "error")
	m = press(t, m, "j", "k", "G", "g", "ctrl+d", "ctrl+u", "a")
	assert.Contains(t, m.View(), "no commits")
}

func TestQuit(t *testing.T) {
	m := loaded(t, linearRepo(t, 1))
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := update(m, keyMsg(k))
		require.NotNil(t, cmd, k)
		_, ok := cmd().(tea.QuitMsg)
		assert.True(t, ok, k)
	}
}

func TestResize(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	r.Git("checkout", "-q", "-b", "feature/a-rather-long-branch-name")
	r.Commit("a commit with a fairly long subject line that will not fit", nil)
	r.Tag("v1.0.0")
	m := loaded(t, r)
	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = press(t, m, "j")

	lines := strings.Split(m.View(), "\n")
	assert.LessOrEqual(t, len(lines), 10)
	for i, l := range lines {
		assert.LessOrEqual(t, lipgloss.Width(l), 40, "line %d: %q", i, l)
	}

	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 200, Height: 3}} {
		m, _ = update(m, size)
		assert.NotPanics(t, func() { _ = m.View() })
	}
}

func writeFile(t *testing.T, r *testutil.Repo, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, name), []byte(content), 0o644))
}

func TestAuthorColumnFitsVisibleNames(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Git("commit", "-q", "--allow-empty", "--author=Ana Paz <a@example.com>", "-m", "one")
	r.Git("commit", "-q", "--allow-empty", "--author=Li <l@example.com>", "-m", "two")
	m := sized(loaded(t, r), 160, 20)
	assert.Equal(t, 7, m.visibleAuthorWidth())
	header := ansi.Strip(strings.Split(m.View(), "\n")[0])
	assert.Contains(t, header, "┊ AUTHOR  ┊")
}
