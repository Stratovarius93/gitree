package ui

import (
	"context"
	"errors"
	"strconv"

	"github.com/Stratovarius93/gitree/internal/git"
	tea "github.com/charmbracelet/bubbletea"
)

// netActionDoneMsg is the result of a pull or push; done is the success flash.
type netActionDoneMsg struct {
	op   string
	done string
	err  error
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// statusFetching refuses a pull or push while a fetch is updating the same
// remote-tracking refs.
const statusFetching = "fetch in progress — try again"

// askPull confirms a fast-forward pull of the current branch, refusing at
// once (in a dialog) what git would refuse: a fetch running, detached HEAD,
// no or gone upstream, known divergence, an operation in progress, and
// uncommitted changes in tracked files (where stashing is offered).
func (m Model) askPull() (tea.Model, tea.Cmd) {
	if m.acting {
		return m, m.flash("action", statusBusy)
	}
	t, err := m.pullTrack()
	if err != nil {
		return m.refuse("pull", err), nil
	}
	if busy, err := m.repo.InProgress(); err == nil && busy {
		return m.refuse("pull", git.ErrInProgress), nil
	}
	if files, err := m.repo.TrackedChanges(); err == nil && len(files) > 0 {
		return m.refuse("pull", git.ErrDirty), nil
	}
	text := "Pull " + plural(t.Behind, "commit") + " from " + t.Upstream + " into " + m.branch + "?"
	if t.Behind == 0 {
		text = "Pull from " + t.Upstream + " into " + m.branch + " (no new commits known — fetch with f)?"
	}
	return m.askConfirm("Pull", text, m.runPull(false))
}

// pullTrack is the current branch's upstream info, or why a pull is refused
// whatever the working tree holds.
func (m Model) pullTrack() (git.Track, error) {
	if m.fetching {
		return git.Track{}, errFetching
	}
	if m.branch == "" {
		return git.Track{}, git.ErrDetached
	}
	t, ok := m.track[m.branch]
	switch {
	case !ok:
		return t, git.ErrNoUpstream
	case t.Gone:
		return t, git.GoneError(t.Upstream)
	case t.Ahead > 0 && t.Behind > 0:
		return t, git.ErrDiverged
	}
	return t, nil
}

// askPullAutostash is the refusal dialog's `s`: a second question, accepted
// with y only, before stashing, pulling and restoring the changes.
func (m Model) askPullAutostash() (Model, tea.Cmd) {
	t, err := m.pullTrack()
	if err != nil {
		return m.refuse("pull", err), nil
	}
	files, err := m.repo.TrackedChanges()
	if err != nil {
		m.setStatus("action", "error: "+err.Error())
		return m, nil
	}
	pull := "pull " + plural(t.Behind, "commit") + " from " + t.Upstream
	if t.Behind == 0 {
		pull = "pull from " + t.Upstream + " (no new commits known)"
	}
	text := "Stash " + plural(len(files), "changed file") + ", " + pull + ", then restore your changes"
	for _, f := range files {
		if f.Staged { // git restores the autostash without --index
			text += " (staged changes will come back unstaged)"
			break
		}
	}
	text += "?"
	m.confirm = confirmState{title: "Pull", text: text, yes: m.runPull(true)}
	m.mode = ModeConfirm
	return m, nil
}

// runPull is the confirmed pull of the current branch; autostash stashes and
// restores tracked changes around it (only ever after the user asked).
func (m Model) runPull(autostash bool) func(Model) (Model, tea.Cmd) {
	branch := m.branch
	return func(m Model) (Model, tea.Cmd) {
		if m.fetching {
			return m.refuse("pull", errFetching), nil
		}
		return m, m.actNet("pull", "pulling…", func(ctx context.Context, r *git.Repo) (string, error) {
			if err := checkBranch(r, branch); err != nil {
				return "", err
			}
			pull := r.Pull
			if autostash {
				pull = r.PullAutostash
			}
			n, err := pull(ctx)
			if err != nil {
				return "", err
			}
			if n == 0 {
				return "already up to date", nil
			}
			return "pulled " + plural(n, "commit"), nil
		})
	}
}

// askPush confirms pushing the current branch. It always goes to a
// same-named branch: the upstream when it is one, otherwise a new branch on
// origin that becomes the upstream (never onto a differently named upstream).
// The confirmed plan is what runs; git refuses it if the branch changed.
//
// Uncommitted changes do not block a push: it only sends commits and never
// touches the working tree.
func (m Model) askPush() (tea.Model, tea.Cmd) {
	if m.acting {
		return m, m.flash("action", statusBusy)
	}
	if m.fetching {
		return m.refuse("push", errFetching), nil
	}
	t, ok := m.track[m.branch]
	plan, err := git.PlanPush(m.branch, t, ok)
	if err != nil {
		return m.refuse("push", err), nil
	}
	var text string
	switch plan.Kind {
	case git.PushNewBranch:
		text = "Push " + plan.Branch + " to " + plan.Dest() + " (new branch) and set upstream?"
	case git.PushRecreate:
		text = "Push " + plan.Branch + " to " + plan.Dest() + " (remote branch was deleted — recreate it)?"
	default:
		text = "Push " + plural(plan.Ahead, "commit") + " to " + plan.Dest() + "?"
	}
	return m.askConfirm("Push", text, func(m Model) (Model, tea.Cmd) {
		if m.fetching {
			return m.refuse("push", errFetching), nil
		}
		return m, m.actNet("push", "pushing…", func(ctx context.Context, r *git.Repo) (string, error) {
			return "pushed", r.Push(ctx, plan)
		})
	})
}

// checkBranch fails when the current branch is no longer the confirmed one.
func checkBranch(r *git.Repo, expected string) error {
	return onBranch(expected, func(*git.Repo) error { return nil })(r)
}

// actNet runs a network action in the background under the acting guard,
// showing busy while it runs.
func (m *Model) actNet(op, busy string, do func(context.Context, *git.Repo) (string, error)) tea.Cmd {
	m.acting = true
	m.actSeq++
	m.setStatus("action", busy)
	repo := m.repo
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), git.NetworkTimeout)
		defer cancel()
		done, err := do(ctx, repo)
		return netActionDoneMsg{op: op, done: done, err: err}
	}
}

