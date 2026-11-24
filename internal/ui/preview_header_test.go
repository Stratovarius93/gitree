package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authoredRepo ends with a tagged commit by someone other than the committer,
// with a co-author, that modifies, adds and deletes files.
func authoredRepo(t *testing.T) (*testutil.Repo, time.Time) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"mod.txt": "one\ntwo\n", "del.txt": "gone\n"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "mod.txt"), []byte("one\nTWO\nthree\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "add.txt"), []byte("fresh\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(r.Dir, "del.txt")))
	r.Git("add", "-A")
	r.Git("commit", "-q", "--author=José Núñez <jose@example.com>", "--date=2026-11-20T10:30:00+00:00",
		"-m", "Add café search\n\nCo-authored-by: Zoë Ångström <zoe@example.com>\nCo-authored-by: Bob <bob@example.com>")
	r.Tag("v1.0")
	return r, time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
}

func openHeader(t *testing.T, r *testutil.Repo, w, h int, now time.Time) Model {
	t.Helper()
	m := loaded(t, r)
	m.now = func() time.Time { return now }
	return press(t, sized(m, w, h), "space")
}

func TestPreviewHeaderCommit(t *testing.T) {
	r, date := authoredRepo(t)
	m := openHeader(t, r, 140, 40, date.Add(3*24*time.Hour+time.Hour))
	require.Equal(t, ModePreviewFiles, m.mode)
	v := viewText(m)
	for _, want := range []string{
		"Add café search",
		"Author     José Núñez <jose@example.com>",
		"Co-authors Zoë Ångström, Bob",
		"Committer  Test <test@example.com>",
		"Date       " + date.Local().Format("Jan 2, 2006 15:04") + "  (3 days ago)",
		"Refs",
		"v1.0",
		"Changes    +3 −2   3 files: 1 M · 1 A · 1 D",
		"─────",
		"M  mod.txt",
	} {
		assert.Contains(t, v, want)
	}
	assert.Less(t, strings.Index(v, "Changes"), strings.Index(v, "M  mod.txt"), "header above the list")

	raw := m.View()
	fg := func(hex, s string) string { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(s) }
	assert.Contains(t, raw, fg(colors.Green, "+3"))
	assert.Contains(t, raw, fg(colors.Red, "−2"))
	assert.Contains(t, raw, fg(colors.Yellow, "1 M"))
	assert.Contains(t, raw, fg(colors.Green, "1 A"))
	assert.Contains(t, raw, fg(colors.Red, "1 D"))
	assert.Contains(t, raw, fg(colors.Gray, "Author     "), "keys are dim")
	assert.Contains(t, raw, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.FgFloat)).Render(" Add café search"))
	assertFits(t, m)
}

func TestPreviewHeaderOmitsEmptyLines(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a\n"})
	r.Commit("second", map[string]string{"a.txt": "b\n"})
	m := loaded(t, r)
	m = selectWhere(t, m, func(row Row) bool { return row.Commit != nil && row.Commit.Subject == "first" })
	m = press(t, sized(m, 120, 30), "space")
	v := viewText(m)
	assert.Contains(t, v, "Author     Test <test@example.com>")
	assert.NotContains(t, v, "Co-authors")
	assert.NotContains(t, v, "Committer")
	assert.NotContains(t, v, "Refs")
	assert.Contains(t, v, "Changes    +1 −0   1 file: 1 A")
	assert.NotContains(t, v, "0 M")
}

func TestPreviewHeaderWIP(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a\n"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("a\nb\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "new.txt"), []byte("hello\n"), 0o644))
	m := press(t, sized(loaded(t, r), 120, 30), "space")
	v := viewText(m)
	assert.Contains(t, v, "Working tree")
	assert.Contains(t, v, "Changes    +2 −0   2 files: 1 M · 1 ?")
	assert.NotContains(t, v, "Author")
	assert.NotContains(t, v, "Date ")
	assert.Contains(t, m.View(), lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray)).Render("1 ?"))
}

func TestPreviewHeaderKeepsListRows(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	files := map[string]string{}
	for _, c := range "abcdefghijklmnop" {
		files[string(c)+".txt"] = "x\n"
	}
	r.Commit("many", files)
	for _, sz := range [][2]int{{40, 12}, {200, 50}, {60, 16}} {
		m := press(t, sized(loaded(t, r), sz[0], sz[1]), "space")
		assertFits(t, m)
		assert.GreaterOrEqual(t, m.previewListRows(), min(3, m.previewBodyHeight()), "%v", sz)
		assert.Contains(t, viewText(m), "A  a.txt", "%v", sz)
		for i := 0; i < 15; i++ {
			m = press(t, m, "j")
		}
		assertFits(t, m)
		assert.Contains(t, viewText(m), "A  p.txt", "the list scrolls to the selection at %v", sz)
		if m.previewListRows() < 16 {
			assert.NotContains(t, viewText(m), "A  a.txt", "%v", sz)
		}
	}
}

func TestPreviewHeaderMetaStale(t *testing.T) {
	r, _ := authoredRepo(t)
	m := openHeader(t, r, 120, 30, time.Now())
	m, _ = update(m, previewFilesMsg{key: "other", meta: &git.CommitMeta{AuthorName: "Intruder"}})
	assert.NotContains(t, viewText(m), "Intruder")
}

func TestRelativeWords(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{3 * 24 * time.Hour, "3 days ago"},
		{14 * 24 * time.Hour, "2 weeks ago"},
		{60 * 24 * time.Hour, "2 months ago"},
		{400 * 24 * time.Hour, "1 year ago"},
		{800 * 24 * time.Hour, "2 years ago"},
		{-time.Hour, "just now"},
	} {
		assert.Equal(t, c.want, relativeWords(now.Add(-c.ago), now))
	}
}
