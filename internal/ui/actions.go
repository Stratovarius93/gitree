package ui

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/Stratovarius93/gitree/internal/git"
	tea "github.com/charmbracelet/bubbletea"
)

// Status texts of the actions.
const (
	statusDirty      = "you have uncommitted changes: commit them or use lazygit (L)"
	statusInProgress = "operation in progress: finish it in lazygit (L)"
	statusNoLazygit  = "lazygit is not installed"
	statusCopied     = "copied"
	statusBusy       = "action in progress…"
)

type (
	// actionDoneMsg is the result of a repo-changing action. On success the
	// model reloads and selects selectHash; created names a branch just
	// created, whose checkout is offered next.
	actionDoneMsg struct {
		err        error
		selectHash string
		op         string // "merge", "cherry-pick", …: names a conflict
		created    string
		plan       *checkoutPlan // the checkout that ran (offered again with a stash)
		done       string        // flashed on success
		warn       string        // shown (and kept) in the status bar on success
	}
	// lazygitDoneMsg is sent when lazygit exits and the TUI is back.
	lazygitDoneMsg struct{ err error }
)

// lazygitCmd is lazygit opened on the repo at root.
func lazygitCmd(root string) *exec.Cmd {
	return exec.Command("lazygit", "-p", root)
}

// act runs do in the background and marks the model busy until its
// actionDoneMsg arrives. On success the result to select is at, or the new
// HEAD when at is "".
func (m *Model) act(op, created, at string, do func(*git.Repo) error) tea.Cmd {
	m.acting = true
	m.actSeq++
	repo := m.repo
	return func() tea.Msg {
		if err := do(repo); err != nil {
			return actionDoneMsg{err: err, op: op}
		}
		sel := at
		if sel == "" {
			sel, _ = repo.HeadHash()
		}
		return actionDoneMsg{selectHash: sel, op: op, created: created}
	}
}

// confirmAct is a confirmation answer that starts an action.
func confirmAct(op, at string, do func(*git.Repo) error) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		cmd := m.act(op, "", at, do)
		return m, cmd
	}
}

// onBranch guards do: the branch shown in the confirmation must still be the
// current one when the action runs.
func onBranch(expected string, do func(*git.Repo) error) func(*git.Repo) error {
	return func(r *git.Repo) error {
		cur, err := r.CurrentBranchRaw()
		if err != nil {
			return err
		}
		if git.SanitizeLine(cur) != cur {
			return git.ErrControlName
		}
		if cur != expected {
			if cur == "" {
				cur = "detached HEAD"
			}
			return fmt.Errorf("current branch is %s, not %s: nothing was done", cur, expected)
		}
		return do(r)
	}
}

// onActionDone applies an action's result. A follow-up question (checkout
// the new branch, open lazygit) is only asked when the list is showing, so a
// key meant for another view can never answer it; otherwise it becomes a
// status hint.
func (m Model) onActionDone(msg actionDoneMsg) (tea.Model, tea.Cmd) {
	m.acting = false
	asking := m.mode == ModeList
	var ae *git.ActionError
	var sce *git.StashConflictError
	var ske *git.StashKeptError
	switch {
	case msg.err == nil:
		m.clearStatus("action")
		m.cancelJump() // the action's result wins over a jump in progress
		m.pendingSelect = msg.selectHash
		cmd := m.reloadAll()
		if name := msg.created; name != "" {
			if !asking {
				return m, tea.Batch(cmd, m.flash("action", "branch "+name+" created"))
			}
			plan := checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutLocal, Name: name}, name: name}
			m.confirm = confirmState{title: "Branch created", text: "Check out " + name + "?",
				yes: func(m Model) (Model, tea.Cmd) { return m, m.actCheckout(plan, false) }}
			m.mode = ModeConfirm
		}
		if msg.warn != "" {
			m.setStatus("action", msg.warn)
			return m, cmd
		}
		if msg.done != "" {
			return m, tea.Batch(cmd, m.flash("action", msg.done))
		}
		return m, cmd
	case errors.As(msg.err, &sce):
		m.markStashSeen(sce.Hash)
		m.cancelJump()
		m.setStatus("action", sce.Error())
		if asking {
			m.confirm = confirmState{title: "Changes not restored",
				text: "Checked out " + sce.Target + ", but your changes conflicted when restored. They are safe in the stash — resolve in lazygit (L)?",
				yes:  Model.openLazygit}
			m.mode = ModeConfirm
		}
		return m, m.reloadAll()
	case errors.As(msg.err, &ske):
		m.markStashSeen(ske.Hash)
		m.setStatus("action", ske.Error())
		if asking {
			m.confirm = confirmState{title: "Changes not restored",
				text: "Your changes could not be restored. They are safe in the stash — restore them in lazygit (L)?",
				yes:  Model.openLazygit}
			m.mode = ModeConfirm
		}
		return m, m.reloadAll()
	case refusable(msg.err):
		// Nothing changed: say why in a dialog, with what the user can do.
		return m.refuseOrStatusWith(msg.op, msg.err, msg.plan), nil
	case errors.As(msg.err, &ae) && (ae.Conflict || ae.InProgress):
		// The repo is left as git left it (never aborted): offer lazygit.
		title, text := "Conflict", "Conflict in "+msg.op+". Open lazygit to resolve it?"
		hint := "conflict in " + msg.op + ": resolve it in lazygit (L)"
		if !ae.Conflict {
			title, text = "Operation in progress", msg.op+" is unfinished. Open lazygit to finish it?"
			hint = msg.op + " is unfinished: finish it in lazygit (L)"
		}
		if !asking {
			m.setStatus("action", hint+" · "+ae.Error())
			return m, m.reloadAll()
		}
		m.setStatus("action", "error: "+ae.Error())
		m.confirm = confirmState{title: title, text: text, yes: Model.openLazygit}
		m.mode = ModeConfirm
		return m, m.reloadAll()
	default:
		m.setStatus("action", "error: "+msg.err.Error())
	}
	return m, nil
}

