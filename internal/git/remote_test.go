package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cloned returns a clone of a fresh origin (one commit on main), opened, plus
// the seed clone that can push to the origin.
func cloned(t *testing.T) (*testutil.Repo, *Repo, *testutil.Repo, string) {
	t.Helper()
	bare, seed := testutil.NewOrigin(t)
	c := testutil.Clone(t, bare)
	repo, err := Open(c.Dir)
	require.NoError(t, err)
	return c, repo, seed, bare
}

func TestPullFastForwards(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("two", map[string]string{"b": "1"})
	tip := seed.Commit("three", map[string]string{"c": "1"})
	seed.Git("push", "-q")

	n, err := repo.Pull(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, tip, c.Git("rev-parse", "HEAD"))
	assert.Equal(t, "main", c.Git("branch", "--show-current"))
}

func TestPullAlreadyUpToDate(t *testing.T) {
	c, repo, _, _ := cloned(t)
	before := c.Git("rev-parse", "HEAD")
	n, err := repo.Pull(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, before, c.Git("rev-parse", "HEAD"))
}

func TestPullDivergedKnown(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	local := c.Commit("local", map[string]string{"l": "1"})

	_, err := repo.Pull(context.Background())
	assert.ErrorIs(t, err, ErrDiverged)
	assert.Equal(t, "branches diverged: pull with lazygit (L)", err.Error())
	assert.Equal(t, local, c.Git("rev-parse", "HEAD"))
}

func TestPullDivergedDiscoveredByGit(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	local := c.Commit("local", map[string]string{"l": "1"}) // not fetched: only git sees it

	_, err := repo.Pull(context.Background())
	assert.ErrorIs(t, err, ErrDiverged)
	assert.Equal(t, local, c.Git("rev-parse", "HEAD"))
	assert.Empty(t, c.Git("status", "--porcelain"))
}

