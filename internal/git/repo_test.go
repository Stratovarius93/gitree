package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func TestOpenNotARepo(t *testing.T) {
	_, err := Open(t.TempDir())
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("want ErrNotRepo, got %v", err)
	}
}

func TestOpenSubdir(t *testing.T) {
	r := testutil.NewRepo(t)
	sub := filepath.Join(r.Dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(sub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(repo.Root())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}
