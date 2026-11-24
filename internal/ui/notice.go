package ui

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// noticeState is a refusal dialog (ModeNotice): why an action can't run
// (text), what is behind it (details, e.g. the changed tracked files) and
// what the user can do instead (opts). esc, enter and n close it.
type noticeState struct {
	title   string
	text    string
	details []string
	files   []git.FileChange
	opts    []noticeOption
}

// noticeOption is one recommendation: key runs it; advice has no key.
type noticeOption struct {
	key   string
	label string
	run   func(Model) (Model, tea.Cmd)
}

// noticeMaxFiles is how many changed files a notice lists before "+N more".
const noticeMaxFiles = 8

// errFetching refuses a pull or push while a fetch updates the same refs.
var errFetching = errors.New(statusFetching)

// opTitles name the dialog of each refused operation.
var opTitles = map[string]string{
	"pull": "Can't pull", "push": "Can't push", "checkout": "Can't check out",
	"merge": "Can't merge", "cherry-pick": "Can't cherry-pick", "branch": "Can't create branch",
}

// refusedBeforeGit reports whether err is a refusal made before git ran
// (nothing changed), which gets a dialog rather than a status line.
func refusedBeforeGit(err error) bool {
	for _, e := range []error{git.ErrDirty, git.ErrInProgress, git.ErrDiverged, git.ErrDetached,
		git.ErrNoUpstream, git.ErrNothingToPush, git.ErrControlName, errFetching} {
		if errors.Is(err, e) {
			return true
		}
	}
	return errors.As(err, new(*git.GoneUpstreamError))
}

// refusable reports whether err gets the refusal dialog: refused before git
// ran, or a refusal git made (files it would overwrite) that changed nothing.
func refusable(err error) bool {
	var ae *git.ActionError
	return refusedBeforeGit(err) || errors.As(err, new(*git.CheckoutRestoredError)) ||
		(errors.As(err, &ae) && len(ae.Blocked) > 0 && !ae.Conflict)
}

// sentence capitalizes s and ends it with a period.
func sentence(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[n:]
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ")") {
		s += "."
	}
	return s
}

// asModel adapts a list action to a dialog option.
func asModel(f func(Model) (tea.Model, tea.Cmd)) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		nm, cmd := f(m)
		return nm.(Model), cmd
	}
}

// detachedOpts are the recommendations without a current branch: find one
// (b), check out the selected row (c).
func detachedOpts() []noticeOption {
	return []noticeOption{
		{key: "b", label: "Find a branch", run: asModel(Model.openPicker)},
		{key: "c", label: "Check out a branch", run: asModel(Model.checkoutSelected)},
	}
}

func optFetch() noticeOption {
	return noticeOption{key: "f", label: "Fetch to check for remote changes", run: func(m Model) (Model, tea.Cmd) {
		return m, m.startFetch()
	}}
}

func optLazygit(label string) noticeOption {
	return noticeOption{key: "L", label: label, run: Model.openLazygit}
}

func optPush(label string) noticeOption {
	return noticeOption{key: "P", label: label, run: asModel(Model.askPush)}
}

// refuse opens the refusal dialog for op.
func (m Model) refuse(op string, err error) Model { return m.refuseWith(op, err, nil) }

// refuseWith opens the refusal dialog for op: the reason, what is behind it
// and the recommendations that apply. plan is the refused checkout (nil for
// other operations), which can be retried with a stash.
func (m Model) refuseWith(op string, err error, plan *checkoutPlan) Model {
	title := opTitles[op]
	if title == "" {
		title = "Can't " + op
	}
	n := noticeState{title: title}
	var ae *git.ActionError
	var gone *git.GoneUpstreamError
	var restored *git.CheckoutRestoredError
	errors.As(err, &restored)
	blocked := errors.As(err, &ae) && len(ae.Blocked) > 0
	switch {
	case blocked && ae.BlockedUntracked:
		n.text = "Untracked files would be overwritten by the " + op + "."
		for _, f := range ae.Blocked {
			n.files = append(n.files, git.FileChange{Status: '?', Path: f})
		}
		n.opts = []noticeOption{optLazygit("Open lazygit to move or remove them")}
	case errors.Is(err, git.ErrDirty), blocked:
		n.text = "You have uncommitted changes in tracked files, and gitree only runs " + op + " on a clean working tree."
		if blocked {
			for _, f := range ae.Blocked {
				n.files = append(n.files, git.FileChange{Status: 'M', Path: f})
			}
		} else {
			n.files, _ = m.repo.TrackedChanges()
		}
		switch {
		case op == "checkout" && plan != nil:
			p := *plan
			n.opts = append(n.opts, noticeOption{key: "s", label: "Stash changes, check out " + p.name + ", then restore them",
				run: func(m Model) (Model, tea.Cmd) { return m.askCheckoutStash(p) }})
		case op == "pull":
			n.opts = append(n.opts, noticeOption{key: "s", label: "Stash changes, pull, and restore them", run: Model.askPullAutostash})
		}
		n.opts = append(n.opts, optLazygit("Open lazygit to commit or stash them"))
	case errors.Is(err, git.ErrInProgress):
		n.text = "A merge, cherry-pick, revert or rebase is in progress (or a pull left its stash behind)."
		n.opts = []noticeOption{optLazygit("Finish or abort it in lazygit")}
	case errors.Is(err, git.ErrDiverged):
		n.text = "Branches diverged: " + m.branch + " and its upstream both have new commits, so a fast-forward pull is impossible."
		n.opts = []noticeOption{optLazygit("Rebase or merge in lazygit")}
	case errors.Is(err, git.ErrNoUpstream):
		n.text = sentence(err.Error())
		n.opts = []noticeOption{optPush("Push to create origin/" + m.branch)}
	case errors.As(err, &gone):
		n.text = sentence(err.Error())
		n.details = []string{gone.Upstream + " was deleted on the remote."}
		n.opts = []noticeOption{optPush("Push to recreate " + gone.Upstream)}
	case errors.Is(err, git.ErrNothingToPush):
		n.text = "Nothing to push: " + m.branch + " has no commits its upstream lacks."
		if up := m.track[m.branch].Upstream; up != "" {
			n.text = "Nothing to push: " + up + " already has every commit of " + m.branch + "."
		}
		n.opts = []noticeOption{optFetch()}
	case errors.Is(err, errFetching):
		n.text = "A fetch is in progress and updates the same remote branches."
		n.opts = []noticeOption{{label: "Wait a moment and try again"}}
	case errors.Is(err, git.ErrDetached):
		n.text = "HEAD is detached: there is no current branch to " + op + "."
		if op == "merge" || op == "cherry-pick" {
			n.text = "HEAD is detached: there is no current branch to " + op + " onto."
		}
		n.opts = detachedOpts()
	case errors.Is(err, git.ErrControlName):
		n.text = sentence(err.Error())
		n.details = []string{"Names are cleaned up for display, so the branch shown may not be the real one."}
		n.opts = []noticeOption{{label: "Use git directly"}}
	default:
		cause := err
		if restored != nil {
			cause = restored.Err
		}
		n.text = sentence(cause.Error())
		n.opts = []noticeOption{optLazygit("Open lazygit to look into it")}
	}
	if restored != nil {
		d := "Nothing changed: your stashed changes were restored."
		if restored.Kept {
			d = "Nothing changed: your stashed changes were restored (the stash changed meanwhile, so gitree's entry was kept in the stash)."
		}
		n.details = append(n.details, d)
	}
	m.notice = n
	m.clearStatus("action")
	m.mode = ModeNotice
	return m
}