func TestPullRefusesDirty(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("two", map[string]string{"b": "1"})
	seed.Git("push", "-q")
	before := c.Git("rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("dirty"), 0o644))

	_, err := repo.Pull(context.Background())
	assert.ErrorIs(t, err, ErrDirty)
	assert.Equal(t, before, c.Git("rev-parse", "HEAD"))
}

func TestPullAndPushRefuseDetached(t *testing.T) {
	c, repo, _, _ := cloned(t)
	c.Git("checkout", "-q", "--detach")
	_, err := repo.Pull(context.Background())
	assert.ErrorIs(t, err, ErrDetached)
	_, err = repo.PushPlan()
	assert.ErrorIs(t, err, ErrDetached)
	assert.ErrorIs(t, repo.Push(context.Background(), PushPlan{Branch: "main", Remote: "origin", Target: "main"}), ErrDetached)
}

func TestPullWithoutUpstream(t *testing.T) {
	c, repo, _, _ := cloned(t)
	c.Git("checkout", "-q", "-b", "topic")
	_, err := repo.Pull(context.Background())
	require.Error(t, err)
	assert.Equal(t, "no upstream: push first with P", err.Error())
}

func TestPushToUpstream(t *testing.T) {
	c, repo, _, bare := cloned(t)
	tip := c.Commit("mine", map[string]string{"m": "1"})
	require.NoError(t, push(t, repo))
	assert.Equal(t, tip, gitIn(t, bare, "rev-parse", "main"))
	track, err := repo.Tracking()
	require.NoError(t, err)
	assert.Equal(t, 0, track["main"].Ahead)
}

func TestPushSetsUpstream(t *testing.T) {
	c, repo, _, bare := cloned(t)
	c.Git("checkout", "-q", "-b", "feature/x")
	tip := c.Commit("feat", map[string]string{"f": "1"})
	require.NoError(t, push(t, repo))
	assert.Equal(t, tip, gitIn(t, bare, "rev-parse", "feature/x"))
	assert.Equal(t, "origin/feature/x", c.Git("rev-parse", "--abbrev-ref", "feature/x@{upstream}"))
}

func TestPushWithoutOrigin(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("one", nil)
	repo, err := Open(r.Dir)
	require.NoError(t, err)
	err = push(t, repo)
	require.Error(t, err)
	assert.Equal(t, "no remote 'origin'", err.Error())
}

func TestPushRejectedNeverForces(t *testing.T) {
	c, repo, seed, bare := cloned(t)
	remoteTip := seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	c.Commit("local", map[string]string{"l": "1"})

	err := push(t, repo)
	assert.ErrorIs(t, err, ErrRejected)
	assert.Equal(t, "push rejected: remote has new commits — pull first (p)", err.Error())
	assert.Equal(t, remoteTip, gitIn(t, bare, "rev-parse", "main"), "nothing forced")
}

func TestPushNothingToPush(t *testing.T) {
	_, repo, _, _ := cloned(t)
	err := push(t, repo)
	assert.ErrorIs(t, err, ErrNothingToPush)
	assert.Equal(t, "nothing to push", err.Error())
}

func TestPullAndPushKilledOnTimeout(t *testing.T) {
	unsetSSHEnv(t)
	for _, op := range []string{"pull", "push"} {
		t.Run(op, func(t *testing.T) {
			repo, log := fakeSSHRepo(t, "sleep 30")
			if op == "pull" {
				_, err := repo.run("config", "branch.main.remote", "origin")
				require.NoError(t, err)
				_, err = repo.run("config", "branch.main.merge", "refs/heads/main")
				require.NoError(t, err)
				_, err = repo.run("update-ref", "refs/remotes/origin/main", "HEAD") // not gone
				require.NoError(t, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			start := time.Now()
			var err error
			if op == "pull" {
				_, err = repo.Pull(ctx)
			} else {
				err = repo.Push(ctx, PushPlan{Branch: "main", Remote: "origin", Target: "main", Kind: PushNewBranch})
			}
			require.Error(t, err)
			assert.Equal(t, "timed out", err.Error())
			assert.Less(t, time.Since(start), 4*time.Second)
			_, statErr := os.Stat(log)
			assert.NoError(t, statErr, "git really reached the remote")
		})
	}
}

func TestNetworkErrorIsOneLine(t *testing.T) {
	unsetSSHEnv(t)
	repo, _ := fakeSSHRepo(t, "echo noise >&2; exit 1")
	err := push(t, repo)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrRejected))
	assert.NotContains(t, err.Error(), "\n")
}

// push plans and runs a push of the current branch, as the UI does.
func push(t *testing.T, repo *Repo) error {
	t.Helper()
	plan, err := repo.PushPlan()
	if err != nil {
		return err
	}
	return repo.Push(context.Background(), plan)
}

func TestPushFromBranchTrackingOtherNameCreatesOwnBranch(t *testing.T) {
	c, repo, _, bare := cloned(t)
	mainTip := gitIn(t, bare, "rev-parse", "main")
	c.Git("checkout", "-q", "-b", "my-feature", "origin/main")
	tip := c.Commit("feat", map[string]string{"f": "1"})

	plan, err := repo.PushPlan()
	require.NoError(t, err)
	assert.Equal(t, PushPlan{Branch: "my-feature", Remote: "origin", Target: "my-feature", Kind: PushNewBranch}, plan)
	require.NoError(t, repo.Push(context.Background(), plan))
	assert.Equal(t, mainTip, gitIn(t, bare, "rev-parse", "main"), "remote main unchanged")
	assert.Equal(t, tip, gitIn(t, bare, "rev-parse", "my-feature"))
	assert.Equal(t, "origin/my-feature", c.Git("rev-parse", "--abbrev-ref", "my-feature@{upstream}"))
}

func TestPushFromBranchTrackingLocalBranch(t *testing.T) {
	c, repo, _, bare := cloned(t)
	mainTip := gitIn(t, bare, "rev-parse", "main")
	c.Git("checkout", "-q", "--track", "-b", "topic", "main")
	tip := c.Commit("t", map[string]string{"t": "1"})

	plan, err := repo.PushPlan()
	require.NoError(t, err)
	assert.Equal(t, PushNewBranch, plan.Kind)
	assert.Equal(t, "origin/topic", plan.Dest())
	require.NoError(t, repo.Push(context.Background(), plan))
	assert.Equal(t, mainTip, gitIn(t, bare, "rev-parse", "main"))
	assert.Equal(t, tip, gitIn(t, bare, "rev-parse", "topic"))
}

func TestPushRefusesChangedPlan(t *testing.T) {
	c, repo, _, bare := cloned(t)
	c.Commit("mine", map[string]string{"m": "1"})
	before := gitIn(t, bare, "rev-parse", "main")
	plan, err := repo.PushPlan()
	require.NoError(t, err)
	c.Git("checkout", "-q", "-b", "other")
	c.Commit("other", map[string]string{"o": "1"})

	err = repo.Push(context.Background(), plan)
	assert.ErrorIs(t, err, ErrBranchChanged)
	assert.Equal(t, "branch changed — try again", err.Error())
	assert.Equal(t, before, gitIn(t, bare, "rev-parse", "main"))

	c.Git("checkout", "-q", "main")
	c.Git("branch", "--set-upstream-to", "origin/main", "other")
	stale := plan
	stale.Target = "elsewhere"
	assert.ErrorIs(t, repo.Push(context.Background(), stale), ErrBranchChanged)
}

func TestGoneUpstream(t *testing.T) {
	c, repo, seed, bare := cloned(t)
	c.Git("checkout", "-q", "-b", "feature/x")
	c.Commit("feat", map[string]string{"f": "1"})
	c.Git("push", "-q", "-u", "origin", "feature/x")
	seed.Git("push", "-q", "origin", "--delete", "feature/x")
	c.Git("fetch", "-q", "--prune")

	_, err := repo.Pull(context.Background())
	require.Error(t, err)
	assert.Equal(t, "upstream origin/feature/x no longer exists", err.Error())

	plan, err := repo.PushPlan()
	require.NoError(t, err)
	assert.Equal(t, PushPlan{Branch: "feature/x", Remote: "origin", Target: "feature/x", Kind: PushRecreate}, plan)
	require.NoError(t, repo.Push(context.Background(), plan))
	assert.Equal(t, c.Git("rev-parse", "HEAD"), gitIn(t, bare, "rev-parse", "feature/x"))
}

func TestPushHookRejectionShowsReason(t *testing.T) {
	c, repo, _, bare := cloned(t)
	hook := bare + "/hooks/pre-receive"
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'pushes are frozen today' >&2\nexit 1\n"), 0o755))
	c.Commit("mine", map[string]string{"m": "1"})

	err := push(t, repo)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrRejected))
	assert.NotContains(t, err.Error(), "\n")
	assert.Contains(t, err.Error(), "pushes are frozen today")
	assert.Contains(t, err.Error(), "rejected by remote")
}

