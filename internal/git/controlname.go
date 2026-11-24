package git

import (
	"errors"
	"strings"
)

// ErrControlName is returned when an action would involve a branch whose real
// name contains control characters. Names are sanitized for display, so the
// name shown could be another branch's; gitree refuses instead of guessing.
var ErrControlName = errors.New("branch name contains control characters: use git directly")

// CurrentBranchRaw returns the checked-out branch name exactly as git reports
// it, "" when detached. Use it for comparisons and git arguments, never for
// display.
func (r *Repo) CurrentBranchRaw() (string, error) {
	out, err := r.run("branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// safeCurrentBranch returns the current branch, or ErrControlName when its
// real name differs from the sanitized one.
func (r *Repo) safeCurrentBranch() (string, error) {
	cur, err := r.CurrentBranchRaw()
	if err != nil {
		return "", err
	}
	if SanitizeLine(cur) != cur {
		return "", ErrControlName
	}
	return cur, nil
}

// ensureUnambiguous fails with ErrControlName when some local or remote branch
// whose real name contains control characters is displayed as name.
func (r *Repo) ensureUnambiguous(name string) error {
	out, err := r.run("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return err
	}
	for _, ref := range strings.Split(out, "\n") {
		if clean := SanitizeLine(ref); clean != ref && clean == name {
			return ErrControlName
		}
	}
	return nil
}