// refuseOrStatus shows the refusal dialog when the list is showing (so a key
// meant for another view never lands in it), otherwise a status line.
func (m Model) refuseOrStatus(op string, err error) Model { return m.refuseOrStatusWith(op, err, nil) }

func (m Model) refuseOrStatusWith(op string, err error, plan *checkoutPlan) Model {
	if m.mode == ModeList {
		return m.refuseWith(op, err, plan)
	}
	switch {
	case errors.Is(err, git.ErrDirty):
		m.setStatus("action", statusDirty)
	case errors.Is(err, git.ErrInProgress):
		m.setStatus("action", statusInProgress)
	default:
		m.setStatus("action", err.Error())
	}
	return m
}

func (m Model) updateNotice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case msg.String() == "n", key.Matches(msg, keys.Esc), key.Matches(msg, keys.Enter):
		m.mode = ModeList
		return m, nil
	}
	for _, o := range m.notice.opts {
		if o.key == "" || o.run == nil || msg.String() != o.key {
			continue
		}
		m.mode = ModeList
		if m.acting {
			return m, m.flash("action", statusBusy)
		}
		return o.run(m)
	}
	return m, nil
}

// noticeContent is the dialog's lines for at most n inner lines.
func (m Model) noticeContent(wrapW, n int) []string {
	gray := floatMuted
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.Yellow))
	statusStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.Yellow))
	var text []string
	for _, l := range strings.Split(ansi.Wordwrap(m.notice.text, wrapW, ""), "\n") {
		text = append(text, " "+l+" ")
	}
	files := m.notice.files
	footer := gray.Render(" esc close ")
	var details []string
	for _, d := range m.notice.details {
		for _, l := range strings.Split(ansi.Wordwrap(d, wrapW, ""), "\n") {
			details = append(details, gray.Render(" "+l+" "))
		}
	}
	build := func(limit int, blanks bool) []string {
		out := append([]string(nil), text...)
		out = append(out, details...)
		if len(files) > 0 {
			if blanks {
				out = append(out, "")
			}
			shown := min(len(files), limit)
			if shown < len(files) && shown > 0 && limit < noticeMaxFiles {
				shown-- // short terminal: room for "+N more"
			}
			for _, f := range files[:shown] {
				path := ansi.Truncate(git.SanitizeLine(f.Path), max(wrapW-3, 1), "…")
				out = append(out, " "+statusStyle.Render(string(f.Status))+" "+path+" ")
			}
			if more := len(files) - shown; more > 0 {
				out = append(out, gray.Render(" +"+strconv.Itoa(more)+" more "))
			}
		}
		if len(m.notice.opts) > 0 {
			if blanks {
				out = append(out, "", floatBold.Render(" What you can do"))
			}
			for _, o := range m.notice.opts {
				k := gray.Render("·")
				if o.key != "" {
					k = keyStyle.Render(o.key)
				}
				out = append(out, " "+k+"  "+o.label+" ")
			}
		}
		if blanks {
			out = append(out, "")
		}
		return append(out, footer)
	}
	content := build(noticeMaxFiles, true)
	for limit := noticeMaxFiles; len(content) > n && limit > 0; limit-- {
		content = build(limit, false)
	}
	if len(content) > n {
		content = keepFooter(content, n)
	}
	return content
}
