package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noDelta(t *testing.T) {
	old := git.LookPath
	git.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { git.LookPath = old })
}

// mixedRepo ends with a commit that modifies, adds, deletes and renames files.
func mixedRepo(t *testing.T) (*testutil.Repo, string) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{
		"mod.txt": "one\ntwo\n", "del.txt": "gone\n",
		"old.txt": "alpha\nbeta\ngamma\ndelta\nepsilon\n",
	})
	r.Git("mv", "old.txt", "new.txt")
	require.NoError(t, os.Remove(filepath.Join(r.Dir, "del.txt")))
	hash := r.Commit("mixed", map[string]string{"mod.txt": "one\ntwo\nthree\n", "add.txt": "fresh\n"})
	return r, hash
}

func viewText(m Model) string { return ansi.Strip(m.View()) }

func TestSpaceOpensFileList(t *testing.T) {
	r, _ := mixedRepo(t)
	m := press(t, sized(loaded(t, r), 120, 30), "space")
	assert.Equal(t, ModePreviewFiles, m.mode)
	v := viewText(m)
	for _, want := range []string{"M  mod.txt", "A  add.txt", "D  del.txt", "R  old.txt → new.txt", "4 files: "} {
		assert.Contains(t, v, want)
	}
	assert.NotContains(t, v, "4 files  +", "the header has the totals: no footer repeats them")
	assert.Contains(t, v, "╭")
	assert.NotContains(t, v, "three", "first level lists files only, no diff")
	assertFits(t, m)
}

func TestPreviewFileListHasNoTotalsFooter(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("one", map[string]string{"a.txt": "a\n"})
	m := press(t, sized(loaded(t, r), 100, 20), "space")
	v := viewText(m)
	assert.Contains(t, v, "+1 −0   1 file: 1 A", "totals live in the header")
	assert.NotContains(t, v, "1 file  +1 −0")
	assert.Equal(t, 1, strings.Count(v, "1 file"))
}

func TestSpaceOnFileOpensDiff(t *testing.T) {
	r, _ := mixedRepo(t)
	m := press(t, sized(loaded(t, r), 120, 30), "space", "j", "space")
	require.Equal(t, ModePreviewDiff, m.mode)
	path := m.preview.files[m.preview.sel].Path
	v := viewText(m)
	assert.Contains(t, v, path)
	assert.Contains(t, v, "@@")
	assertFits(t, m)

	m = press(t, sized(loaded(t, r), 120, 30), "space")
	for i, f := range m.preview.files {
		if f.Path == "mod.txt" {
			m.preview.sel = i
		}
	}
	m = press(t, m, "space")
	require.Equal(t, ModePreviewDiff, m.mode)
	assert.Contains(t, viewText(m), "+three")
}

func TestEscBackAndClose(t *testing.T) {
	r, _ := mixedRepo(t)
	m := press(t, sized(loaded(t, r), 120, 30), "space", "j", "j", "space")
	require.Equal(t, ModePreviewDiff, m.mode)
	sel := m.preview.sel
	m = press(t, m, "esc")
	assert.Equal(t, ModePreviewFiles, m.mode)
	assert.Equal(t, sel, m.preview.sel)
	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
	assert.NotContains(t, viewText(m), "files")
}

func TestPreviewDiffScrolls(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("long", map[string]string{"a.txt": strings.Repeat("line\n", 100)})
	m := press(t, sized(loaded(t, r), 100, 20), "space", "space")
	require.Equal(t, 0, m.preview.vp.YOffset)
	m = press(t, m, "ctrl+d")
	assert.Greater(t, m.preview.vp.YOffset, 0)
	m = press(t, m, "ctrl+u")
	assert.Equal(t, 0, m.preview.vp.YOffset)
}

func TestPreviewOnBranchUsesTip(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a\n"})
	r.Commit("second", map[string]string{"b.txt": "b\n"})
	r.Branch("feature")
	r.Commit("third", map[string]string{"c.txt": "c\n"})
	m := sized(loaded(t, r), 120, 30)
	for i, row := range m.rows {
		if row.Commit != nil && row.Commit.Subject == "second" {
			m.cursor = i // the row carrying the feature label
		}
	}
	m = press(t, m, "space")
	v := viewText(m)
	assert.Contains(t, v, "A  b.txt")
	assert.NotContains(t, v, "c.txt")
}

func TestPreviewOnWIP(t *testing.T) {
	noDelta(t)
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a\n"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("a\nb\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "new.txt"), []byte("hello\n"), 0o644))
	m := loaded(t, r)
	require.Equal(t, RowWIP, m.Selected().Kind)
	m = press(t, sized(m, 120, 30), "space")
	v := viewText(m)
	assert.Contains(t, v, "M  a.txt")
	assert.Contains(t, v, "?  new.txt")
	assert.Contains(t, v, "// WIP")

	m = press(t, m, "j", "space")
	require.Equal(t, ModePreviewDiff, m.mode)
	assert.Contains(t, viewText(m), "+hello")
}

func TestPreviewRenamedFileDiff(t *testing.T) {
	r, _ := mixedRepo(t)
	m := press(t, sized(loaded(t, r), 120, 30), "space")
	for i, f := range m.preview.files {
		if f.Status == 'R' {
			m.preview.sel = i
		}
	}
	m = press(t, m, "space")
	require.Equal(t, ModePreviewDiff, m.mode)
	v := viewText(m)
	assert.Contains(t, v, "rename")
	assert.NotContains(t, v, "new file mode")
	assert.NotContains(t, v, "+alpha", "rename is not shown as a whole-file add")
}

func TestPreviewStaleMessagesDropped(t *testing.T) {
	r, hash := mixedRepo(t)
	m := press(t, sized(loaded(t, r), 120, 30), "space")
	m, _ = update(m, previewFilesMsg{key: "other", files: []git.FileChange{{Status: 'M', Path: "zzz"}}})
	assert.NotContains(t, viewText(m), "zzz")
	m, _ = update(m, previewDiffMsg{key: hash, path: "mod.txt", diff: "late"})
	assert.Equal(t, ModePreviewFiles, m.mode)
	assert.NotContains(t, viewText(m), "late")
}

func TestPreviewSmallTerminal(t *testing.T) {
	r, _ := mixedRepo(t)
	for _, sz := range [][2]int{{40, 12}, {20, 6}, {10, 3}, {1, 1}} {
		m := sized(loaded(t, r), sz[0], sz[1])
		m = press(t, m, "space")
		assertFits(t, m)
		m = press(t, m, "space")
		assertFits(t, m)
		m = press(t, m, "j", "esc", "j", "space")
		assertFits(t, m)
	}
}
