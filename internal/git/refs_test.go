package git

import (
	"reflect"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func TestListRefs(t *testing.T) {
	up := testutil.NewRepo(t)
	base := up.Commit("base", nil)
	up.Branch("master")
	up.Git("checkout", "-q", "-b", "feature/x")
	fx := up.Commit("feature", nil)
	up.Checkout("main")

	r := testutil.NewRepo(t)
	r.Git("remote", "add", "origin", up.Dir)
	r.Git("fetch", "-q", "origin")
	r.Git("remote", "set-head", "origin", "main") // creates refs/remotes/origin/HEAD
	r.Git("checkout", "-q", "-b", "main", "origin/main")
	local := r.Commit("local work", nil)
	r.Git("tag", "-a", "-m", "v0", "v0", base)
	r.Branch("Zed") // sorts after main ignoring case
	r.Tag("light")
	r.Git("tag", "tree-tag", "HEAD^{tree}")                        // not a commit: skipped
	r.Git("tag", "-a", "-m", "on tree", "tree-ann", "HEAD^{tree}") // peels to a tree: skipped
	r.Git("tag", "-a", "-m", "nested", "nested", "v0")             // tag of a tag: peeled to its commit
	r.Git("tag", "-a", "-m", "annotated", "v1.0", base)

	got, err := openRepo(t, r.Dir).ListRefs()
	if err != nil {
		t.Fatal(err)
	}
	want := []RefInfo{
		{Name: "main", Kind: LocalBranch, Hash: local},
		{Name: "Zed", Kind: LocalBranch, Hash: local},
		{Name: "origin/feature/x", Kind: RemoteBranch, Remote: "origin", Hash: fx},
		{Name: "origin/main", Kind: RemoteBranch, Remote: "origin", Hash: base},
		{Name: "origin/master", Kind: RemoteBranch, Remote: "origin", Hash: base},
		{Name: "light", Kind: Tag, Hash: local},
		{Name: "nested", Kind: Tag, Hash: base},
		{Name: "v0", Kind: Tag, Hash: base},
		{Name: "v1.0", Kind: Tag, Hash: base}, // peeled: the commit, not the tag object
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestListRefsEmptyRepo(t *testing.T) {
	r := testutil.NewRepo(t)
	got, err := openRepo(t, r.Dir).ListRefs()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
