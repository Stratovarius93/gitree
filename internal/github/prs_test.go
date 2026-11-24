package github

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
)

func loadFixture(t *testing.T) map[string]PR {
	t.Helper()
	data, err := os.ReadFile("testdata/prs.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParsePRs(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestParsePRsStates(t *testing.T) {
	m := loadFixture(t)
	want := map[string]PRState{
		"a": PRDraft, "b": PRApproved, "c": PRChangesRequested,
		"d": PRMerged, "e": PRClosed, "f": PROpen,
	}
	for k, s := range want {
		if got := m[k].State; got != s {
			t.Errorf("%s: state %v, want %v", k, got, s)
		}
	}
	if p := m["a"]; p.Number != 1 || p.Title != "A" || p.Head != "a" || p.Base != "main" || p.URL == "" {
		t.Errorf("fields not populated: %+v", p)
	}
}

func TestParsePRsPrefersOpen(t *testing.T) {
	m := loadFixture(t)
	if m["g"].Number != 10 {
		t.Errorf("g: got #%d, want #10", m["g"].Number)
	}
	if m["h"].Number != 9 {
		t.Errorf("h: got #%d, want #9", m["h"].Number)
	}
}

func repoWithOrigin(t *testing.T, url string) string {
	r := testutil.NewRepo(t)
	r.Git("remote", "add", "origin", url)
	return r.Dir
}

func TestFetchPRsNotGitHub(t *testing.T) {
	dir := repoWithOrigin(t, "https://gitlab.com/x/y.git")
	if _, err := FetchPRs(dir); !errors.Is(err, ErrNotGitHub) {
		t.Fatalf("got %v, want ErrNotGitHub", err)
	}
}

func TestFetchPRsGhMissing(t *testing.T) {
	dir := repoWithOrigin(t, "git@github.com:x/y.git")
	old := LookPath
	LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { LookPath = old })
	if _, err := FetchPRs(dir); !errors.Is(err, ErrGhMissing) {
		t.Fatalf("got %v, want ErrGhMissing", err)
	}
}

func stubGh(t *testing.T, script string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := LookPath
	LookPath = func(string) (string, error) { return p, nil }
	t.Cleanup(func() { LookPath = old })
}

func TestFetchPRsNoAccess(t *testing.T) {
	dir := repoWithOrigin(t, "https://github.com/x/y.git")
	stubGh(t, `echo "Could not resolve to a Repository" >&2; exit 1`)
	_, err := FetchPRs(dir)
	if !errors.Is(err, ErrNoAccess) {
		t.Fatalf("got %v, want ErrNoAccess", err)
	}
}

func TestFetchPRsSuccess(t *testing.T) {
	dir := repoWithOrigin(t, "https://github.com/x/y.git")
	stubGh(t, `echo '[{"number":3,"title":"T","state":"OPEN","isDraft":false,"reviewDecision":"","headRefName":"x","baseRefName":"main","url":"u"}]'`)
	m, err := FetchPRs(dir)
	if err != nil || m["x"].Number != 3 {
		t.Fatalf("got %v, %v", m, err)
	}
}
