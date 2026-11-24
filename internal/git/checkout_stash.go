package git

import (
	"context"
	"errors"
	"strings"
)

// stashPrefix starts the message of every stash entry gitree makes.
const stashPrefix = "gitree: checkout "

// HookFailedError is a checkout that happened (HEAD moved) but whose
// post-checkout hook failed; Line is the hook's first error line.
type HookFailedError struct{ Line string }

func (e *HookFailedError) Error() string { return "post-checkout hook failed: " + e.Line }

// CheckoutRestoredError is a checkout that failed after gitree stashed the
// local changes: HEAD did not move and they were restored on the original
// branch. Err is git's refusal (often an *ActionError). Kept means the stash
// entry could not be dropped safely and is still in the stash.
type CheckoutRestoredError struct {
	Err  error
	Kept bool
}

func (e *CheckoutRestoredError) Error() string {
	s := e.Err.Error() + " — your changes were restored"
	if e.Kept {
		s += " (the stash changed meanwhile, so the entry was kept)"
	}
	return s
}

func (e *CheckoutRestoredError) Unwrap() error { return e.Err }

// StashConflictError means the checkout of Target succeeded but the stashed
// changes conflicted when restored: the entry (Entry, commit Hash) stays in
// the stash and the conflicts are left in the working tree. Hook is set when
// the post-checkout hook failed too.
type StashConflictError struct {
	Target string
	Entry  string
	Hash   string
	Hook   string
}

func (e *StashConflictError) Error() string {
	s := "checked out " + e.Target + ", but your changes conflicted when restored: they are safe in the stash (" +
		e.Entry + ") — resolve in lazygit (L)"
	if e.Hook != "" {
		s += "; post-checkout hook failed: " + e.Hook
	}
	return s
}

// StashKeptError means the stashed changes could not be restored (Cause):
// they are still in the stash entry Entry (commit Hash), untouched.
type StashKeptError struct {
	Cause error
	Entry string
	Hash  string
}

func (e *StashKeptError) Error() string {
	return e.Cause.Error() + " — your changes are safe in the stash (" + e.Entry + "; open lazygit with L to restore them)"
}

func (e *StashKeptError) Unwrap() error { return e.Cause }

// CheckoutResult is a successful CheckoutWithStash: Unstaged when the staged
// state could not be restored (the changes came back unstaged), Hook the
// failed post-checkout hook's first error line, Kept when the stash entry
// was restored but left in the stash (the stash changed meanwhile).
type CheckoutResult struct {
	Unstaged bool
	Hook     string
	Kept     bool
}

// headState identifies where HEAD is: its symbolic ref and commit.
func (r *Repo) headState() string {
	ref, _ := r.runAllow([]int{1}, "symbolic-ref", "-q", "HEAD")
	h, _ := r.HeadHash()
	return strings.TrimSpace(ref) + " " + h
}

// switchChecked is switchTo that tells a failed checkout (HEAD did not move:
// moved false) from a checkout whose post-checkout hook failed (moved true,
// hook its first error line).
func (r *Repo) switchChecked(t CheckoutTarget) (moved bool, hook string, err error) {
	before := r.headState()
	err = r.switchTo(t)
	if err == nil {
		return true, "", nil
	}
	if r.headState() == before {
		return false, "", err
	}
	return true, hookLine(err), nil
}

// hookLine is the first line of a failed command's output.
func hookLine(err error) string {
	out := err.Error()
	var ae *ActionError
	if errors.As(err, &ae) {
		out = ae.Output
	}
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return SanitizeLine(l)
		}
	}
	return "unknown error"
}

