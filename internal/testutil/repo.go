// Package testutil provides helpers for tests that need a real git repo.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo is a temporary git repository.
type Repo struct {
	Dir string
	t   *testing.T
}

// NewRepo creates an initialised repo in a temp dir with a local identity.
func NewRepo(t *testing.T) *Repo {
	t.Helper()
	r := &Repo{Dir: t.TempDir(), t: t}
	r.Git("init", "-q", "-b", "main")
	r.Git("config", "init.defaultBranch", "main")
	r.Git("config", "user.name", "Test")
	r.Git("config", "user.email", "test@example.com")
	r.Git("config", "commit.gpgsign", "false")
	return r
}

// Git runs git in the repo and returns trimmed stdout only; fails the test on
// error, including stderr in the message.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// Commit writes files, stages everything and commits; returns the hash.
// With no files, an empty commit is created.
func (r *Repo) Commit(msg string, files map[string]string) string {
	r.t.Helper()
	for name, content := range files {
		p := filepath.Join(r.Dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", msg)
	return r.Git("rev-parse", "HEAD")
}

// Branch creates a branch at HEAD without switching to it.
func (r *Repo) Branch(name string) { r.t.Helper(); r.Git("branch", name) }

// Checkout switches to ref.
func (r *Repo) Checkout(ref string) { r.t.Helper(); r.Git("checkout", "-q", ref) }

// Merge merges branch with a merge commit (--no-ff) and returns its hash.
func (r *Repo) Merge(branch, msg string) string {
	r.t.Helper()
	r.Git("merge", "-q", "--no-ff", "-m", msg, branch)
	return r.Git("rev-parse", "HEAD")
}

// Tag creates a lightweight tag at HEAD.
func (r *Repo) Tag(name string) { r.t.Helper(); r.Git("tag", name) }

// NewOrigin creates a bare repo with one commit ("base") on main, made through
// a seed clone, and returns the bare repo's path and that seed clone (an
// "other developer" who can push more commits).
func NewOrigin(t *testing.T) (bare string, seed *Repo) {
	t.Helper()
	bare = t.TempDir()
	seed = NewRepo(t)
	seed.Git("init", "-q", "--bare", "-b", "main", bare)
	seed.Commit("base", map[string]string{"base.txt": "base"})
	seed.Git("remote", "add", "origin", bare)
	seed.Git("push", "-q", "-u", "origin", "main")
	return bare, seed
}

// Clone clones url into a temp dir and gives it a local identity.
func Clone(t *testing.T, url string) *Repo {
	t.Helper()
	r := &Repo{Dir: t.TempDir(), t: t}
	r.Git("clone", "-q", url, r.Dir)
	r.Git("config", "user.name", "Test")
	r.Git("config", "user.email", "test@example.com")
	r.Git("config", "commit.gpgsign", "false")
	return r
}