// gitIn runs git in dir (e.g. the bare origin) and returns trimmed stdout.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestPullArgsAutostashOnlyWhenAsked(t *testing.T) {
	plain := pullArgs(false)
	assert.Contains(t, plain, "--no-autostash")
	assert.NotContains(t, plain, "--autostash")
	as := pullArgs(true)
	assert.Contains(t, as, "--autostash")
	assert.NotContains(t, as, "--no-autostash")
	for _, a := range [][]string{plain, as} {
		assert.Contains(t, a, "--ff-only")
		assert.Contains(t, a, "--no-rebase")
	}
}

func TestPullAutostashKeepsLocalChanges(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("two", map[string]string{"b": "1"})
	tip := seed.Commit("three", map[string]string{"base.txt": "base"}) // same content: no conflict
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("mine"), 0o644))

	n, err := repo.PullAutostash(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, tip, c.Git("rev-parse", "HEAD"))
	b, _ := os.ReadFile(c.Dir + "/base.txt")
	assert.Equal(t, "mine", string(b), "local change restored")
	assert.Empty(t, c.Git("stash", "list"))
}

func TestPullAutostashRestoreConflict(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	tip := seed.Commit("remote", map[string]string{"base.txt": "theirs"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("mine"), 0o644))

	n, err := repo.PullAutostash(context.Background())
	var ae *AutostashConflictError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 1, ae.Commits)
	assert.Equal(t, 1, n)
	assert.Equal(t, tip, c.Git("rev-parse", "HEAD"), "pull applied")
	assert.NotEmpty(t, c.Git("stash", "list"), "changes are safe in the stash")
}

func TestPullAutostashStillRefusesDivergedAndInProgress(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("remote", map[string]string{"r": "1"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	local := c.Commit("local", map[string]string{"l": "1"})
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("mine"), 0o644))
	_, err := repo.PullAutostash(context.Background())
	assert.ErrorIs(t, err, ErrDiverged)
	assert.Equal(t, local, c.Git("rev-parse", "HEAD"))

	c.Git("reset", "-q", "--hard", "origin/main")
	gitDir := c.Git("rev-parse", "--absolute-git-dir")
	require.NoError(t, os.WriteFile(gitDir+"/MERGE_HEAD", []byte(local+"\n"), 0o644))
	_, err = repo.PullAutostash(context.Background())
	assert.ErrorIs(t, err, ErrInProgress)
}

