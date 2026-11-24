package git

import (
	"reflect"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func TestTracking(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit("base", map[string]string{"a": "1"})
	remote := r.Git("commit-tree", "-p", base, "-m", "remote", r.Git("rev-parse", base+"^{tree}"))
	r.Git("update-ref", "refs/remotes/origin/main", remote)
	r.Git("config", "remote.origin.url", ".")
	r.Git("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	r.Git("config", "branch.main.remote", "origin")
	r.Git("config", "branch.main.merge", "refs/heads/main")
	r.Commit("l1", map[string]string{"b": "1"})
	r.Commit("l2", map[string]string{"c": "1"})
	r.Branch("nouptream")

	got, err := openRepo(t, r.Dir).Tracking()
	if err != nil {
		t.Fatal(err)
	}
	want := Track{Upstream: "origin/main", Remote: "origin", UpstreamBranch: "main", Ahead: 2, Behind: 1}
	if !reflect.DeepEqual(got["main"], want) {
		t.Fatalf("got %+v, want %+v", got["main"], want)
	}
	if _, ok := got["nouptream"]; ok {
		t.Fatal("branch without upstream should be absent")
	}
}

func TestTrackingGone(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Git("config", "remote.origin.url", ".")
	r.Git("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	r.Git("config", "branch.main.remote", "origin")
	r.Git("config", "branch.main.merge", "refs/heads/main")

	got, err := openRepo(t, r.Dir).Tracking()
	if err != nil {
		t.Fatal(err)
	}
	want := Track{Upstream: "origin/main", Remote: "origin", UpstreamBranch: "main", Gone: true}
	if !reflect.DeepEqual(got["main"], want) {
		t.Fatalf("got %+v, want %+v", got["main"], want)
	}
}