// onNetActionDone flashes the result and reloads (a failed pull may still
// have fetched), keeping the selection by hash. A refusal made before git ran
// opens the refusal dialog; errors from git stay in the status bar.
func (m Model) onNetActionDone(msg netActionDoneMsg) (tea.Model, tea.Cmd) {
	m.acting = false
	reload := m.reloadAll()
	var ace *git.AutostashConflictError
	var ake *git.AutostashKeptError
	switch {
	case msg.err == nil:
		m.cancelJump()
		return m, tea.Batch(reload, m.flash("action", msg.done))
	case errors.As(msg.err, &ace):
		m.cancelJump()
		m.setStatus("action", "pulled "+plural(ace.Commits, "commit")+
			", but your changes conflicted when restored: they are safe in the stash — resolve in lazygit (L)")
		if m.mode == ModeList {
			m.confirm = confirmState{title: "Changes not restored",
				text: "Pulled, but your changes conflicted when restored. They are safe in the stash — resolve in lazygit (L)?",
				yes:  Model.openLazygit}
			m.mode = ModeConfirm
		}
	case errors.As(msg.err, &ake):
		m.setStatus("action", ake.Error())
		if m.mode == ModeList {
			m.confirm = confirmState{title: "Pull interrupted",
				text: "The pull did not finish. Your local changes are safe in the stash — restore them in lazygit (L)?",
				yes:  Model.openLazygit}
			m.mode = ModeConfirm
		}
	case refusedBeforeGit(msg.err):
		m = m.refuseOrStatus(msg.op, msg.err)
	case errors.Is(msg.err, git.ErrRejected), errors.Is(msg.err, git.ErrBranchChanged):
		m.setStatus("action", msg.err.Error())
	default:
		m.setStatus("action", "error: "+msg.op+": "+msg.err.Error())
	}
	return m, reload
}
