package git

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func byPath(fs []FileChange) map[string]FileChange {
	m := map[string]FileChange{}
	for _, f := range fs {
		m[f.Path] = f
	}
	return m
}

func TestWorkingFiles(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n", "b": "2\n", "d": "d1\nd2\nd3\nd4\n"})
	if err := os.WriteFile(filepath.Join(tr.Dir, "a"), []byte("1\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tr.Dir, "b")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tr.Dir, "c"), []byte("x\ny\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr.Git("mv", "d", "e")
	r, err := Open(tr.Dir)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := r.WorkingFiles()
	if err != nil {
		t.Fatal(err)
	}
	m := byPath(fs)
	if len(fs) != 4 {
		t.Fatalf("got %+v", fs)
	}
	if m["a"].Status != 'M' || m["a"].Added != 1 {
		t.Errorf("a: %+v", m["a"])
	}
	if m["b"].Status != 'D' || m["b"].Deleted != 1 {
		t.Errorf("b: %+v", m["b"])
	}
	if m["c"].Status != '?' {
		t.Errorf("c: %+v", m["c"])
	}
	if e := m["e"]; e.Status != 'R' || e.OldPath != "d" {
		t.Errorf("e: %+v", e)
	}
}

func TestHasTrackedChanges(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	r, _ := Open(tr.Dir)
	has, err := r.HasTrackedChanges()
	if err != nil || has {
		t.Fatalf("clean: %v %v", has, err)
	}
	os.WriteFile(filepath.Join(tr.Dir, "u"), []byte("u"), 0o644)
	if has, _ := r.HasTrackedChanges(); has {
		t.Fatal("untracked only should be false")
	}
	os.WriteFile(filepath.Join(tr.Dir, "a"), []byte("changed\n"), 0o644)
	if has, _ := r.HasTrackedChanges(); !has {
		t.Fatal("modified should be true")
	}
}

func TestCommitFiles(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"y": "1\n"})
	h := tr.Commit("two", map[string]string{"x": "1\n2\n3\n", "y": "1\n2\n"})
	r, _ := Open(tr.Dir)
	fs, err := r.CommitFiles(h)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileChange{{Status: 'A', Path: "x", Added: 3}, {Status: 'M', Path: "y", Added: 1}}
	if !reflect.DeepEqual(fs, want) {
		t.Fatalf("got %+v want %+v", fs, want)
	}
}

func TestCommitFilesRootCommit(t *testing.T) {
	tr := testutil.NewRepo(t)
	h := tr.Commit("init", map[string]string{"a": "1\n", "b": "2\n"})
	r, _ := Open(tr.Dir)
	fs, err := r.CommitFiles(h)
	if err != nil || len(fs) != 2 {
		t.Fatalf("%+v %v", fs, err)
	}
	for _, f := range fs {
		if f.Status != 'A' {
			t.Errorf("%+v", f)
		}
	}
}

func TestCommitFilesMergeCommit(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	tr.Branch("feat")
	tr.Checkout("feat")
	tr.Commit("z", map[string]string{"z": "z\n"})
	tr.Checkout("main")
	tr.Commit("m", map[string]string{"m": "m\n"})
	h := tr.Merge("feat", "merge")
	r, _ := Open(tr.Dir)
	fs, err := r.CommitFiles(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].Path != "z" || fs[0].Status != 'A' {
		t.Fatalf("%+v", fs)
	}
}

func TestShowHeader(t *testing.T) {
	tr := testutil.NewRepo(t)
	h := tr.Commit("subject\n\nbody text", map[string]string{"a": "1\n"})
	r, _ := Open(tr.Dir)
	out, err := r.ShowHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{h, "Test <test@example.com>", "body text"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in %q", s, out)
		}
	}
}

func TestTrackedChanges(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n", "b": "1\n"})
	r, _ := Open(tr.Dir)
	fs, err := r.TrackedChanges()
	if err != nil || len(fs) != 0 {
		t.Fatalf("clean: %v %v", fs, err)
	}
	os.WriteFile(filepath.Join(tr.Dir, "u"), []byte("u"), 0o644)
	os.WriteFile(filepath.Join(tr.Dir, "a"), []byte("changed\n"), 0o644)
	os.Remove(filepath.Join(tr.Dir, "b"))
	fs, err = r.TrackedChanges()
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(fs)
	if len(fs) != 2 || got["a"].Status != 'M' || got["b"].Status != 'D' {
		t.Fatalf("got %+v", fs)
	}
}