// CheckoutWithStash checks out t with uncommitted changes in tracked files:
// it stashes them (`git stash push`, untracked files stay where they are),
// checks out t and restores them (`git stash apply --index`, then drops the
// entry while it is still stash@{0}). It is only run after the user
// explicitly confirmed it.
//
// When the checkout fails without moving HEAD the changes are restored on the
// original branch and the error is a *CheckoutRestoredError. When HEAD moved
// (a failing post-checkout hook) the changes are restored on the new branch
// and the hook is reported. When restoring conflicts it is a
// *StashConflictError (the entry is kept). When the staged state cannot be
// restored the changes come back unstaged. A stash entry is never dropped on
// a failure path.
func (r *Repo) CheckoutWithStash(t CheckoutTarget) (CheckoutResult, error) {
	var res CheckoutResult
	if err := r.validate(t); err != nil {
		return res, err
	}
	busy, err := r.InProgress()
	if err != nil {
		return res, err
	}
	if busy {
		return res, ErrInProgress
	}
	if _, err := r.safeCurrentBranch(); err != nil {
		return res, err
	}
	name := SanitizeLine(t.Name)
	entry := stashPrefix + name
	before := r.stashTip()
	_, perr := r.runCtx(context.Background(), actionEnv, nil, "stash", "push", "-q", "-m", entry)
	stash := r.stashTip()
	created := stash != "" && stash != before
	if perr != nil {
		if created {
			return res, &StashKeptError{Cause: errors.New("git stash failed (" + hookLine(perr) + ")"), Entry: entry, Hash: stash}
		}
		return res, &ActionError{Op: "stash", Output: hookLine(perr) + " — nothing was stashed or changed"}
	}
	if !created {
		// nothing to stash (the tree became clean): a plain checkout
		err := r.Checkout(t)
		var he *HookFailedError
		if errors.As(err, &he) {
			res.Hook = he.Line
			return res, nil
		}
		return res, err
	}
	moved, hook, err := r.switchChecked(t)
	if !moved {
		kept, rerr := r.restoreStash(stash, true)
		if rerr != nil {
			return res, &StashKeptError{Cause: errors.New("checkout failed (" + err.Error() + ") and your changes could not be restored"),
				Entry: entry, Hash: stash}
		}
		return res, &CheckoutRestoredError{Err: err, Kept: kept}
	}
	res.Hook = hook
	conflict := &StashConflictError{Target: name, Entry: entry, Hash: stash, Hook: hook}
	failed := &StashKeptError{Cause: errors.New("checked out " + name + ", but restoring your changes failed"), Entry: entry, Hash: stash}
	kept, err := r.restoreStash(stash, true)
	if err != nil {
		if r.hasUnmerged() {
			return res, conflict
		}
		if !strings.Contains(err.Error(), "Try without --index") {
			return res, failed
		}
		// git applies the index first and changed nothing: restore unstaged
		if kept, err = r.restoreStash(stash, false); err != nil {
			if r.hasUnmerged() {
				return res, conflict
			}
			return res, failed
		}
		res.Unstaged = true
	}
	res.Kept = kept
	return res, nil
}

// restoreStash applies the stash entry hash (with its index when index is
// set) and then drops it, but only while it is still stash@{0}: another
// tool's entry is never dropped. kept reports an entry left in the stash.
func (r *Repo) restoreStash(hash string, index bool) (kept bool, err error) {
	args := []string{"stash", "apply", "-q"}
	if index {
		args = append(args, "--index")
	}
	if _, err := r.runCtx(context.Background(), actionEnv, nil, append(args, hash)...); err != nil {
		return false, errors.New(cmdOutput(err))
	}
	if r.stashTip() != hash {
		return true, nil
	}
	if _, err := r.runCtx(context.Background(), actionEnv, nil, "stash", "drop", "-q", "stash@{0}"); err != nil {
		return true, nil
	}
	return false, nil
}

// StashEntry is one stash entry: Ref ("stash@{1}"), its commit and message.
type StashEntry struct {
	Ref     string
	Hash    string
	Message string
}

// GitreeStashes lists the stash entries gitree made for a checkout (their
// message starts with "gitree: checkout "), wherever they are in the stash.
func (r *Repo) GitreeStashes() ([]StashEntry, error) {
	if r.stashTip() == "" {
		return nil, nil
	}
	out, err := r.run("stash", "list", "--format=%gd%x00%H%x00%gs")
	if err != nil {
		return nil, err
	}
	var es []StashEntry
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\x00")
		if len(f) != 3 {
			continue
		}
		// the reflog message is "On <branch>: <message>"
		msg := f[2]
		if _, rest, ok := strings.Cut(msg, ": "); ok && strings.HasPrefix(rest, stashPrefix) {
			msg = rest
		}
		if strings.HasPrefix(msg, stashPrefix) {
			es = append(es, StashEntry{Ref: f[0], Hash: f[1], Message: SanitizeLine(msg)})
		}
	}
	return es, nil
}

// hasUnmerged reports whether the index has unmerged (conflicted) paths.
func (r *Repo) hasUnmerged() bool {
	out, err := r.run("diff", "--name-only", "--diff-filter=U")
	return err == nil && strings.TrimSpace(out) != ""
}

// cmdOutput is a failed command's output, or the error itself.
func cmdOutput(err error) string {
	var ce *CmdError
	if errors.As(err, &ce) {
		if out := joinOutput(ce.Stdout, ce.Stderr); out != "" {
			return out
		}
	}
	return err.Error()
}
