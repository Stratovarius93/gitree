package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fingerprint(t *testing.T, r *testutil.Repo) string {
	t.Helper()
	repo, err := Open(r.Dir)
	require.NoError(t, err)
	fp, err := repo.Fingerprint()
	require.NoError(t, err)
	return fp
}

func TestFingerprintStable(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	assert.Equal(t, fingerprint(t, r), fingerprint(t, r))
}

func TestFingerprintChangesOnCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	before := fingerprint(t, r)
	r.Commit("second", map[string]string{"b.txt": "b"})
	assert.NotEqual(t, before, fingerprint(t, r))
}

func TestFingerprintChangesOnCheckoutSameCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	r.Branch("other")
	before := fingerprint(t, r)
	r.Checkout("other")
	assert.NotEqual(t, before, fingerprint(t, r), "switching to a branch at the same commit")
}

func TestFingerprintChangesOnDetach(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first", map[string]string{"a.txt": "a"})
	r.Commit("second", map[string]string{"b.txt": "b"})
	r.Git("checkout", "-q", "--detach")
	before := fingerprint(t, r)
	r.Git("checkout", "-q", first)
	assert.NotEqual(t, before, fingerprint(t, r), "moving a detached HEAD")
}

func TestFingerprintChangesOnNewTagAndStash(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	before := fingerprint(t, r)
	r.Tag("v1")
	afterTag := fingerprint(t, r)
	assert.NotEqual(t, before, afterTag)

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644))
	r.Git("stash", "push", "-q")
	assert.NotEqual(t, afterTag, fingerprint(t, r))
}

func TestFingerprintUnchangedByStatus(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	before := fingerprint(t, r)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("edited"), 0o644))
	repo, err := Open(r.Dir)
	require.NoError(t, err)
	_, err = repo.WorkingFiles()
	require.NoError(t, err)
	assert.Equal(t, before, fingerprint(t, r))
}

func TestFingerprintUnbornRepo(t *testing.T) {
	r := testutil.NewRepo(t)
	repo, err := Open(r.Dir)
	require.NoError(t, err)
	_, err = repo.Fingerprint()
	assert.NoError(t, err)
}
