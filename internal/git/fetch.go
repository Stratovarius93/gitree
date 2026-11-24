package git

import (
	"context"
	"strings"
)

// Fetch updates all remotes and prunes deleted branches. It never prompts:
// git gets GIT_TERMINAL_PROMPT=0 (runCtx), credential helpers are told not to
// be interactive, and runCtx detaches git from the controlling terminal so
// ssh cannot ask for a passphrase either. The user's own ssh configuration
// (core.sshCommand, GIT_SSH_COMMAND, ...) is left untouched. ctx bounds the
// whole operation, children included.
func (r *Repo) Fetch(ctx context.Context) error {
	_, err := r.runCtx(ctx, nil, nil,
		"-c", "credential.interactive=never", "fetch", "--all", "--prune", "--quiet")
	if err == nil {
		return nil
	}
	return netError(ctx, err)
}

// firstErrorLine picks the line of git's stderr that explains the failure:
// the first "fatal:" or "error:" line, else the first non-empty one.
func firstErrorLine(stderr string) string {
	first := ""
	for _, l := range strings.Split(stderr, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "fatal:") || strings.HasPrefix(l, "error:") {
			return SanitizeLine(l)
		}
		if first == "" {
			first = SanitizeLine(l)
		}
	}
	return first
}