// openLazygit suspends the TUI and runs lazygit on the repo; the repo is
// reloaded when it exits.
func (m Model) openLazygit() (Model, tea.Cmd) {
	if m.acting {
		return m, m.flash("action", statusBusy)
	}
	if _, err := m.sys.LookPath("lazygit"); err != nil {
		return m, m.flash("action", statusNoLazygit)
	}
	m.external = true
	return m, m.execProcess(lazygitCmd(m.repo.Root()), func(err error) tea.Msg { return lazygitDoneMsg{err: err} })
}

func (m Model) onLazygitDone(msg lazygitDoneMsg) (tea.Model, tea.Cmd) {
	m.external = false
	m.clearStatus("action") // a conflict message is stale now
	if msg.err != nil {
		m.setStatus("lazygit", "error: lazygit: "+msg.err.Error())
	} else {
		m.clearStatus("lazygit")
	}
	return m, m.reloadAll()
}

func (m Model) copyText(s string) (tea.Model, tea.Cmd) {
	if err := m.sys.Copy(s); err != nil {
		m.setStatus("action", "error: copy: "+err.Error())
		return m, nil
	}
	return m, m.flash("action", statusCopied)
}

func (m Model) openURL(url string) (tea.Model, tea.Cmd) {
	if err := m.sys.OpenURL(url); err != nil {
		m.setStatus("action", "error: open: "+err.Error())
	}
	return m, nil
}

// copySelected is the direct `y`: the branch name when the row has a branch,
// otherwise the full hash.
func (m Model) copySelected() (tea.Model, tea.Cmd) {
	r := m.Selected()
	if r == nil || r.Commit == nil {
		return m, nil
	}
	if bs := branchLabels(r.Labels); len(bs) > 0 {
		return m.copyText(bs[0].Name)
	}
	return m.copyText(r.Commit.Hash)
}

// openSelectedPR is the direct `o`: the PR of the row's first branch that has one.
func (m Model) openSelectedPR() (tea.Model, tea.Cmd) {
	if r := m.Selected(); r != nil {
		for _, l := range branchLabels(r.Labels) {
			if l.PR != nil && l.PR.URL != "" {
				return m.openURL(l.PR.URL)
			}
		}
	}
	return m, m.flash("action", "no PR")
}

// runMenuItem performs a menu choice; repo changes are confirmed first.
func (m Model) runMenuItem(it MenuItem) (tea.Model, tea.Cmd) {
	c, l, cur := m.menu.commit, m.menu.label, m.branch
	short := shortHash(c.Hash)
	switch it.Action {
	case ActCheckout:
		return m.askCheckout(c, l)
	case ActMerge:
		name := l.Name
		if err := m.precheck(); err != nil {
			return m.refuse("merge", err), nil
		}
		return m.askConfirm("Confirm", "Merge "+name+" → "+cur+"?",
			confirmAct("merge", "", onBranch(cur, func(r *git.Repo) error { return r.Merge(name) })))
	case ActCherryPick:
		if err := m.precheck(); err != nil {
			return m.refuse("cherry-pick", err), nil
		}
		return m.askConfirm("Confirm", "Cherry-pick "+short+" → "+cur+"?",
			confirmAct("cherry-pick", "", onBranch(cur, func(r *git.Repo) error { return r.CherryPick(c.Hash) })))
	case ActCreateBranch:
		return m.openInput(c.Hash)
	case ActCopyBranch:
		return m.copyText(l.Name)
	case ActCopyHash:
		return m.copyText(c.Hash)
	case ActCopyShortHash:
		return m.copyText(short)
	case ActCopyMessage:
		msg, err := m.repo.Message(c.Hash)
		if err != nil {
			m.setStatus("action", "error: "+err.Error())
			return m, nil
		}
		return m.copyText(msg)
	case ActCopyPRURL:
		return m.copyText(l.PR.URL)
	case ActOpenPR:
		return m.openURL(l.PR.URL)
	}
	return m, nil
}
