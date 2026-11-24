package git

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

// NetworkTimeout bounds one pull or push (and its ssh / remote helpers).
const NetworkTimeout = 60 * time.Second

var (
	// ErrDetached is returned when pull or push is asked for without a branch.
	ErrDetached = errors.New("HEAD is detached: check out a branch first")
	// ErrDiverged means the branch and its upstream both have commits: a
	// fast-forward pull is impossible and nothing was changed.
	ErrDiverged = errors.New("branches diverged: pull with lazygit (L)")
	// ErrRejected is a push the remote refused because it has new commits.
	ErrRejected = errors.New("push rejected: remote has new commits — pull first (p)")
	// ErrNothingToPush is returned, without running git, when the branch has
	// no commits its upstream lacks.
	ErrNothingToPush = errors.New("nothing to push")
	// ErrNoUpstream is returned by Pull for a branch that tracks nothing.
	ErrNoUpstream = errors.New("no upstream: push first with P")
	// ErrBranchChanged is returned by Push when the branch or its push
	// destination is no longer the one that was confirmed.
	ErrBranchChanged = errors.New("branch changed — try again")
)

// GoneUpstreamError reports an upstream branch that no longer exists.
type GoneUpstreamError struct{ Upstream string }

func (e *GoneUpstreamError) Error() string {
	return "upstream " + e.Upstream + " no longer exists"
}

// GoneError reports an upstream branch that no longer exists.
func GoneError(upstream string) error { return &GoneUpstreamError{Upstream: upstream} }

// netArgs are the options every network command gets so it never prompts
// (runCtx adds GIT_TERMINAL_PROMPT=0 and detaches from the tty).
var netArgs = []string{"-c", "credential.interactive=never"}

// netError turns a failed network command into a one-line error.
func netError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("timed out")
	}
	var ce *CmdError
	if errors.As(err, &ce) {
		if line := firstErrorLine(ce.Stderr); line != "" {
			return errors.New(line)
		}
	}
	return err
}

// currentTrack returns the current branch and its upstream info (ok false
// when it has none).
func (r *Repo) currentTrack() (branch string, t Track, ok bool, err error) {
	branch, err = r.safeCurrentBranch()
	if err != nil {
		return "", Track{}, false, err
	}
	if branch == "" {
		return "", Track{}, false, ErrDetached
	}
	track, err := r.tracking(false) // raw keys: a sibling named like it on screen cannot shadow it
	if err != nil {
		return "", Track{}, false, err
	}
	t, ok = track[branch]
	return branch, t, ok, nil
}

// AutostashConflictError is returned by PullAutostash when the pull
// (Commits commits) was applied but the stashed local changes conflicted when
// git restored them: git keeps them in the stash and leaves the conflicts in
// the working tree for the user to resolve.
type AutostashConflictError struct{ Commits int }

func (e *AutostashConflictError) Error() string {
	return "pulled " + strconv.Itoa(e.Commits) + " commit(s), but your changes conflicted when restored: they are safe in the stash — resolve in lazygit (L)"
}

// AutostashKeptError is returned by PullAutostash when the pull did not
// finish (Cause: timed out, killed, …) after git had stashed the local
// changes: gitree moved them into the stash (`git merge --quit`), where they
// wait to be restored.
type AutostashKeptError struct{ Cause error }

func (e *AutostashKeptError) Error() string {
	return "pull did not finish (" + e.Cause.Error() + ") — your local changes are safe in the stash (open lazygit with L to restore them)"
}

func (e *AutostashKeptError) Unwrap() error { return e.Cause }

// pullArgs is the pull command line: fast-forward only, never rebasing, and
// stashing local changes only when autostash is asked for explicitly.
func pullArgs(autostash bool) []string {
	stash := "--no-autostash"
	if autostash {
		stash = "--autostash"
	}
	return append(slices.Clone(netArgs), "pull", "--ff-only", "--no-rebase", stash, "--no-edit")
}

// Pull fast-forwards the current branch to its upstream (`git pull
// --ff-only`) and returns how many commits it brought (0: already up to
// date). It never merges, rebases or stashes: uncommitted changes in tracked
// files are ErrDirty.
func (r *Repo) Pull(ctx context.Context) (int, error) {
	return r.pull(ctx, false)
}

// PullAutostash is Pull for a dirty tree: git stashes the tracked changes,
// fast-forwards and restores them (`--autostash`). It is only run after the
// user explicitly confirmed it. When restoring conflicts the pull stays
// applied and the error is an *AutostashConflictError.
func (r *Repo) PullAutostash(ctx context.Context) (int, error) {
	return r.pull(ctx, true)
}

