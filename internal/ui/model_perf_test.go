package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

// mergeHeavyRepo builds about n commits with one git fast-import: up to sixteen
// side branches fork from main, collect commits in turn and are merged back
// into main, so the graph keeps several lanes and a merge every few commits.
func mergeHeavyRepo(t testing.TB, n int) string {
	t.Helper()
	dir := t.TempDir()
	run := func(stdin string, args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("", "init", "-q", "-b", "main")

	var s strings.Builder
	mark, mainTip := 0, 0
	var sides []int // tip mark of each open side branch
	commit := func(ref string, from int, merge int, msg string) int {
		mark++
		fmt.Fprintf(&s, "commit %s\nmark :%d\ncommitter Test <t@example.com> %d +0000\ndata %d\n%s\n", ref, mark, 1700000000+mark, len(msg), msg)
		if from > 0 {
			fmt.Fprintf(&s, "from :%d\n", from)
		}
		if merge > 0 {
			fmt.Fprintf(&s, "merge :%d\n", merge)
		}
		return mark
	}
	mainTip = commit("refs/heads/main", 0, 0, "root")
	for i := 0; mark < n; i++ {
		switch {
		case i%3 == 0 && len(sides) < 16:
			sides = append(sides, commit(fmt.Sprintf("refs/heads/side%d", i), mainTip, 0, fmt.Sprintf("side start %d", i)))
		case i%3 == 1 && len(sides) > 0:
			k := i % len(sides)
			sides[k] = commit(fmt.Sprintf("refs/heads/side-work%d", i), sides[k], 0, fmt.Sprintf("side work %d", i))
		case i%11 == 0 && len(sides) > 0:
			mainTip = commit("refs/heads/main", mainTip, sides[0], fmt.Sprintf("merge %d", i))
			sides = sides[1:]
		default:
			mainTip = commit("refs/heads/main", mainTip, 0, fmt.Sprintf("main %d", i))
		}
	}
	run(s.String(), "fast-import", "--quiet", "--force")
	return dir
}

// largeModel returns a model with every commit of a merge-heavy repo loaded.
func largeModel(t testing.TB, n int) Model {
	t.Helper()
	repo, err := git.Open(mergeHeavyRepo(t, n))
	require.NoError(t, err)
	commits, err := repo.Log(git.ScopeAll, 0, n+10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(commits), n)
	m := New(repo, &fakeSystem{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 50})
	m, _ = update(m, logLoadedMsg{gen: m.gen, scope: m.scope, commits: commits})
	m, _ = update(m, trackLoadedMsg{track: map[string]git.Track{}})
	require.Equal(t, len(commits), len(m.rows))
	return m
}

// TestNavigationStaysFast guards against per-keystroke work that grows with
// the number of loaded commits: one `j` (Update + View) must fit in a frame.
func TestNavigationStaysFast(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test")
	}
	m := largeModel(t, 3000)
	_ = m.View()
	const moves = 100
	start := time.Now()
	for range moves {
		m, _ = update(m, keyMsg("j"))
		_ = m.View()
	}
	avg := time.Since(start) / moves
	t.Logf("average j + View over %d loaded commits: %v", len(m.rows), avg)
	require.Less(t, avg, 10*time.Millisecond)
}

func BenchmarkNavigate3000(b *testing.B) {
	m := largeModel(b, 3000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := "j"
		if (i/500)%2 == 1 {
			k = "k"
		}
		m, _ = update(m, keyMsg(k))
		_ = m.View()
	}
}

// TestInitialSelectionIndependentOfLoadOrder: until the user moves, row 0
// is selected whether the log or the WIP status arrives first.
func TestInitialSelectionIndependentOfLoadOrder(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Commit("second", map[string]string{"b.txt": "b"})
	writeFile(t, r, "a.txt", "changed")
	repo, err := git.Open(r.Dir)
	require.NoError(t, err)

	var logMsg logLoadedMsg
	var wipMsg wipLoadedMsg
	base := New(repo, &fakeSystem{})
	for _, msg := range collect(base.Init()) {
		switch msg := msg.(type) {
		case logLoadedMsg:
			logMsg = msg
		case wipLoadedMsg:
			wipMsg = msg
		}
	}
	require.Len(t, wipMsg.files, 1)

	for name, order := range map[string][]tea.Msg{
		"log then wip": {logMsg, wipMsg},
		"wip then log": {wipMsg, logMsg},
	} {
		m := base
		for _, msg := range order {
			m, _ = update(m, msg)
		}
		require.Len(t, m.rows, 3, name)
		require.Equal(t, 0, m.cursor, name)
		require.Equal(t, RowWIP, m.Selected().Kind, name)
	}

	// Once the user has moved, a late WIP row keeps the cursor on its commit.
	m := base
	m, _ = update(m, logMsg)
	m = press(t, m, "j")
	sel := m.Selected().Commit.Hash
	m, _ = update(m, wipMsg)
	require.Equal(t, sel, m.Selected().Commit.Hash)
}

