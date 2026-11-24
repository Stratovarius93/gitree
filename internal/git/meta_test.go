package git

import (
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitMetaCoAuthorsAndCommitter(t *testing.T) {
	tr := testutil.NewRepo(t)
	msg := "Add café search\n\nBody line.\n\nCo-authored-by: Zoë Ångström <zoe@example.com>\nco-authored-by: Bob <bob@example.com>\nCo-Authored-By: Zoë Ångström <zoe@example.com>\n"
	tr.Git("commit", "-q", "--allow-empty", "--author=José Núñez <jose@example.com>",
		"--date=2026-11-20T10:30:00+00:00", "-m", msg)
	h := tr.Git("rev-parse", "HEAD")
	r, err := Open(tr.Dir)
	require.NoError(t, err)

	meta, err := r.CommitMeta(h)
	require.NoError(t, err)
	assert.Equal(t, h, meta.Hash)
	assert.Equal(t, "Add café search", meta.Subject)
	assert.Equal(t, "José Núñez", meta.AuthorName)
	assert.Equal(t, "jose@example.com", meta.AuthorEmail)
	assert.Equal(t, "Test", meta.CommitterName)
	assert.Equal(t, "test@example.com", meta.CommitterEmail)
	assert.True(t, meta.CommitterDiffers())
	assert.True(t, meta.AuthorDate.Equal(time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)))
	assert.Equal(t, []string{"Zoë Ångström", "Bob"}, meta.CoAuthors)
	assert.Contains(t, meta.Body, "Body line.")
}

func TestCommitMetaSameCommitter(t *testing.T) {
	tr := testutil.NewRepo(t)
	h := tr.Commit("plain", nil)
	r, _ := Open(tr.Dir)
	meta, err := r.CommitMeta(h)
	require.NoError(t, err)
	assert.False(t, meta.CommitterDiffers())
	assert.Empty(t, meta.CoAuthors)
	assert.Equal(t, "plain", meta.Subject)
}

func TestCommitMetaBadHash(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("x", nil)
	r, _ := Open(tr.Dir)
	_, err := r.CommitMeta("deadbeef")
	assert.Error(t, err)
}

func TestParseCoAuthors(t *testing.T) {
	assert.Equal(t, []string{"A B", "c@x.org"}, parseCoAuthors("s\n\nCo-authored-by: A B <a@b>\nCo-authored-by:   <c@x.org>\nCo-authored-by:\nnot a trailer Co-authored-by: X <x>"))
}

func TestCommitMetaUsesMailmapAndOneLineSubject(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Git("commit", "-q", "--allow-empty", "--author=old nick <old@example.com>",
		"-m", "first line\nwrapped line\n\nbody")
	h := tr.Git("rev-parse", "HEAD")
	tr.Commit("mailmap", map[string]string{".mailmap": "Real Name <real@example.com> old nick <old@example.com>\nCommitter Real <cr@example.com> Test <test@example.com>\n"})
	r, err := Open(tr.Dir)
	require.NoError(t, err)

	meta, err := r.CommitMeta(h)
	require.NoError(t, err)
	assert.Equal(t, "Real Name", meta.AuthorName)
	assert.Equal(t, "real@example.com", meta.AuthorEmail)
	assert.Equal(t, "Committer Real", meta.CommitterName)
	assert.Equal(t, "cr@example.com", meta.CommitterEmail)
	commits, err := r.Log(ScopeDefault, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, commits[1].Subject, meta.Subject, "same subject as the table")
	assert.Equal(t, "first line wrapped line", meta.Subject)
}
