package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func head(r *testutil.Repo) string { return r.Git("rev-parse", "HEAD") }

func TestCheckoutBranch(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("feature")
	repo := openRepo(t, r.Dir)
	if err := repo.CheckoutBranch("feature"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := repo.CurrentBranch(); cur != "feature" {
		t.Fatalf("current = %q", cur)
	}
	if err := repo.CheckoutBranch("nope"); err == nil {
		t.Fatal("want error for missing branch")
	}
	if err := repo.CheckoutBranch("--detach"); err == nil {
		t.Fatal("option-looking name must be refused")
	}
	if err := repo.CheckoutBranch(""); err == nil {
		t.Fatal("empty name must be refused")
	}
}

func TestCheckoutRemoteCreatesTracking(t *testing.T) {
	up := testutil.NewRepo(t)
	up.Commit("base", map[string]string{"a": "1"})
	up.Branch("x")
	clone := t.TempDir()
	up.Git("clone", "-q", up.Dir, clone)
	repo := openRepo(t, clone)
	if err := repo.CheckoutRemote("origin/x"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := repo.CurrentBranch(); cur != "x" {
		t.Fatalf("current = %q", cur)
	}
	tr, err := repo.Tracking()
	if err != nil {
		t.Fatal(err)
	}
	if tr["x"].Upstream != "origin/x" {
		t.Fatalf("upstream = %q", tr["x"].Upstream)
	}
}

func TestCheckoutDetached(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("one", map[string]string{"a": "1"})
	r.Commit("two", map[string]string{"a": "2"})
	repo := openRepo(t, r.Dir)
	if err := repo.CheckoutDetached(first); err != nil {
		t.Fatal(err)
	}
	if cur, _ := repo.CurrentBranch(); cur != "" {
		t.Fatalf("want detached, got %q", cur)
	}
	if head(r) != first {
		t.Fatal("HEAD not at commit")
	}
	if err := repo.CheckoutDetached("--force"); err == nil {
		t.Fatal("option-looking hash must be refused")
	}
}

func TestDirtyBlocks(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("other")
	r.Checkout("other")
	pick := r.Commit("o", map[string]string{"b": "1"})
	r.Checkout("main")
	repo := openRepo(t, r.Dir)

	// Untracked-only does not block.
	if err := os.WriteFile(filepath.Join(r.Dir, "untracked"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckoutBranch("other"); err != nil {
		t.Fatalf("untracked-only blocked: %v", err)
	}
	r.Checkout("main")

	if err := os.WriteFile(filepath.Join(r.Dir, "a"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := head(r)
	checks := map[string]error{
		"CheckoutBranch":   repo.CheckoutBranch("other"),
		"CheckoutRemote":   repo.CheckoutRemote("origin/other"),
		"CheckoutDetached": repo.CheckoutDetached(pick),
		"Merge":            repo.Merge("other"),
		"CherryPick":       repo.CherryPick(pick),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrDirty) {
			t.Errorf("%s: want ErrDirty, got %v", name, err)
		}
	}
	if head(r) != before {
		t.Fatal("HEAD moved")
	}
	// CreateBranch is allowed while dirty.
	if err := repo.CreateBranch("newb", before); err != nil {
		t.Fatal(err)
	}
}

func TestMergeFastForwardAndMergeCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("ff")
	r.Checkout("ff")
	tip := r.Commit("f", map[string]string{"f": "1"})
	r.Checkout("main")
	repo := openRepo(t, r.Dir)
	if err := repo.Merge("ff"); err != nil {
		t.Fatal(err)
	}
	if head(r) != tip {
		t.Fatal("expected fast-forward")
	}

	r.Branch("div")
	r.Checkout("div")
	r.Commit("d", map[string]string{"d": "1"})
	r.Checkout("main")
	r.Commit("m", map[string]string{"m": "1"})
	if err := repo.Merge("div"); err != nil {
		t.Fatal(err)
	}
	if n := r.Git("rev-list", "--parents", "-n1", "HEAD"); len(strings.Fields(n)) != 3 {
		t.Fatalf("expected merge commit, got %q", n)
	}
	if err := repo.Merge("main"); err == nil {
		t.Fatal("merging current branch into itself must be refused")
	}
}

func conflictRepo(t *testing.T) (*testutil.Repo, string) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "base"})
	r.Branch("other")
	r.Checkout("other")
	pick := r.Commit("other change", map[string]string{"a": "other"})
	r.Checkout("main")
	r.Commit("main change", map[string]string{"a": "main"})
	return r, pick
}

func TestMergeConflict(t *testing.T) {
	r, _ := conflictRepo(t)
	err := openRepo(t, r.Dir).Merge("other")
	var ae *ActionError
	if !errors.As(err, &ae) || !ae.Conflict {
		t.Fatalf("want conflict ActionError, got %v", err)
	}
	if ae.Output == "" || ae.Op == "" {
		t.Fatalf("empty Op/Output: %+v", ae)
	}
	if _, statErr := os.Stat(filepath.Join(r.Dir, ".git", "MERGE_HEAD")); statErr != nil {
		t.Fatal("merge must be left in progress")
	}
}

func TestCherryPick(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("other")
	r.Checkout("other")
	pick := r.Commit("picked", map[string]string{"p": "1"})
	r.Checkout("main")
	r.Commit("main diverges", map[string]string{"m": "1"})
	repo := openRepo(t, r.Dir)
	if err := repo.CherryPick(pick); err != nil {
		t.Fatal(err)
	}
	if r.Git("log", "-1", "--format=%s") != "picked" || head(r) == pick {
		t.Fatal("cherry-pick did not create new commit")
	}
	if err := repo.CherryPick("--abort"); err == nil {
		t.Fatal("option-looking hash must be refused")
	}
}

func TestCherryPickDetachedRefused(t *testing.T) {
	r := testutil.NewRepo(t)
	h := r.Commit("base", map[string]string{"a": "1"})
	r.Checkout(h)
	if err := openRepo(t, r.Dir).CherryPick(h); err == nil {
		t.Fatal("cherry-pick while detached must be refused")
	}
}

func TestCherryPickConflict(t *testing.T) {
	r, pick := conflictRepo(t)
	err := openRepo(t, r.Dir).CherryPick(pick)
	var ae *ActionError
	if !errors.As(err, &ae) || !ae.Conflict {
		t.Fatalf("want conflict ActionError, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(r.Dir, ".git", "CHERRY_PICK_HEAD")); statErr != nil {
		t.Fatal("cherry-pick must be left in progress")
	}
}

func TestCreateBranchAtCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("one", map[string]string{"a": "1"})
	r.Commit("two", map[string]string{"a": "2"})
	repo := openRepo(t, r.Dir)
	if err := repo.CreateBranch("topic", first); err != nil {
		t.Fatal(err)
	}
	if r.Git("rev-parse", "topic") != first {
		t.Fatal("branch not at commit")
	}
	if cur, _ := repo.CurrentBranch(); cur != "main" {
		t.Fatal("must not switch branch")
	}
	for _, bad := range [][2]string{{"topic", first}, {"bad..name", first}, {"", first}, {"-x", first}, {"ok", ""}, {"ok2", "--foo"}} {
		if err := repo.CreateBranch(bad[0], bad[1]); err == nil {
			t.Errorf("CreateBranch(%q,%q) should fail", bad[0], bad[1])
		}
	}
}

func TestValidBranchName(t *testing.T) {
	repo := openRepo(t, testutil.NewRepo(t).Dir)
	for name, want := range map[string]bool{"feature/x": true, "bad..name": false, "": false, "-x": false, "a b": false} {
		if got := repo.ValidBranchName(name); got != want {
			t.Errorf("ValidBranchName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestActionsRefusedWhileInProgress(t *testing.T) {
	r, pick := conflictRepo(t)
	repo := openRepo(t, r.Dir)
	if err := repo.Merge("other"); err == nil {
		t.Fatal("want conflict")
	}
	before := head(r)
	checks := map[string]error{
		"CheckoutBranch":   repo.CheckoutBranch("other"),
		"CheckoutRemote":   repo.CheckoutRemote("origin/other"),
		"CheckoutDetached": repo.CheckoutDetached(pick),
		"Merge":            repo.Merge("other"),
		"CherryPick":       repo.CherryPick(pick),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrInProgress) {
			t.Errorf("%s: want ErrInProgress, got %v", name, err)
		}
	}
	if head(r) != before {
		t.Fatal("HEAD moved")
	}
	// CreateBranch still works, and its failures are never conflicts.
	err := repo.CreateBranch("main", before)
	var ae *ActionError
	if !errors.As(err, &ae) {
		t.Fatalf("want ActionError, got %v", err)
	}
	if ae.Conflict || ae.InProgress {
		t.Fatalf("CreateBranch failure flagged as conflict/in-progress: %+v", ae)
	}
}

func TestInProgressDetectsRebaseDirs(t *testing.T) {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		r := testutil.NewRepo(t)
		r.Commit("base", map[string]string{"a": "1"})
		r.Branch("other")
		repo := openRepo(t, r.Dir)
		if busy, err := repo.InProgress(); err != nil || busy {
			t.Fatalf("clean repo: busy=%v err=%v", busy, err)
		}
		if err := os.Mkdir(filepath.Join(r.Dir, ".git", name), 0o755); err != nil {
			t.Fatal(err)
		}
		if busy, _ := repo.InProgress(); !busy {
			t.Fatalf("%s not detected", name)
		}
		if err := repo.CheckoutBranch("other"); !errors.Is(err, ErrInProgress) {
			t.Fatalf("%s: want ErrInProgress, got %v", name, err)
		}
	}
}

func TestConflictOutputIncludesStdout(t *testing.T) {
	r, _ := conflictRepo(t)
	err := openRepo(t, r.Dir).Merge("other")
	var ae *ActionError
	if !errors.As(err, &ae) || !ae.Conflict || !ae.InProgress {
		t.Fatalf("want conflict in progress, got %#v", err)
	}
	if !strings.Contains(ae.Output, "CONFLICT (content)") {
		t.Fatalf("stdout missing from output: %q", ae.Output)
	}
}

func TestEmptyCherryPickIsInProgress(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("other")
	r.Checkout("other")
	pick := r.Commit("change", map[string]string{"a": "2"})
	r.Checkout("main")
	r.Commit("same change", map[string]string{"a": "2"})
	err := openRepo(t, r.Dir).CherryPick(pick)
	var ae *ActionError
	if !errors.As(err, &ae) {
		t.Fatalf("want ActionError, got %v", err)
	}
	if ae.Conflict || !ae.InProgress {
		t.Fatalf("want in-progress without conflict, got %+v", ae)
	}
	if _, statErr := os.Stat(filepath.Join(r.Dir, ".git", "CHERRY_PICK_HEAD")); statErr != nil {
		t.Fatal("cherry-pick must be left in progress, not aborted")
	}
}

func TestHeadAndMessage(t *testing.T) {
	r := testutil.NewRepo(t)
	h := r.Commit("subject line\n\nbody text", map[string]string{"a": "1"})
	repo := openRepo(t, r.Dir)
	if got, err := repo.HeadHash(); err != nil || got != h {
		t.Fatalf("HeadHash = %q, %v", got, err)
	}
	if got, err := repo.Message(h); err != nil || got != "subject line\n\nbody text" {
		t.Fatalf("Message = %q, %v", got, err)
	}
	if _, err := repo.Message("--all"); err == nil {
		t.Fatal("option-looking hash must be refused")
	}
}

func TestLocalBranchFor(t *testing.T) {
	up := testutil.NewRepo(t)
	up.Commit("base", map[string]string{"a": "1"})
	up.Branch("x")
	up.Branch("y")
	up.Branch("z")
	clone := t.TempDir()
	up.Git("clone", "-q", up.Dir, clone)
	repo := openRepo(t, clone)
	// local "mine" tracks origin/x (different name)
	if _, err := repo.run("branch", "--track", "mine", "origin/x"); err != nil {
		t.Fatal(err)
	}
	// local "y" exists without tracking
	if _, err := repo.run("branch", "--no-track", "y", "origin/y"); err != nil {
		t.Fatal(err)
	}
	for remote, want := range map[string][2]any{
		"origin/x": {"mine", true},
		"origin/y": {"y", true},
		"origin/z": {"z", false},
	} {
		name, exists, err := repo.LocalBranchFor(remote)
		if err != nil || name != want[0] || exists != want[1] {
			t.Errorf("LocalBranchFor(%q) = %q, %v, %v; want %v", remote, name, exists, err, want)
		}
	}
}

func TestBranchExists(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("feature/x")
	repo := openRepo(t, r.Dir)
	if !repo.BranchExists("feature/x") || !repo.BranchExists("main") {
		t.Fatal("existing branch not found")
	}
	if repo.BranchExists("nope") || repo.BranchExists("-x") {
		t.Fatal("missing branch reported")
	}
}

// TestActionErrorsInEnglish: the errors gitree itself writes (not git's own
// output) are user-visible and in English, like the rest of the UI.
func TestActionErrorsInEnglish(t *testing.T) {
	r := testutil.NewRepo(t)
	hash := r.Commit("base", map[string]string{"a": "1"})
	repo := openRepo(t, r.Dir)
	checks := map[string]error{
		"invalid branch name":           repo.CheckoutBranch("-x"),
		"cannot merge main into itself": repo.Merge("main"),
		"invalid commit hash":           repo.CherryPick("zz"),
	}
	r.Checkout(hash)
	checks["cannot merge: HEAD is detached"] = repo.Merge("main")
	checks["cannot cherry-pick: HEAD is detached"] = repo.CherryPick(hash)
	for want, err := range checks {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	ae := &ActionError{Op: "merge x", Output: "CONFLICT", Conflict: true}
	if got := ae.Error(); got != "merge x: conflicts: CONFLICT" {
		t.Errorf("ActionError: %q", got)
	}
}