func (r *Repo) pull(ctx context.Context, autostash bool) (int, error) {
	_, t, ok, err := r.currentTrack()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrNoUpstream
	}
	if t.Gone {
		return 0, GoneError(SanitizeLine(t.Upstream))
	}
	if t.Ahead > 0 && t.Behind > 0 {
		return 0, ErrDiverged
	}
	if autostash {
		busy, err := r.InProgress()
		if err != nil {
			return 0, err
		}
		if busy {
			return 0, ErrInProgress
		}
	} else if err := r.ensureReady(); err != nil {
		return 0, err
	}
	before, err := r.HeadHash()
	if err != nil {
		return 0, err
	}
	stashBefore := r.stashTip()
	ctx, cancel := context.WithTimeout(ctx, NetworkTimeout)
	defer cancel()
	_, runErr := r.runCtx(ctx, actionEnv, nil, pullArgs(autostash)...)
	stashed := false
	if autostash {
		// A pull killed between stashing and restoring leaves the changes
		// only in MERGE_AUTOSTASH: move them into the stash.
		if err := r.saveLeftAutostash(); err != nil {
			return 0, err
		}
		stashed = r.autostashStored(stashBefore) // git (or we) kept the autostash
	}
	if runErr != nil {
		var ce *CmdError
		if ctx.Err() == nil && errors.As(runErr, &ce) && notFastForward(ce.Stderr) {
			runErr = ErrDiverged
		} else {
			runErr = netError(ctx, runErr)
		}
		if stashed {
			return 0, &AutostashKeptError{Cause: runErr}
		}
		return 0, runErr
	}
	n, err := r.countSince(before)
	if err != nil {
		return 0, err
	}
	if stashed {
		// git could not re-apply the autostash and stored it instead.
		return n, &AutostashConflictError{Commits: n}
	}
	return n, nil
}

// saveLeftAutostash stores a MERGE_AUTOSTASH left by an interrupted pull in
// the stash (`git merge --quit`); nothing to do without one.
func (r *Repo) saveLeftAutostash() error {
	if _, err := r.run("rev-parse", "-q", "--verify", "MERGE_AUTOSTASH"); err != nil {
		return nil
	}
	if _, err := r.run("merge", "--quit"); err != nil {
		return errors.New("pull did not finish and its autostash could not be saved: run git merge --quit (lazygit: L)")
	}
	return nil
}

// autostashStored reports whether refs/stash moved away from before to an
// entry git made from an autostash (not a stash pushed by another tool).
func (r *Repo) autostashStored(before string) bool {
	tip := r.stashTip()
	if tip == "" || tip == before {
		return false
	}
	out, err := r.run("log", "-g", "-1", "--format=%gs", "refs/stash", "--")
	return err == nil && strings.TrimSpace(out) == "autostash"
}

