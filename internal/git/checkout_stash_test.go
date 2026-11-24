package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

// stashRepo: main and other, which changes `a` (line 8 of 12 lines), with main
// checked out.
func stashRepo(t *testing.T) (*testutil.Repo, *Repo) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": lines("8"), "b": "b\n"})
	r.Git("checkout", "-q", "-b", "other")
	r.Commit("other", map[string]string{"a": lines("eight")})
	r.Checkout("main")
	return r, openRepo(t, r.Dir)
}

// lines is 1..12 one per line, with line 8 replaced by eight.
func lines(eight string) string {
	var b strings.Builder
	for _, l := range []string{"1", "2", "3", "4", "5", "6", "7", eight, "9", "10", "11", "12"} {
		b.WriteString(l + "\n")
	}
	return b.String()
}

func write(t *testing.T, r *testutil.Repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, r *testutil.Repo, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCheckoutWithStashRestoresChangesKeepingStaged(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "b", "staged\n")
	r.Git("add", "b")
	write(t, r, "new", "added\n")
	r.Git("add", "new")
	write(t, r, "untracked", "u")

	res, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Unstaged {
		t.Fatal("staged state should be restored with --index")
	}
	if cur := r.Git("branch", "--show-current"); cur != "other" {
		t.Fatalf("current = %q", cur)
	}
	if got := r.Git("status", "--porcelain"); got != "M  b\nA  new\n?? untracked" {
		t.Fatalf("status = %q", got)
	}
	if read(t, r, "a") != lines("eight") {
		t.Fatal("target content")
	}
	if l := r.Git("stash", "list"); l != "" {
		t.Fatalf("stash not popped: %q", l)
	}
}

func TestCheckoutWithStashRemoteAndDetached(t *testing.T) {
	r, repo := stashRepo(t)
	base := r.Git("rev-parse", "main")
	write(t, r, "b", "x\n")
	if _, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutDetached, Name: base}); err != nil {
		t.Fatal(err)
	}
	if cur := r.Git("branch", "--show-current"); cur != "" {
		t.Fatalf("want detached, on %q", cur)
	}
	if read(t, r, "b") != "x\n" {
		t.Fatal("change not restored")
	}

	up := testutil.NewRepo(t)
	up.Commit("base", map[string]string{"a": "1"})
	up.Branch("x")
	c := testutil.NewRepo(t)
	c.Git("remote", "add", "origin", up.Dir)
	c.Git("fetch", "-q", "origin")
	c.Git("checkout", "-q", "-b", "main", "origin/main")
	write(t, c, "a", "dirty")
	if _, err := openRepo(t, c.Dir).CheckoutWithStash(CheckoutTarget{Kind: CheckoutTrack, Name: "origin/x"}); err != nil {
		t.Fatal(err)
	}
	if cur := c.Git("branch", "--show-current"); cur != "x" {
		t.Fatalf("current = %q", cur)
	}
	if read(t, c, "a") != "dirty" {
		t.Fatal("change not restored")
	}
}

func TestCheckoutWithStashFailedCheckoutRestores(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "b", "mine\n")
	r.Git("add", "b")
	write(t, r, "a", lines("mine"))

	_, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "missing"})
	var re *CheckoutRestoredError
	if !errors.As(err, &re) {
		t.Fatalf("want CheckoutRestoredError, got %v", err)
	}
	if cur := r.Git("branch", "--show-current"); cur != "main" {
		t.Fatalf("current = %q", cur)
	}
	if got := r.Git("diff", "--cached", "--name-only"); got != "b" {
		t.Fatalf("staged = %q", got)
	}
	if got := r.Git("diff", "--name-only"); got != "a" {
		t.Fatalf("unstaged = %q", got)
	}
	if l := r.Git("stash", "list"); l != "" {
		t.Fatalf("stash left: %q", l)
	}
}

func TestCheckoutWithStashUntrackedWouldBeOverwritten(t *testing.T) {
	r, repo := stashRepo(t)
	r.Checkout("other")
	r.Commit("adds c", map[string]string{"c": "theirs"})
	r.Checkout("main")
	write(t, r, "c", "untracked mine")
	write(t, r, "b", "mine\n")

	_, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	var re *CheckoutRestoredError
	if !errors.As(err, &re) {
		t.Fatalf("want CheckoutRestoredError, got %v", err)
	}
	var ae *ActionError
	if !errors.As(err, &ae) || !ae.BlockedUntracked || len(ae.Blocked) != 1 || ae.Blocked[0] != "c" {
		t.Fatalf("want blocked untracked c, got %#v", ae)
	}
	if cur := r.Git("branch", "--show-current"); cur != "main" {
		t.Fatalf("current = %q", cur)
	}
	if read(t, r, "b") != "mine\n" || read(t, r, "c") != "untracked mine" {
		t.Fatal("changes not restored")
	}
	if l := r.Git("stash", "list"); l != "" {
		t.Fatalf("stash left: %q", l)
	}
}

