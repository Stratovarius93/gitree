package git

import (
	"reflect"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func TestParseDecorations(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		remotes []string
		want    []Ref
	}{
		{"mixed", "HEAD -> develop, tag: v1.0, origin/develop, feature/x", []string{"origin"}, []Ref{
			{Name: "develop", Kind: LocalBranch, IsHead: true},
			{Name: "v1.0", Kind: Tag},
			{Name: "origin/develop", Kind: RemoteBranch, Remote: "origin"},
			{Name: "feature/x", Kind: LocalBranch},
		}},
		{"slash remote", "up/stream/fix/a, upstream/b", []string{"up/stream", "upstream"}, []Ref{
			{Name: "up/stream/fix/a", Kind: RemoteBranch, Remote: "up/stream"},
			{Name: "upstream/b", Kind: RemoteBranch, Remote: "upstream"},
		}},
		{"detached", "HEAD", nil, []Ref{{Name: "HEAD", Kind: DetachedHead, IsHead: true}}},
		{"origin HEAD", "origin/HEAD", []string{"origin"}, nil},
		{"origin HEAD arrow", "origin/HEAD -> origin/main", []string{"origin"}, nil},
		{"empty", "", []string{"origin"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDecorations(tc.in, tc.remotes)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func hashes(cs []Commit) map[string]bool {
	m := map[string]bool{}
	for _, c := range cs {
		m[c.Hash] = true
	}
	return m
}

func openRepo(t *testing.T, dir string) *Repo {
	t.Helper()
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestLogDefaultScope(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit("base", map[string]string{"a": "1"})
	r.Branch("feature/a")
	r.Checkout("feature/a")
	fa := r.Commit("on feature", map[string]string{"b": "1"})
	r.Checkout("main")
	other := r.Git("commit-tree", "-p", base, "-m", "other", r.Git("rev-parse", base+"^{tree}"))
	r.Git("update-ref", "refs/remotes/origin/other", other)

	repo := openRepo(t, r.Dir)
	def, err := repo.Log(ScopeDefault, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	h := hashes(def)
	if !h[base] || !h[fa] || h[other] {
		t.Fatalf("default scope wrong: %v", h)
	}
	all, err := repo.Log(ScopeAll, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hashes(all)[other] {
		t.Fatal("ScopeAll should include other")
	}
}

func TestLogDefaultIncludesUpstream(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit("base", map[string]string{"a": "1"})
	ahead := r.Git("commit-tree", "-p", base, "-m", "remote ahead", r.Git("rev-parse", base+"^{tree}"))
	r.Git("update-ref", "refs/remotes/origin/main", ahead)
	r.Git("config", "remote.origin.url", ".")
	r.Git("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	r.Git("config", "branch.main.remote", "origin")
	r.Git("config", "branch.main.merge", "refs/heads/main")

	cs, err := openRepo(t, r.Dir).Log(ScopeDefault, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hashes(cs)[ahead] {
		t.Fatalf("upstream commit missing: %+v", cs)
	}
}

func TestLogPaging(t *testing.T) {
	r := testutil.NewRepo(t)
	var hs []string
	for _, m := range []string{"c1", "c2", "c3", "c4", "c5"} {
		hs = append(hs, r.Commit(m, map[string]string{"f": m}))
	}
	cs, err := openRepo(t, r.Dir).Log(ScopeDefault, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Hash != hs[2] || cs[1].Hash != hs[1] {
		t.Fatalf("got %+v", cs)
	}
	if cs[0].Subject != "c3" || cs[0].AuthorName != "Test" || cs[0].AuthorEmail != "test@example.com" ||
		cs[0].Time.IsZero() || len(cs[0].Parents) != 1 || cs[0].Parents[0] != hs[1] {
		t.Fatalf("fields wrong: %+v", cs[0])
	}
}

func TestLogRefsAndMerge(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("feature/a")
	r.Checkout("feature/a")
	r.Commit("f", map[string]string{"b": "1"})
	r.Checkout("main")
	r.Commit("m", map[string]string{"c": "1"})
	mg := r.Merge("feature/a", "merge")
	r.Tag("v1.0")
	cs, err := openRepo(t, r.Dir).Log(ScopeDefault, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].Hash != mg || len(cs[0].Parents) != 2 {
		t.Fatalf("merge not first/2 parents: %+v", cs[0])
	}
	want := []Ref{{Name: "main", Kind: LocalBranch, IsHead: true}, {Name: "v1.0", Kind: Tag}}
	if len(cs[0].Refs) != 2 || !reflect.DeepEqual(cs[0].Refs[0], want[0]) || !reflect.DeepEqual(cs[0].Refs[1], want[1]) {
		t.Fatalf("refs %+v", cs[0].Refs)
	}
}

func TestLogEmptyRepo(t *testing.T) {
	r := testutil.NewRepo(t)
	cs, err := openRepo(t, r.Dir).Log(ScopeDefault, 0, 10)
	if err != nil || cs != nil {
		t.Fatalf("got %v, %v", cs, err)
	}
}

func TestCurrentBranch(t *testing.T) {
	r := testutil.NewRepo(t)
	hash := r.Commit("base", map[string]string{"a": "1"})
	repo := openRepo(t, r.Dir)
	if b, err := repo.CurrentBranch(); err != nil || b != "main" {
		t.Fatalf("got %q, %v", b, err)
	}
	r.Checkout(hash)
	if b, err := repo.CurrentBranch(); err != nil || b != "" {
		t.Fatalf("detached: got %q, %v", b, err)
	}
}

func refNames(cs []Commit) map[string]bool {
	out := map[string]bool{}
	for _, c := range cs {
		for _, r := range c.Refs {
			out[r.Name] = true
		}
	}
	return out
}

// TestLogDecorationsFollowScope: the default scope labels local branches,
// their upstreams, tags and HEAD, but not remote branches without a local
// copy that happen to sit on a visible commit; the all scope labels them.
func TestLogDecorationsFollowScope(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit("base", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	r.Tag("v1")
	r.Git("config", "remote.origin.url", ".")
	r.Git("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	r.Git("update-ref", "refs/remotes/origin/main", "HEAD")
	r.Git("update-ref", "refs/remotes/origin/stale", base)
	r.Git("update-ref", "refs/remotes/origin/HEAD", "HEAD")
	r.Git("config", "branch.main.remote", "origin")
	r.Git("config", "branch.main.merge", "refs/heads/main")

	repo := openRepo(t, r.Dir)
	def, err := repo.Log(ScopeDefault, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := refNames(def)
	for _, want := range []string{"main", "origin/main", "v1"} {
		if !got[want] {
			t.Errorf("default scope: %s missing from %v", want, got)
		}
	}
	if got["origin/stale"] {
		t.Errorf("default scope labels remote-only branch: %v", got)
	}
	if def[0].Refs[0].Name != "main" || !def[0].Refs[0].IsHead {
		t.Errorf("HEAD -> main lost: %+v", def[0].Refs)
	}

	all, err := repo.Log(ScopeAll, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	got = refNames(all)
	for _, want := range []string{"main", "origin/main", "origin/stale", "v1"} {
		if !got[want] {
			t.Errorf("all scope: %s missing from %v", want, got)
		}
	}

	r.Checkout(base)
	def, err = repo.Log(ScopeDefault, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var detached bool
	for _, c := range def {
		for _, ref := range c.Refs {
			detached = detached || (ref.Kind == DetachedHead && c.Hash == base)
		}
	}
	if !detached {
		t.Errorf("detached HEAD not labelled: %+v", def)
	}
}