// stashTip is the hash refs/stash points at ("" without stashes).
func (r *Repo) stashTip() string {
	out, err := r.runAllow([]int{1}, "rev-parse", "-q", "--verify", "refs/stash")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// countSince is the number of commits HEAD moved forward from before.
func (r *Repo) countSince(before string) (int, error) {
	after, err := r.HeadHash()
	if err != nil || after == before {
		return 0, err
	}
	out, err := r.run("rev-list", "--count", before+".."+after)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

func notFastForward(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not possible to fast-forward") || strings.Contains(s, "diverging branches")
}

// PushKind says where a push goes relative to the branch's upstream.
type PushKind int

const (
	// PushUpstream pushes to the existing, same-named upstream branch.
	PushUpstream PushKind = iota
	// PushNewBranch pushes to a same-named branch on origin and makes it the
	// upstream: used without an upstream, or when the upstream is a local
	// branch or has a different name (pushing there would publish the
	// commits onto another branch, which push.default=simple refuses too).
	PushNewBranch
	// PushRecreate pushes to the upstream branch, which was deleted on the
	// remote, recreating it.
	PushRecreate
)

// PushPlan is what a push will do: Branch (local) goes to Target on Remote.
// Ahead is the number of commits to push for PushUpstream.
type PushPlan struct {
	Branch string
	Remote string
	Target string
	Kind   PushKind
	Ahead  int
}

// Dest is the remote-tracking name of the destination ("origin/feature").
func (p PushPlan) Dest() string { return p.Remote + "/" + p.Target }

// samePlace reports whether p and q push the same branch to the same place.
func (p PushPlan) samePlace(q PushPlan) bool {
	return p.Branch == q.Branch && p.Remote == q.Remote && p.Target == q.Target && p.Kind == q.Kind
}

// PlanPush decides how branch is pushed given its upstream info (ok false:
// none). It fails with ErrDetached for "" and ErrNothingToPush when the
// upstream already has every commit.
func PlanPush(branch string, t Track, ok bool) (PushPlan, error) {
	if branch == "" {
		return PushPlan{}, ErrDetached
	}
	newBranch := PushPlan{Branch: branch, Remote: "origin", Target: branch, Kind: PushNewBranch}
	switch {
	case !ok, t.Remote == "." || t.Remote == "", t.UpstreamBranch != branch:
		return newBranch, nil
	case t.Gone:
		return PushPlan{Branch: branch, Remote: t.Remote, Target: branch, Kind: PushRecreate}, nil
	case t.Ahead == 0:
		return PushPlan{}, ErrNothingToPush
	}
	return PushPlan{Branch: branch, Remote: t.Remote, Target: branch, Kind: PushUpstream, Ahead: t.Ahead}, nil
}

// PushPlan plans a push of the current branch from the repo's state now.
func (r *Repo) PushPlan() (PushPlan, error) {
	branch, t, ok, err := r.currentTrack()
	if err != nil {
		return PushPlan{}, err
	}
	return PlanPush(branch, t, ok)
}

// Push pushes the current branch as want (a plan the user confirmed). The plan
// is made again first: if the branch or its destination changed meanwhile it
// fails with ErrBranchChanged and pushes nothing. Branches always go to a
// same-named remote branch, with explicit refspecs. It never forces: a
// rejected push is ErrRejected.
func (r *Repo) Push(ctx context.Context, want PushPlan) error {
	branch, t, ok, err := r.currentTrack()
	if err != nil {
		return err
	}
	if branch != want.Branch {
		return ErrBranchChanged
	}
	plan, err := PlanPush(branch, t, ok)
	if err != nil {
		return err
	}
	if !plan.samePlace(want) {
		return ErrBranchChanged
	}
	refspec := "refs/heads/" + plan.Branch + ":refs/heads/" + plan.Target
	args := append(slices.Clone(netArgs), "push", "--porcelain")
	if plan.Kind == PushNewBranch {
		remotes, err := r.remotes()
		if err != nil {
			return err
		}
		if !slices.Contains(remotes, plan.Remote) {
			return errors.New("no remote '" + plan.Remote + "'")
		}
		args = append(args, "-u")
	}
	args = append(args, plan.Remote, refspec)
	ctx, cancel := context.WithTimeout(ctx, NetworkTimeout)
	defer cancel()
	if _, err := r.runCtx(ctx, actionEnv, nil, args...); err != nil {
		var ce *CmdError
		if ctx.Err() == nil && errors.As(err, &ce) {
			if rejected(ce.Stdout + ce.Stderr) {
				return ErrRejected
			}
			if reason, ok := remoteRejected(ce.Stdout, ce.Stderr); ok {
				return errors.New(reason)
			}
		}
		return netError(ctx, err)
	}
	return nil
}

// remoteRejected explains a push the remote refused for its own reasons (a
// hook, a protected branch) as one line: the reason from the porcelain
// "[remote rejected] (…)" status plus the first "remote:" message.
func remoteRejected(stdout, stderr string) (string, bool) {
	reason := ""
	found := false
	for _, l := range strings.Split(stdout, "\n") {
		_, rest, ok := strings.Cut(l, "[remote rejected]")
		if !ok {
			continue
		}
		found = true
		rest = strings.TrimSpace(rest)
		reason = strings.TrimSuffix(strings.TrimPrefix(rest, "("), ")")
		break
	}
	msg := ""
	for _, l := range strings.Split(stderr, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(l), "remote:"); ok {
			if t = strings.TrimSpace(t); t != "" {
				msg = t
				break
			}
		}
	}
	if !found {
		return "", false
	}
	out := "push rejected by remote"
	if reason != "" {
		out += " (" + reason + ")"
	}
	if msg != "" {
		out += ": " + msg
	}
	return SanitizeLine(out), true
}

func rejected(out string) bool {
	return strings.Contains(out, "[rejected]") || strings.Contains(out, "Updates were rejected")
}
