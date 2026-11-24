package git

import (
	"strconv"
	"strings"
)

// Track describes how a local branch relates to its upstream. Upstream is
// the short name ("origin/feature/x"); Remote is the upstream's remote
// ("origin", or "." for a local branch); UpstreamBranch is the branch name on
// that remote ("feature/x"). Gone means the upstream branch no longer exists
// (deleted on the remote and pruned).
type Track struct {
	Upstream       string
	Remote         string
	UpstreamBranch string
	Ahead          int
	Behind         int
	Gone           bool
}

// Tracking returns upstream info keyed by local branch name. Branches without
// an upstream are omitted.
func (r *Repo) Tracking() (map[string]Track, error) { return r.tracking(true) }

// tracking is Tracking with names sanitized for display or, with sanitize
// false, raw (for comparisons and git arguments).
func (r *Repo) tracking(sanitize bool) (map[string]Track, error) {
	out, err := r.run("for-each-ref",
		"--format=%(refname:short)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(upstream:remoteref)%00%(upstream:remotename)", "refs/heads")
	if err != nil {
		return nil, err
	}
	res := map[string]Track{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x00")
		for i := range f {
			if sanitize {
				f[i] = SanitizeLine(f[i])
			}
		}
		if len(f) < 5 || f[0] == "" || f[1] == "" {
			continue
		}
		t := Track{Upstream: f[1], Remote: f[4], UpstreamBranch: strings.TrimPrefix(f[3], "refs/heads/")}
		for _, p := range strings.Split(f[2], ",") {
			fields := strings.Fields(p)
			if len(fields) == 1 && fields[0] == "gone" {
				t.Gone = true
			}
			if len(fields) != 2 {
				continue
			}
			n, _ := strconv.Atoi(fields[1])
			switch fields[0] {
			case "ahead":
				t.Ahead = n
			case "behind":
				t.Behind = n
			}
		}
		res[f[0]] = t
	}
	return res, nil
}
