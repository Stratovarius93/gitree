// Package git wraps the git CLI for gitree.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ErrNotRepo is returned by Open when the path is not inside a git repository.
var ErrNotRepo = errors.New("not a git repository")

// CmdError describes a failed git invocation.
type CmdError struct {
	Args     []string
	Stdout   string // what git printed before failing (e.g. "CONFLICT (content): …")
	Stderr   string
	ExitCode int
}

func (e *CmdError) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", strings.Join(e.Args, " "), e.ExitCode, strings.TrimSpace(e.Stderr))
}

// Repo is an opened git repository.
type Repo struct {
	root string
}

// Root returns the repository top-level directory.
func (r *Repo) Root() string { return r.root }

// Open resolves the repository containing path.
func Open(path string) (*Repo, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	probe := &Repo{root: path}
	out, err := probe.run("rev-parse", "--show-toplevel")
	if err != nil {
		var ce *CmdError
		if errors.As(err, &ce) && strings.Contains(ce.Stderr, "not a git repository") {
			return nil, fmt.Errorf("%w: %s", ErrNotRepo, path)
		}
		return nil, err
	}
	return &Repo{root: strings.TrimSpace(out)}, nil
}

// run executes git -C <root> with args and returns stdout.
func (r *Repo) run(args ...string) (string, error) {
	return r.runAllow(nil, args...)
}

// runAllow is like run but treats the listed exit codes as success (for
// commands such as `git diff --no-index` that exit 1 when differences exist).
func (r *Repo) runAllow(okCodes []int, args ...string) (string, error) {
	return r.runCtx(context.Background(), nil, okCodes, args...)
}

// runCtx is runAllow with a context (the process is killed when it ends) and
// extra environment entries.
func (r *Repo) runCtx(ctx context.Context, env []string, okCodes []int, args ...string) (string, error) {
	full := append([]string{"-C", r.root}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// Detach from the controlling terminal (so ssh and credential helpers cannot
	// prompt on the TUI's tty and simply fail) and put git and its children in
	// their own process group, so cancelling kills ssh / git-remote-* too
	// instead of leaving them holding the output pipes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C"), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		ce := &CmdError{Args: args, Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: -1}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ce.ExitCode = ee.ExitCode()
			for _, c := range okCodes {
				if c == ce.ExitCode {
					return stdout.String(), nil
				}
			}
		} else {
			ce.Stderr = err.Error()
		}
		return "", ce
	}
	return stdout.String(), nil
}
