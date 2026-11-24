package ui

import (
	"errors"
	"strings"

	"github.com/Stratovarius93/gitree/internal/git"
	tea "github.com/charmbracelet/bubbletea"
)

// statusNoCheckout is the hint for `c` on a row with nothing to check out.
const statusNoCheckout = "select a branch or commit to check out"

// checkoutPlan is a checkout the user is asked about: target is what git
// switches to, name what it is called on screen (the local branch, or the
// short hash), at the commit to select afterwards ("" the new HEAD).
type checkoutPlan struct {
	target git.CheckoutTarget
	name   string
	at     string
}

// checkoutSelected is the direct `c`: check out the selected row's branch
// (asking which when it has several) or, without one, its commit detached.
func (m Model) checkoutSelected() (tea.Model, tea.Cmd) {
	r := m.Selected()
	if r == nil || r.Commit == nil {
		return m, m.flash("action", statusNoCheckout)
	}
	if m.acting {
		return m, m.flash("action", statusBusy)
	}
	switch bs := branchLabels(r.Labels); len(bs) {
	case 0:
		return m.askCheckout(*r.Commit, nil)
	case 1:
		return m.askCheckout(*r.Commit, &bs[0])
	default:
		m.menu = menuState{commit: *r.Commit, title: "Check out which branch?", choices: bs, checkout: true}
		m.mode = ModeMenu
		return m, nil
	}
}

// askCheckout asks to check out branch l of commit c (c itself, detached,
// when l is nil). It is refused at once, in the dialog, when an operation is
// in progress or tracked files have changes (offering to stash them).
func (m Model) askCheckout(c git.Commit, l *Label) (tea.Model, tea.Cmd) {
	short := shortHash(c.Hash)
	var plan checkoutPlan
	var text string
	switch {
	case l == nil:
		plan = checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutDetached, Name: c.Hash}, name: short, at: c.Hash}
		text = "HEAD will be detached, with no branch: commits you make will not be on " +
			"any branch until you create one. Check out " + short + "?"
	case l.Local:
		plan = checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutLocal, Name: l.Name}, name: l.Name}
		text = "Check out " + l.Name + "?"
	default:
		local, exists, err := m.repo.LocalBranchFor(l.Name)
		if err != nil {
			m.setStatus("action", "error: "+err.Error())
			return m, nil
		}
		switch {
		case !exists:
			plan = checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutTrack, Name: l.Name}, name: local}
			text = "Check out " + l.Name + " as a new local branch " + local + " tracking it?"
		case m.track[local].Upstream == l.Name:
			plan = checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutLocal, Name: local}, name: local}
			text = "Check out " + local + " (tracks " + l.Name + ")?"
		default:
			plan = checkoutPlan{target: git.CheckoutTarget{Kind: git.CheckoutLocal, Name: local}, name: local}
			text = "Check out " + local + " (local branch named after " + l.Name + ")?"
		}
	}
	if plan.target.Kind == git.CheckoutLocal && plan.name == m.branch && m.branch != "" {
		return m, m.flash("action", "already on "+m.branch)
	}
	if err := m.precheck(); err != nil {
		return m.refuseWith("checkout", err, &plan), nil
	}
	return m.askConfirm("Confirm", text, func(m Model) (Model, tea.Cmd) {
		return m, m.actCheckout(plan, false)
	})
}

// precheck refuses, before asking, what a checkout, merge or cherry-pick
// would refuse: an operation in progress or changed tracked files. The git
// layer checks again when the action runs.
func (m Model) precheck() error {
	if busy, err := m.repo.InProgress(); err == nil && busy {
		return git.ErrInProgress
	}
	if files, err := m.repo.TrackedChanges(); err == nil && len(files) > 0 {
		return git.ErrDirty
	}
	return nil
}

// askCheckoutStash is the refusal dialog's `s` for a checkout: a second
// question, accepted with y only, before stashing, checking out and
// restoring the changes.
func (m Model) askCheckoutStash(plan checkoutPlan) (Model, tea.Cmd) {
	if busy, err := m.repo.InProgress(); err == nil && busy {
		return m.refuseWith("checkout", git.ErrInProgress, &plan), nil
	}
	files, err := m.repo.TrackedChanges()
	if err != nil {
		m.setStatus("action", "error: "+err.Error())
		return m, nil
	}
	text := "Stash " + plural(len(files), "changed file") + ", check out " + plan.name + ", then restore your changes?"
	stash := true
	if len(files) == 0 { // committed or stashed meanwhile
		text, stash = "Check out "+plan.name+"?", false
	}
	m.confirm = confirmState{title: "Check out", text: text, yes: func(m Model) (Model, tea.Cmd) {
		return m, m.actCheckout(plan, stash)
	}}
	m.mode = ModeConfirm
	return m, nil
}

// actCheckout runs the confirmed checkout in the background; with stash the
// tracked changes are stashed and restored around it.
func (m *Model) actCheckout(plan checkoutPlan, stash bool) tea.Cmd {
	m.acting = true
	m.actSeq++
	repo := m.repo
	return func() tea.Msg {
		msg := actionDoneMsg{op: "checkout", plan: &plan}
		var err error
		var warns []string
		if stash {
			var res git.CheckoutResult
			res, err = repo.CheckoutWithStash(plan.target)
			msg.done = "checked out " + plan.name + ", changes restored"
			if res.Unstaged {
				msg.done += " (staged changes came back unstaged)"
			}
			if res.Kept {
				warns = append(warns, "the stash changed meanwhile, so gitree's entry was kept in the stash")
			}
			if res.Hook != "" {
				warns = append(warns, "post-checkout hook failed: "+res.Hook)
			}
		} else {
			err = repo.Checkout(plan.target)
			var he *git.HookFailedError
			if errors.As(err, &he) {
				msg.done, err = "checked out "+plan.name, nil
				warns = append(warns, he.Error())
			}
		}
		if len(warns) > 0 {
			msg.warn = msg.done + "; " + strings.Join(warns, "; ")
		}
		if err != nil {
			return actionDoneMsg{op: "checkout", plan: &plan, err: err}
		}
		msg.selectHash = plan.at
		if msg.selectHash == "" {
			msg.selectHash, _ = repo.HeadHash()
		}
		return msg
	}
}
