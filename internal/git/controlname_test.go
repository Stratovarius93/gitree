package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const c1Name = "a\u0085b" // git allows C1 characters in branch names

// c1Repo is a clone with a real branch "ab" and a branch "a<U+0085>b" with
// its own commit, checked out.
func c1Repo(t *testing.T) (*Repo, string, string) {
	c, repo, _, bare := cloned(t)
	c.Git("branch", "ab")
	c.Git("checkout", "-q", "-b", c1Name)
	tip := c.Commit("c1", map[string]string{"c1": "1"})
	return repo, tip, bare
}

func TestControlNameBranchIsRefused(t *testing.T) {
	repo, _, bare := c1Repo(t)
	before := gitIn(t, bare, "for-each-ref")

	_, err := repo.PushPlan()
	assert.ErrorIs(t, err, ErrControlName)
	assert.ErrorIs(t, repo.Push(context.Background(), PushPlan{Branch: "ab", Remote: "origin", Target: "ab", Kind: PushNewBranch}), ErrControlName)
	_, err = repo.Pull(context.Background())
	assert.ErrorIs(t, err, ErrControlName)
	assert.Equal(t, "branch name contains control characters: use git directly", ErrControlName.Error())
	assert.Equal(t, before, gitIn(t, bare, "for-each-ref"), "nothing pushed")
}

func TestControlNameDisplayIsSanitized(t *testing.T) {
	repo, _, _ := c1Repo(t)
	cur, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "ab", cur)
}

func TestCheckoutAndMergeRefuseAmbiguousName(t *testing.T) {
	repo, _, _ := c1Repo(t)
	// the display name "ab" is also the name of the C1 branch: refuse
	assert.ErrorIs(t, repo.CheckoutBranch("ab"), ErrControlName)
	assert.ErrorIs(t, repo.Merge("ab"), ErrControlName)
	// and with the C1 branch current, merge/cherry-pick refuse
	assert.ErrorIs(t, repo.Merge("main"), ErrControlName)
	assert.ErrorIs(t, repo.CherryPick("0123456789abcdef"), ErrControlName)
}

func TestCheckoutUnaffectedByUnrelatedControlBranch(t *testing.T) {
	c, repo, _, _ := cloned(t)
	c.Git("branch", c1Name)
	c.Git("branch", "feature/café")
	require.NoError(t, repo.CheckoutBranch("feature/café"))
	assert.Equal(t, "feature/café", c.Git("branch", "--show-current"))
	cur, _ := repo.CurrentBranch()
	assert.Equal(t, "feature/café", cur)
}
