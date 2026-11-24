package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// ErrDirty is returned, without running git, when tracked files have
// uncommitted changes. gitree never stashes automatically.
var ErrDirty = errors.New("there are uncommitted changes in tracked files")

// ErrInProgress is returned, without running git, when a merge, cherry-pick,
// revert or rebase is in progress: it has to be finished (in lazygit) first.
var ErrInProgress = errors.New("a merge, cherry-pick, revert or rebase is in progress")

// ActionError is a failed repo-mutating git command. When Conflict is true the
// repo is deliberately left in the conflicted state for the user to resolve.
// InProgress means the failed merge/cherry-pick left an operation in progress
// (always with Conflict; alone e.g. for an empty cherry-pick). It is never
// aborted automatically.
//
// Blocked lists the files git refused to overwrite (nothing was changed):
// untracked files when BlockedUntracked, otherwise local changes.
type ActionError struct {
	Op               string
	Output           string
	Conflict         bool
	InProgress       bool
	Blocked          []string
	BlockedUntracked bool
}

func (e *ActionError) Error() string {
	if e.Conflict {
		return fmt.Sprintf("%s: conflicts: %s", e.Op, e.Output)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Output)
}

var hashRe = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// actionEnv guarantees no editor is ever launched (there is no tty).
var actionEnv = []string{"GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no"}

// ValidBranchName reports whether name is acceptable as a branch name
// (git check-ref-format --branch). Names starting with '-' are rejected.
func (r *Repo) ValidBranchName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	_, err := r.run("check-ref-format", "--branch", name)
	return err == nil
}

// inProgressMarkers are the files/dirs in the git dir that mean an operation
// is waiting to be continued or aborted (MERGE_AUTOSTASH: a pull with
// autostash that never restored the local changes).
var inProgressMarkers = []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "MERGE_AUTOSTASH"}

