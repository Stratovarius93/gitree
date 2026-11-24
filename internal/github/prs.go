// Package github loads pull request status through the gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// PRState is the display state of a pull request.
type PRState int

const (
	PROpen PRState = iota
	PRDraft
	PRApproved
	PRChangesRequested
	PRMerged
	PRClosed
)

// PR is a pull request, reduced to what the UI needs.
type PR struct {
	Number     int
	Title, URL string
	Head, Base string
	State      PRState
}

var (
	ErrNotGitHub = errors.New("origin is not a GitHub remote")
	ErrGhMissing = errors.New("gh CLI not found")
	ErrNoAccess  = errors.New("gh cannot access this repository")
)

// LookPath locates executables; replaced in tests.
var LookPath = exec.LookPath

type ghPR struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	HeadRefName    string `json:"headRefName"`
	BaseRefName    string `json:"baseRefName"`
	URL            string `json:"url"`
}

func stateOf(g ghPR) PRState {
	switch g.State {
	case "MERGED":
		return PRMerged
	case "CLOSED":
		return PRClosed
	}
	switch {
	case g.IsDraft:
		return PRDraft
	case g.ReviewDecision == "APPROVED":
		return PRApproved
	case g.ReviewDecision == "CHANGES_REQUESTED":
		return PRChangesRequested
	}
	return PROpen
}

// ParsePRs parses `gh pr list --json` output into a map keyed by head branch.
// When a branch has several PRs, an open one wins; otherwise the highest number.
func ParsePRs(data []byte) (map[string]PR, error) {
	var list []ghPR
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	open := map[string]bool{}
	out := map[string]PR{}
	for _, g := range list {
		isOpen := g.State == "OPEN"
		if cur, ok := out[g.HeadRefName]; ok {
			if open[g.HeadRefName] && !isOpen {
				continue
			}
			if open[g.HeadRefName] == isOpen && cur.Number > g.Number {
				continue
			}
		}
		open[g.HeadRefName] = isOpen
		out[g.HeadRefName] = PR{
			Number: g.Number, Title: g.Title, URL: g.URL,
			Head: g.HeadRefName, Base: g.BaseRefName, State: stateOf(g),
		}
	}
	return out, nil
}

// Timeout bounds one FetchPRs call so a stuck gh cannot hang the app.
const Timeout = 20 * time.Second

// FetchPRs loads pull requests for the repo at repoRoot via gh, within Timeout.
func FetchPRs(repoRoot string) (map[string]PR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return FetchPRsContext(ctx, repoRoot)
}

// FetchPRsContext is FetchPRs bounded by ctx. gh is told never to prompt.
func FetchPRsContext(ctx context.Context, repoRoot string) (map[string]PR, error) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1", "LC_ALL=C")
	git := exec.CommandContext(ctx, "git", "-C", repoRoot, "remote", "get-url", "origin")
	git.Env = env
	url, err := git.Output()
	if err != nil || !strings.Contains(string(url), "github.com") {
		return nil, ErrNotGitHub
	}
	gh, err := LookPath("gh")
	if err != nil {
		return nil, ErrGhMissing
	}
	cmd := exec.CommandContext(ctx, gh, "pr", "list", "--state", "all", "--limit", "200",
		"--json", "number,title,state,isDraft,reviewDecision,headRefName,baseRefName,url")
	cmd.Dir = repoRoot
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNoAccess, strings.TrimSpace(stderr.String()))
	}
	return ParsePRs(stdout.Bytes())
}