// BenchmarkNavigateRepo measures j/k on a real repository; it runs only with
// GITREE_BENCH_REPO set (GITREE_BENCH_COMMITS loaded commits, default 3000).
func BenchmarkNavigateRepo(b *testing.B) {
	path := os.Getenv("GITREE_BENCH_REPO")
	if path == "" {
		b.Skip("set GITREE_BENCH_REPO")
	}
	n := 3000
	if v := os.Getenv("GITREE_BENCH_COMMITS"); v != "" {
		_, _ = fmt.Sscan(v, &n)
	}
	repo, err := git.Open(path)
	require.NoError(b, err)
	commits, err := repo.Log(git.ScopeDefault, 0, n)
	require.NoError(b, err)
	m := New(repo, &fakeSystem{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 50})
	m, _ = update(m, logLoadedMsg{gen: m.gen, scope: m.scope, commits: commits})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := "j"
		if (i/500)%2 == 1 {
			k = "k"
		}
		m, _ = update(m, keyMsg(k))
		_ = m.View()
	}
}

// TestGraphHighlightFollowsCursor: the windowed graph rendering in View must
// match a full BuildRows with the selected hash, for every cursor position
// (including the WIP row and scrolled windows).
func TestGraphHighlightFollowsCursor(t *testing.T) {
	repo, err := git.Open(mergeHeavyRepo(t, 60))
	require.NoError(t, err)
	m := New(repo, &fakeSystem{})
	m.now = func() time.Time { return time.Unix(1700000100, 0) }
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 15})
	m = settle(t, m, m.Init())
	m, _ = update(m, wipLoadedMsg{files: []git.FileChange{{Status: '?', Path: "x"}}})
	require.Equal(t, RowWIP, m.rows[0].Kind)

	distinct := map[string]bool{}
	for i := 0; i < len(m.rows); i++ {
		if i > 0 {
			m = press(t, m, "j")
		}
		require.Equal(t, i, m.cursor)
		hash := ""
		if c := m.Selected().Commit; c != nil {
			hash = c.Hash
		}
		ref := BuildRows(m.commits, m.wip, m.track, m.prs, hash)
		for k := range ref { // View attaches the cached stats
			if ref[k].Kind == RowCommit {
				if st, ok := m.stats[ref[k].Commit.Hash]; ok {
					ref[k].Stat = &st
				}
			}
		}
		w := ComputeWidths(m.width, m.visibleGraphWidth(), m.labelWidth, m.visibleAuthorWidth())
		view := strings.Split(m.View(), "\n")
		var want []string
		for j := m.offset; j < min(m.offset+m.listHeight(), len(ref)); j++ {
			want = append(want, RenderRow(ref[j], w, j == m.cursor, m.now()))
		}
		require.Equal(t, want, view[headerLines:headerLines+len(want)], "cursor %d", i)
		distinct[strings.Join(want, "\n")] = true
	}
	require.Greater(t, len(distinct), len(m.rows)/2)
}

// TestDataUpdateStaysCheap: WIP, tracking and PR results only rebuild labels
// and the WIP row; the graph layout is recomputed only when commits change.
func TestDataUpdateStaysCheap(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test")
	}
	m := largeModel(t, 3000)
	msgs := []tea.Msg{
		wipLoadedMsg{files: []git.FileChange{{Status: 'M', Path: "a"}}},
		trackLoadedMsg{track: map[string]git.Track{"main": {Upstream: "origin/main", Ahead: 1}}},
		prsLoadedMsg{prs: map[string]github.PR{"main": {Number: 7}}},
		wipLoadedMsg{},
	}
	const rounds = 5
	start := time.Now()
	for range rounds {
		for _, msg := range msgs {
			m, _ = update(m, msg)
		}
	}
	avg := time.Since(start) / (rounds * time.Duration(len(msgs)))
	t.Logf("average WIP/tracking/PR update over %d commits: %v", len(m.commits), avg)
	require.Less(t, avg, 20*time.Millisecond)
}

// TestGraphWidthFollowsVisibleWindow: the graph column is sized by the rows
// on screen, not by the widest row ever loaded.
func TestGraphWidthFollowsVisibleWindow(t *testing.T) {
	m := largeModel(t, 600)
	ref := BuildRows(m.commits, m.wip, m.track, m.prs, "")
	seen := map[int]bool{}
	for m.cursor < len(m.rows)-1 {
		want := 0
		for i := m.offset; i < min(m.offset+m.listHeight(), len(ref)); i++ {
			want = max(want, lipgloss.Width(ref[i].Graph))
		}
		require.Equal(t, want, m.visibleGraphWidth(), "offset %d", m.offset)
		seen[want] = true
		m, _ = update(m, keyMsg("ctrl+d"))
	}
	require.Greater(t, len(seen), 1, "window widths vary")
}
