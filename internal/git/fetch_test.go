package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchBringsRemoteCommit(t *testing.T) {
	origin := testutil.NewRepo(t)
	origin.Commit("first", map[string]string{"a.txt": "a"})
	cloneDir := t.TempDir()
	cmd := exec.Command("git", "clone", "-q", origin.Dir, cloneDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	newHash := origin.Commit("second", map[string]string{"b.txt": "b"})

	repo, err := Open(cloneDir)
	require.NoError(t, err)
	require.NoError(t, repo.Fetch(context.Background()))

	commits, err := repo.Log(ScopeAll, 0, 50)
	require.NoError(t, err)
	var found bool
	for _, c := range commits {
		found = found || c.Hash == newHash
	}
	assert.True(t, found, "fetched commit missing from log")
	track, err := repo.Tracking()
	require.NoError(t, err)
	assert.Equal(t, 1, track["main"].Behind)
}

// fakeSSHRepo returns a repo whose only remote is ssh://, with core.sshCommand
// set to a script running body (the script appends "ran" to the returned log).
func fakeSSHRepo(t *testing.T, body string) (*Repo, string) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := filepath.Join(dir, "ssh.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"ran $*\" >> "+log+"\n"+body+"\n"), 0o755))
	r.Git("remote", "add", "origin", "ssh://fake.invalid/x.git")
	r.Git("config", "core.sshCommand", script)
	repo, err := Open(r.Dir)
	require.NoError(t, err)
	return repo, log
}

func TestFetchRespectsCoreSSHCommand(t *testing.T) {
	unsetSSHEnv(t)
	repo, log := fakeSSHRepo(t, "exit 1")
	assert.Error(t, repo.Fetch(context.Background()))
	data, err := os.ReadFile(log)
	require.NoError(t, err, "the repo's core.sshCommand must have been used")
	assert.NotContains(t, string(data), "BatchMode")
}

func TestFetchKillsRunningFetchOnTimeout(t *testing.T) {
	unsetSSHEnv(t)
	repo, log := fakeSSHRepo(t, "sleep 30")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	err := repo.Fetch(ctx)
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Equal(t, "timed out", err.Error())
	assert.Less(t, elapsed, 4*time.Second, "must return within timeout + WaitDelay")
	_, statErr := os.Stat(log)
	assert.NoError(t, statErr, "the fetch was really running when killed")
}

func TestFetchErrorIsOneLine(t *testing.T) {
	unsetSSHEnv(t)
	repo, _ := fakeSSHRepo(t, "echo noise >&2; exit 1")
	err := repo.Fetch(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "\n")
	assert.Contains(t, err.Error(), "fatal:")
}

// unsetSSHEnv removes GIT_SSH* from the environment for the test (an empty
// value would not do: git treats it as a command).
func unsetSSHEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GIT_SSH_COMMAND", "GIT_SSH"} {
		t.Setenv(k, "x") // registers restore of the original value
		require.NoError(t, os.Unsetenv(k))
	}
}