func TestPushWithDirtyTree(t *testing.T) {
	c, repo, _, _ := cloned(t)
	tip := c.Commit("one", map[string]string{"x": "1"})
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("dirty"), 0o644))
	require.NoError(t, push(t, repo))
	assert.Equal(t, tip, c.Git("rev-parse", "origin/main"))
	b, _ := os.ReadFile(c.Dir + "/base.txt")
	assert.Equal(t, "dirty", string(b))
}

func TestPullAutostashKilledBeforeRestoreKeepsChangesInStash(t *testing.T) {
	c, repo, seed, _ := cloned(t)
	seed.Commit("two", map[string]string{"b": "1"})
	seed.Git("push", "-q")
	c.Git("fetch", "-q")
	gitDir := c.Git("rev-parse", "--absolute-git-dir")
	// Hang after the branch moved, i.e. after git stashed the changes but
	// before it restores them.
	hook := "#!/bin/sh\nif [ \"$1\" = committed ] && grep -q ' refs/heads/main$'; then sleep 30; fi\nexit 0\n"
	require.NoError(t, os.WriteFile(gitDir+"/hooks/reference-transaction", []byte(hook), 0o755))
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("mine"), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := repo.PullAutostash(ctx)
	var ke *AutostashKeptError
	require.ErrorAs(t, err, &ke)
	assert.Contains(t, err.Error(), "pull did not finish")
	assert.Contains(t, err.Error(), "safe in the stash")
	require.NoError(t, os.Remove(gitDir+"/hooks/reference-transaction"))

	_, statErr := os.Stat(gitDir + "/MERGE_AUTOSTASH")
	assert.True(t, os.IsNotExist(statErr), "moved into the stash")
	assert.Contains(t, c.Git("stash", "list"), "autostash")
	busy, err := repo.InProgress()
	require.NoError(t, err)
	assert.False(t, busy)
	c.Git("checkout", "-q", "--", ".")
	c.Git("stash", "pop", "-q")
	b, _ := os.ReadFile(c.Dir + "/base.txt")
	assert.Equal(t, "mine", string(b), "the change is recoverable")
}

func TestMergeAutostashMeansInProgress(t *testing.T) {
	c, repo, _, _ := cloned(t)
	gitDir := c.Git("rev-parse", "--absolute-git-dir")
	require.NoError(t, os.WriteFile(gitDir+"/MERGE_AUTOSTASH", []byte(c.Git("rev-parse", "HEAD")+"\n"), 0o644))
	busy, err := repo.InProgress()
	require.NoError(t, err)
	assert.True(t, busy)
	_, err = repo.PullAutostash(context.Background())
	assert.ErrorIs(t, err, ErrInProgress)
}

func TestAutostashStoredIgnoresOtherStashes(t *testing.T) {
	c, repo, _, _ := cloned(t)
	assert.False(t, repo.autostashStored(""), "no stash")
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("x"), 0o644))
	c.Git("stash", "push", "-q", "-m", "someone else")
	tip := repo.stashTip()
	assert.False(t, repo.autostashStored(""), "a stash pushed by another tool")
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("y"), 0o644))
	sha := c.Git("stash", "create", "autostash")
	c.Git("stash", "store", "-m", "autostash", sha)
	assert.True(t, repo.autostashStored(tip))
	assert.False(t, repo.autostashStored(repo.stashTip()), "unchanged")
}

func TestTrackedChangesStaged(t *testing.T) {
	c, repo, _, _ := cloned(t)
	require.NoError(t, os.WriteFile(c.Dir+"/base.txt", []byte("x"), 0o644))
	fs, err := repo.TrackedChanges()
	require.NoError(t, err)
	require.Len(t, fs, 1)
	assert.False(t, fs[0].Staged)
	c.Git("add", "base.txt")
	fs, _ = repo.TrackedChanges()
	require.Len(t, fs, 1)
	assert.True(t, fs[0].Staged)
}
