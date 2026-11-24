package git

import (
	"strconv"
	"strings"
	"time"
)

// RefKind classifies a decoration on a commit.
type RefKind int

const (
	LocalBranch RefKind = iota
	RemoteBranch
	Tag
	DetachedHead
)

// Ref is a branch, tag or HEAD pointing at a commit. Name is "develop",
// "origin/develop", "v1.0" or "HEAD". Remote is the remote of a RemoteBranch
// ("origin"), "" otherwise.
type Ref struct {
	Name   string
	Kind   RefKind
	IsHead bool
	Remote string
}

// Commit is one entry of the log.
type Commit struct {
	Hash        string
	Parents     []string
	Refs        []Ref
	AuthorName  string
	AuthorEmail string
	Time        time.Time
	Subject     string
}

// Scope selects which refs the log walks.
type Scope int

const (
	// ScopeDefault walks local branches, their upstreams and HEAD.
	ScopeDefault Scope = iota
	// ScopeAll walks all branches, remotes, tags and HEAD (not stash).
	ScopeAll
)

// ParseDecorations parses git's %D output. A name is a remote branch when it
// is prefixed by "<remote>/" for one of remotes; remote HEAD symrefs are
// dropped. Control characters are stripped from the names.
func ParseDecorations(d string, remotes []string) []Ref {
	var refs []Ref
	for _, part := range strings.Split(d, ", ") {
		part = strings.TrimSpace(SanitizeLine(part))
		if part == "" {
			continue
		}
		if tag, ok := strings.CutPrefix(part, "tag: "); ok {
			refs = append(refs, Ref{Name: tag, Kind: Tag})
			continue
		}
		if left, right, ok := strings.Cut(part, " -> "); ok {
			if left == "HEAD" {
				refs = append(refs, newRef(right, remotes, true))
			}
			continue // "origin/HEAD -> origin/main" and similar symrefs
		}
		switch {
		case part == "HEAD":
			refs = append(refs, Ref{Name: "HEAD", Kind: DetachedHead, IsHead: true})
		case strings.HasPrefix(part, "refs/"):
			// refs/stash and other non-branch refs
		case isRemoteHead(part, remotes):
		default:
			refs = append(refs, newRef(part, remotes, false))
		}
	}
	return refs
}

// newRef classifies branch name: a remote branch when it starts with
// "<remote>/" (the longest matching remote wins), else a local branch.
func newRef(name string, remotes []string, head bool) Ref {
	ref := Ref{Name: name, Kind: LocalBranch, IsHead: head}
	for _, rm := range remotes {
		if strings.HasPrefix(name, rm+"/") && len(rm) > len(ref.Remote) {
			ref.Kind, ref.Remote = RemoteBranch, rm
		}
	}
	return ref
}

func isRemoteHead(name string, remotes []string) bool {
	for _, rm := range remotes {
		if name == rm+"/HEAD" {
			return true
		}
	}
	return false
}

// CurrentBranch returns the checked-out branch name, or "" when detached.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.run("branch", "--show-current")
	if err != nil {
		return "", err
	}
	return SanitizeLine(strings.TrimSpace(out)), nil
}

func (r *Repo) remotes() ([]string, error) {
	out, err := r.run("remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// logRefs returns the revisions to walk for scope (empty means nothing to
// log) and the --decorate-refs options that limit the labels to that scope:
// the default scope labels local branches, their existing upstreams, tags
// and HEAD; a stale remote branch on a visible commit stays unlabelled.
func (r *Repo) logRefs(scope Scope) (args, decorate []string, err error) {
	decorate = []string{"--decorate-refs=refs/heads/", "--decorate-refs=refs/tags/", "--decorate-refs=HEAD"}
	if scope == ScopeAll {
		decorate = append(decorate, "--decorate-refs=refs/remotes/")
		out, err := r.run("for-each-ref", "--count=1", "--format=%(refname)", "refs/heads", "refs/remotes", "refs/tags")
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(out) == "" {
			return nil, nil, nil // empty repo
		}
		args = []string{"--branches", "--remotes", "--tags"}
	} else {
		out, err := r.run("for-each-ref", "--format=%(refname)%00%(upstream)", "refs/heads", "refs/remotes")
		if err != nil {
			return nil, nil, err
		}
		existing := map[string]bool{}
		var upstreams []string
		for _, line := range strings.Split(out, "\n") {
			name, up, _ := strings.Cut(line, "\x00")
			if name == "" {
				continue
			}
			existing[name] = true
			if strings.HasPrefix(name, "refs/heads/") {
				args = append(args, name)
				if up != "" {
					upstreams = append(upstreams, up)
				}
			}
		}
		for _, up := range upstreams { // skip upstreams that no longer exist
			if existing[up] {
				args = append(args, up)
				decorate = append(decorate, "--decorate-refs="+up)
			}
		}
	}
	if _, err := r.run("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		args = append(args, "HEAD")
	}
	if len(args) == 0 {
		return nil, nil, nil
	}
	return args, decorate, nil
}

// Log returns commits for scope in topological order, newest first.
func (r *Repo) Log(scope Scope, skip, limit int) ([]Commit, error) {
	refs, decorate, err := r.logRefs(scope)
	if err != nil || refs == nil {
		return nil, err
	}
	remotes, err := r.remotes()
	if err != nil {
		return nil, err
	}
	args := []string{
		"log", "--topo-order", "--skip=" + strconv.Itoa(skip), "-n", strconv.Itoa(limit),
		"--pretty=format:%H%x00%P%x00%D%x00%aN%x00%aE%x00%at%x00%s%x1e",
	}
	args = append(args, decorate...)
	args = append(args, refs...)
	args = append(args, "--")
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x00")
		if len(f) < 7 {
			continue
		}
		ts, _ := strconv.ParseInt(f[5], 10, 64)
		commits = append(commits, Commit{
			Hash:        f[0],
			Parents:     strings.Fields(f[1]),
			Refs:        ParseDecorations(f[2], remotes),
			AuthorName:  SanitizeLine(f[3]),
			AuthorEmail: SanitizeLine(f[4]),
			Time:        time.Unix(ts, 0),
			Subject:     SanitizeLine(f[6]),
		})
	}
	return commits, nil
}