func TestCheckoutWithStashPopConflictKeepsStash(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "a", lines("mine"))

	_, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	var ce *StashConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want StashConflictError, got %v", err)
	}
	if cur := r.Git("branch", "--show-current"); cur != "other" {
		t.Fatalf("current = %q", cur)
	}
	if l := r.Git("stash", "list"); !strings.Contains(l, "gitree: checkout other") {
		t.Fatalf("stash must be kept: %q", l)
	}
}

func TestCheckoutWithStashIndexFallback(t *testing.T) {
	r, repo := stashRepo(t)
	// staged change near other's change: the index cannot be restored as is,
	// but the change merges cleanly without --index
	write(t, r, "a", strings.Replace(lines("8"), "10\n", "ten\n", 1))
	r.Git("add", "a")

	res, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Unstaged {
		t.Fatal("want unstaged note")
	}
	if got := read(t, r, "a"); got != strings.Replace(lines("eight"), "10\n", "ten\n", 1) {
		t.Fatalf("a = %q", got)
	}
	if l := r.Git("stash", "list"); l != "" {
		t.Fatalf("stash left: %q", l)
	}
}

func TestCheckoutWithStashRefusals(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "a", lines("mine"))
	r.Git("stash", "push", "-q", "-m", "someone else's")
	// clean tree: nothing stashed, plain checkout, foreign stash untouched
	if _, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"}); err != nil {
		t.Fatal(err)
	}
	if l := r.Git("stash", "list"); !strings.Contains(l, "someone else's") {
		t.Fatalf("foreign stash touched: %q", l)
	}
	if _, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "--detach"}); err == nil {
		t.Fatal("option-looking name must be refused")
	}

	c := testutil.NewRepo(t)
	c.Commit("base", map[string]string{"a": "base"})
	c.Branch("x")
	c.Checkout("x")
	c.Commit("x", map[string]string{"a": "x"})
	c.Checkout("main")
	c.Commit("main", map[string]string{"a": "main"})
	cr := openRepo(t, c.Dir)
	if err := cr.Merge("x"); err == nil {
		t.Fatal("want conflict")
	}
	if _, err := cr.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "x"}); !errors.Is(err, ErrInProgress) {
		t.Fatalf("want ErrInProgress, got %v", err)
	}
	if l := c.Git("stash", "list"); l != "" {
		t.Fatalf("nothing may be stashed: %q", l)
	}
}

func TestCheckoutTargetPlain(t *testing.T) {
	r, repo := stashRepo(t)
	if err := repo.Checkout(CheckoutTarget{Kind: CheckoutLocal, Name: "other"}); err != nil {
		t.Fatal(err)
	}
	write(t, r, "b", "dirty\n")
	if err := repo.Checkout(CheckoutTarget{Kind: CheckoutLocal, Name: "main"}); !errors.Is(err, ErrDirty) {
		t.Fatalf("want ErrDirty, got %v", err)
	}
}

func TestMergeDetachedIsErrDetached(t *testing.T) {
	r, repo := stashRepo(t)
	r.Git("checkout", "-q", "--detach")
	if err := repo.Merge("other"); !errors.Is(err, ErrDetached) {
		t.Fatalf("merge: want ErrDetached, got %v", err)
	}
	if err := repo.CherryPick(r.Git("rev-parse", "other")); !errors.Is(err, ErrDetached) {
		t.Fatalf("cherry-pick: want ErrDetached, got %v", err)
	}
}

func TestCheckoutBlockedByUntracked(t *testing.T) {
	r, repo := stashRepo(t)
	r.Checkout("other")
	r.Commit("adds c", map[string]string{"c": "theirs"})
	r.Checkout("main")
	write(t, r, "c", "mine")
	err := repo.CheckoutBranch("other")
	var ae *ActionError
	if !errors.As(err, &ae) || !ae.BlockedUntracked || len(ae.Blocked) != 1 || ae.Blocked[0] != "c" {
		t.Fatalf("want blocked untracked c, got %#v (%v)", ae, err)
	}
}