// InProgress reports whether a merge, cherry-pick, revert or rebase is in progress.
func (r *Repo) InProgress() (bool, error) {
	out, err := r.run("rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	dir := strings.TrimSpace(out)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.root, dir)
	}
	for _, name := range inProgressMarkers {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// ensureReady refuses to act while an operation is in progress or tracked
// files have uncommitted changes (in that order: a conflicted repo is also dirty).
func (r *Repo) ensureReady() error {
	busy, err := r.InProgress()
	if err != nil {
		return err
	}
	if busy {
		return ErrInProgress
	}
	return r.ensureClean()
}

func (r *Repo) ensureClean() error {
	dirty, err := r.HasTrackedChanges()
	if err != nil {
		return err
	}
	if dirty {
		return ErrDirty
	}
	return nil
}

// mutate runs a repo-changing git command. git's stdout and stderr both go
// into ActionError.Output.
func (r *Repo) mutate(op string, args ...string) error {
	_, err := r.runCtx(context.Background(), actionEnv, nil, args...)
	if err == nil {
		return nil
	}
	var ce *CmdError
	if !errors.As(err, &ce) {
		return &ActionError{Op: op, Output: err.Error()}
	}
	ae := &ActionError{Op: op, Output: joinOutput(ce.Stdout, ce.Stderr)}
	ae.Blocked, ae.BlockedUntracked = blockedFiles(ce.Stderr)
	return ae
}

// blockedFiles parses git's "The following untracked working tree files would
// be overwritten by …" / "Your local changes to the following files would be
// overwritten by …" refusal: the files, tab-indented on the next lines.
func blockedFiles(stderr string) (files []string, untracked bool) {
	in := false
	for _, l := range strings.Split(stderr, "\n") {
		switch {
		case strings.Contains(l, "would be overwritten by"):
			in = true
			untracked = strings.Contains(l, "untracked working tree files")
		case in && strings.HasPrefix(l, "\t"):
			files = append(files, strings.TrimSpace(l))
		default:
			in = false
		}
	}
	if len(files) == 0 {
		return nil, false
	}
	return files, untracked
}

// mutateMerging is mutate for merge and cherry-pick, the only operations that
// can stop half-way: a failure that leaves unmerged files is a conflict, and
// one that leaves the operation in progress is flagged. Both are left in
// place, never aborted.
func (r *Repo) mutateMerging(op string, args ...string) error {
	err := r.mutate(op, args...)
	var ae *ActionError
	if !errors.As(err, &ae) {
		return err
	}
	if files, derr := r.run("diff", "--name-only", "--diff-filter=U"); derr == nil && strings.TrimSpace(files) != "" {
		ae.Output = joinOutput(ae.Output, "conflicted files: "+strings.Join(strings.Fields(files), ", "))
		ae.Conflict = true
	}
	if busy, berr := r.InProgress(); berr == nil && busy {
		ae.InProgress = true
	}
	return ae
}

func joinOutput(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "\n")
}

// CheckoutKind is what a CheckoutTarget names.
type CheckoutKind int

const (
	// CheckoutLocal switches to the local branch Name.
	CheckoutLocal CheckoutKind = iota
	// CheckoutTrack creates a local branch tracking the remote branch Name
	// ("origin/x") and switches to it.
	CheckoutTrack
	// CheckoutDetached checks out commit Name (a hash) with a detached HEAD.
	CheckoutDetached
)

// CheckoutTarget is what a checkout switches to.
type CheckoutTarget struct {
	Kind CheckoutKind
	Name string
}

// validate refuses names git could misread before anything runs.
func (r *Repo) validate(t CheckoutTarget) error {
	switch t.Kind {
	case CheckoutLocal:
		if !r.ValidBranchName(t.Name) {
			return fmt.Errorf("invalid branch name: %q", t.Name)
		}
	case CheckoutTrack:
		if !r.ValidBranchName(t.Name) || !strings.Contains(t.Name, "/") {
			return fmt.Errorf("invalid remote branch: %q", t.Name)
		}
	case CheckoutDetached:
		if !hashRe.MatchString(t.Name) {
			return fmt.Errorf("invalid commit hash: %q", t.Name)
		}
		return nil
	default:
		return fmt.Errorf("unknown checkout kind %d", t.Kind)
	}
	return r.ensureUnambiguous(t.Name)
}

// switchTo runs the checkout of a validated target.
func (r *Repo) switchTo(t CheckoutTarget) error {
	switch t.Kind {
	case CheckoutTrack:
		return r.mutate("checkout --track "+t.Name, "checkout", "-q", "--track", t.Name)
	case CheckoutDetached:
		return r.mutate("checkout "+t.Name, "checkout", "-q", "--detach", t.Name)
	}
	return r.mutate("checkout "+t.Name, "checkout", "-q", t.Name, "--")
}

// Checkout switches to t, refusing with ErrInProgress or ErrDirty first. A
// checkout that moved HEAD but whose post-checkout hook failed is a
// *HookFailedError (the checkout happened).
func (r *Repo) Checkout(t CheckoutTarget) error {
	if err := r.validate(t); err != nil {
		return err
	}
	if err := r.ensureReady(); err != nil {
		return err
	}
	moved, hook, err := r.switchChecked(t)
	if moved && hook != "" {
		return &HookFailedError{Line: hook}
	}
	return err
}

// CheckoutBranch switches to a local branch.
func (r *Repo) CheckoutBranch(name string) error {
	return r.Checkout(CheckoutTarget{Kind: CheckoutLocal, Name: name})
}

// CheckoutRemote creates a local branch tracking remoteBranch ("origin/x") and
// switches to it.
func (r *Repo) CheckoutRemote(remoteBranch string) error {
	return r.Checkout(CheckoutTarget{Kind: CheckoutTrack, Name: remoteBranch})
}

// CheckoutDetached checks out a commit in detached HEAD state.
func (r *Repo) CheckoutDetached(hash string) error {
	return r.Checkout(CheckoutTarget{Kind: CheckoutDetached, Name: hash})
}

// Merge merges branch into the current branch (fast-forward when possible).
func (r *Repo) Merge(branch string) error {
	if !r.ValidBranchName(branch) {
		return fmt.Errorf("invalid branch name: %q", branch)
	}
	if err := r.ensureUnambiguous(branch); err != nil {
		return err
	}
	cur, err := r.safeCurrentBranch()
	if err != nil {
		return err
	}
	if cur == "" {
		return fmt.Errorf("cannot merge: %w", ErrDetached)
	}
	if cur == branch {
		return fmt.Errorf("cannot merge %s into itself", branch)
	}
	if err := r.ensureReady(); err != nil {
		return err
	}
	return r.mutateMerging("merge "+branch, "merge", "--no-edit", branch)
}

// CherryPick applies commit hash onto the current branch.
func (r *Repo) CherryPick(hash string) error {
	if !hashRe.MatchString(hash) {
		return fmt.Errorf("invalid commit hash: %q", hash)
	}
	cur, err := r.safeCurrentBranch()
	if err != nil {
		return err
	}
	if cur == "" {
		return fmt.Errorf("cannot cherry-pick: %w", ErrDetached)
	}
	if err := r.ensureReady(); err != nil {
		return err
	}
	return r.mutateMerging("cherry-pick "+hash, "cherry-pick", hash)
}

// CreateBranch creates branch name at the given commit/ref without switching.
func (r *Repo) CreateBranch(name, at string) error {
	if !r.ValidBranchName(name) {
		return fmt.Errorf("invalid branch name: %q", name)
	}
	if at == "" || strings.HasPrefix(at, "-") {
		return fmt.Errorf("invalid start point: %q", at)
	}
	return r.mutate("branch "+name, "branch", "--", name, at)
}

// HeadHash returns the full hash HEAD points at.
func (r *Repo) HeadHash() (string, error) {
	out, err := r.run("rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Message returns the full commit message of hash (subject and body).
func (r *Repo) Message(hash string) (string, error) {
	if !hashRe.MatchString(hash) {
		return "", fmt.Errorf("invalid commit hash: %q", hash)
	}
	out, err := r.run("show", "-s", "--format=%B", hash)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// BranchExists reports whether the local branch name exists.
func (r *Repo) BranchExists(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	_, err := r.run("rev-parse", "--verify", "-q", "refs/heads/"+name)
	return err == nil
}

// LocalBranchFor finds the local copy of remoteBranch ("origin/x"): the local
// branch whose upstream it is, else a local branch with the name after the
// remote. exists is false when there is none; name is then the branch
// `git checkout --track` would create.
func (r *Repo) LocalBranchFor(remoteBranch string) (name string, exists bool, err error) {
	track, err := r.Tracking()
	if err != nil {
		return "", false, err
	}
	var matches []string
	for local, t := range track {
		if t.Upstream == remoteBranch {
			matches = append(matches, local)
		}
	}
	if len(matches) > 0 {
		slices.Sort(matches) // deterministic when several track it
		return matches[0], true, nil
	}
	remotes, err := r.remotes()
	if err != nil {
		return "", false, err
	}
	name = remoteBranch
	for _, rem := range remotes {
		// the longest matching remote name wins
		if rest, ok := strings.CutPrefix(remoteBranch, rem+"/"); ok && rest != "" && len(rest) < len(name) {
			name = rest
		}
	}
	return name, r.BranchExists(name), nil
}
