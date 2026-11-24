package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	old := LookPath
	LookPath = fn
	t.Cleanup(func() { LookPath = old })
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func noDelta(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
}

func TestDiffFallsBackWithoutDelta(t *testing.T) {
	noDelta(t)
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "old\n"})
	h := tr.Commit("two", map[string]string{"a": "old\nnew line\n"})
	r, _ := Open(tr.Dir)
	out, err := r.Diff(DiffTarget{Hash: h}, "a", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripANSI(out), "+new line") || !strings.Contains(out, "\x1b[") {
		t.Fatalf("got %q", out)
	}
	root, err := r.Diff(DiffTarget{Hash: tr.Git("rev-list", "--max-parents=0", "HEAD")}, "", 100)
	if err != nil || !strings.Contains(stripANSI(root), "+old") {
		t.Fatalf("root: %q %v", root, err)
	}
}

func TestDiffUsesDeltaWhenWide(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "delta")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncat >/dev/null\nfor a in \"$@\"; do echo \"ARG:$a\"; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubLookPath(t, func(string) (string, error) { return fake, nil })
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "old\n"})
	h := tr.Commit("two", map[string]string{"a": "new\n"})
	r, _ := Open(tr.Dir)

	wide, err := r.Diff(DiffTarget{Hash: h}, "a", 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"ARG:--side-by-side", "ARG:--width=200", `ARG:--plus-style=syntax "#34381b"`} {
		if !strings.Contains(wide, s) {
			t.Errorf("missing %q in %q", s, wide)
		}
	}
	narrow, _ := r.Diff(DiffTarget{Hash: h}, "a", 120)
	if strings.Contains(narrow, "--side-by-side") || !strings.Contains(narrow, "ARG:--width=120") {
		t.Errorf("narrow: %q", narrow)
	}
}

func TestDiffUntrackedFile(t *testing.T) {
	noDelta(t)
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	os.WriteFile(filepath.Join(tr.Dir, "u.txt"), []byte("hello untracked\n"), 0o644)
	r, _ := Open(tr.Dir)
	out, err := r.Diff(DiffTarget{}, "u.txt", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripANSI(out), "+hello untracked") {
		t.Fatalf("got %q", out)
	}
}

func TestDiffWorkingTree(t *testing.T) {
	noDelta(t)
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	os.WriteFile(filepath.Join(tr.Dir, "a"), []byte("1\nwork\n"), 0o644)
	r, _ := Open(tr.Dir)
	out, err := r.Diff(DiffTarget{}, "a", 100)
	if err != nil || !strings.Contains(stripANSI(out), "+work") {
		t.Fatalf("%q %v", out, err)
	}
}
