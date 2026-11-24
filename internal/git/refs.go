package git

import (
	"sort"
	"strings"
)

// RefInfo is one branch or tag of the repo for the branch picker. Name is
// short ("develop", "origin/develop", "v1.0"); Remote is the remote of a
// RemoteBranch ("origin"), "" otherwise; Hash is the commit it points at
// (annotated tags are peeled).
type RefInfo struct {
	Name   string
	Kind   RefKind
	Remote string
	Hash   string
}

// ListRefs returns the local branches, remote branches (without the
// <remote>/HEAD symrefs) and tags, ordered local → remote → tag and
// alphabetically (ignoring case) within each group. Tags that do not lead
// to a commit (a tree or blob) are left out; a tag of a tag is peeled to
// its commit with rev-parse.
func (r *Repo) ListRefs() ([]RefInfo, error) {
	out, err := r.run("for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(objecttype)%00%(*objectname)%00%(*objecttype)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	remotes, err := r.remotes()
	if err != nil {
		return nil, err
	}
	var refs []RefInfo
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x00")
		if len(f) < 5 || f[0] == "" {
			continue
		}
		hash := f[1]
		ref0 := f[0] // raw, for git
		f[0] = SanitizeLine(f[0])
		switch {
		case f[2] == "commit":
		case f[4] == "commit":
			hash = f[3] // annotated tag: the commit it points at
		case f[4] == "tag": // tag of a tag: peel all the way
			out, err := r.run("rev-parse", "--verify", "-q", ref0+"^{commit}")
			if err != nil {
				continue
			}
			hash = strings.TrimSpace(out)
		default:
			continue // a tag of a tree or blob
		}
		switch {
		case strings.HasPrefix(f[0], "refs/heads/"):
			refs = append(refs, RefInfo{Name: strings.TrimPrefix(f[0], "refs/heads/"), Kind: LocalBranch, Hash: hash})
		case strings.HasPrefix(f[0], "refs/tags/"):
			refs = append(refs, RefInfo{Name: strings.TrimPrefix(f[0], "refs/tags/"), Kind: Tag, Hash: hash})
		case strings.HasPrefix(f[0], "refs/remotes/"):
			name := strings.TrimPrefix(f[0], "refs/remotes/")
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
			ref := newRef(name, remotes, false)
			ri := RefInfo{Name: name, Kind: RemoteBranch, Remote: ref.Remote, Hash: hash}
			if ri.Remote == "" { // a remote-tracking ref of a removed remote
				ri.Remote, _, _ = strings.Cut(name, "/")
			}
			refs = append(refs, ri)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		a, b := strings.ToLower(refs[i].Name), strings.ToLower(refs[j].Name)
		if a != b {
			return a < b
		}
		return refs[i].Name < refs[j].Name
	})
	return refs, nil
}