// failingHook installs a post-checkout hook that fails after HEAD moved.
func failingHook(t *testing.T, r *testutil.Repo) {
	t.Helper()
	p := filepath.Join(r.Dir, ".git", "hooks", "post-checkout")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho 'git-lfs not found' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCheckoutWithStashHookFailsAfterSwitch(t *testing.T) {
	r, repo := stashRepo(t)
	failingHook(t, r)
	write(t, r, "b", "mine\n")
	res, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	if err != nil {
		t.Fatalf("checkout happened, want success with a note, got %v", err)
	}
	if !strings.Contains(res.Hook, "git-lfs not found") {
		t.Fatalf("hook note = %q", res.Hook)
	}
	if cur := r.Git("branch", "--show-current"); cur != "other" {
		t.Fatalf("current = %q", cur)
	}
	if read(t, r, "b") != "mine\n" {
		t.Fatal("changes not restored on the new branch")
	}
	if l := r.Git("stash", "list"); l != "" {
		t.Fatalf("stash left: %q", l)
	}
}

func TestCheckoutWithStashHookFailsAndConflicts(t *testing.T) {
	r, repo := stashRepo(t)
	failingHook(t, r)
	write(t, r, "a", lines("mine"))
	_, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	var ce *StashConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want StashConflictError, got %v", err)
	}
	if !strings.Contains(ce.Hook, "git-lfs not found") || !strings.Contains(err.Error(), "post-checkout hook failed") {
		t.Fatalf("hook not reported: %v", err)
	}
	if !strings.Contains(err.Error(), "gitree: checkout other") {
		t.Fatalf("entry not named: %v", err)
	}
	if cur := r.Git("branch", "--show-current"); cur != "other" {
		t.Fatalf("current = %q", cur)
	}
	if l := r.Git("stash", "list"); !strings.Contains(l, "gitree: checkout other") {
		t.Fatalf("stash must be kept: %q", l)
	}
}

func TestCheckoutHookFailsAfterSwitch(t *testing.T) {
	r, repo := stashRepo(t)
	failingHook(t, r)
	err := repo.Checkout(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	var he *HookFailedError
	if !errors.As(err, &he) || !strings.Contains(he.Line, "git-lfs not found") {
		t.Fatalf("want HookFailedError, got %v", err)
	}
	if cur := r.Git("branch", "--show-current"); cur != "other" {
		t.Fatalf("current = %q", cur)
	}
}

func TestRestoreStashKeepsEntryWhenNotOnTop(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "b", "mine\n")
	r.Git("stash", "push", "-q", "-m", "gitree: checkout x")
	mine := r.Git("rev-parse", "refs/stash")
	write(t, r, "a", lines("other tool"))
	r.Git("stash", "push", "-q", "-m", "other tool")
	kept, err := repo.restoreStash(mine, true)
	if err != nil {
		t.Fatal(err)
	}
	if !kept {
		t.Fatal("entry not on top must be kept")
	}
	if read(t, r, "b") != "mine\n" {
		t.Fatal("not applied")
	}
	if l := r.Git("stash", "list"); !strings.Contains(l, "gitree: checkout x") || !strings.Contains(l, "other tool") {
		t.Fatalf("entries = %q", l)
	}
}

func TestCheckoutWithStashPushFailsSaysNothingStashed(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "b", "mine\n")
	lock := filepath.Join(r.Dir, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CheckoutWithStash(CheckoutTarget{Kind: CheckoutLocal, Name: "other"})
	if err == nil || !strings.Contains(err.Error(), "nothing was stashed") {
		t.Fatalf("err = %v", err)
	}
	os.Remove(lock)
	if read(t, r, "b") != "mine\n" || r.Git("branch", "--show-current") != "main" {
		t.Fatal("state changed")
	}
}

func TestGitreeStashes(t *testing.T) {
	r, repo := stashRepo(t)
	write(t, r, "b", "1\n")
	r.Git("stash", "push", "-q", "-m", "gitree: checkout other")
	write(t, r, "b", "2\n")
	r.Git("stash", "push", "-q", "-m", "mine")
	got, err := repo.GitreeStashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Ref != "stash@{1}" || got[0].Message != "gitree: checkout other" || got[0].Hash == "" {
		t.Fatalf("got %#v", got)
	}
}
