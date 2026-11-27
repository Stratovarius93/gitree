package ui

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// PageSize is how many commits one log load fetches.
const PageSize = 300

// pageAhead: the next page is requested once the cursor is this close to the end.
const pageAhead = 50

// headerLines and statusLines frame the commit list.
const (
	headerLines = 2
	statusLines = 1
)

// Mode is what the model is currently showing and routing keys to.
type Mode int

const (
	ModeList Mode = iota
	ModeDetail
	ModePreviewFiles
	ModePreviewDiff
	ModeMenu
	ModeConfirm
	ModeInput
	ModeSearch
	ModeHelp
	ModeBranchPicker
	ModeNotice
)

// Messages produced by the async loaders. Each carries what it needs to be
// applied (or recognised as stale) without looking at the model that issued it.
type (
	// logLoadedMsg is one page of the log. gen identifies the full reload it
	// belongs to; pages from an older generation are dropped.
	logLoadedMsg struct {
		gen     int
		scope   git.Scope
		limit   int // commits requested; 0 means PageSize
		skip    int
		commits []git.Commit
		append  bool
		err     error
		// fp is the refs fingerprint taken just before a full (non-append)
		// load read the log ("" for pages or when it could not be taken).
		fp string
	}
	wipLoadedMsg struct {
		poll  bool // from a poll or focus check, not a load or reload
		files []git.FileChange
		err   error
	}
	trackLoadedMsg struct {
		track map[string]git.Track
		err   error
	}
	branchLoadedMsg struct {
		branch string
		err    error
	}
	// statsLoadedMsg carries line totals for commits; see loadStats.
	statsLoadedMsg struct {
		stats map[string]git.Stat
		err   error
	}
)

// Model is the Bubble Tea model for the whole app.
type Model struct {
	repo     *git.Repo
	sys      System
	now      func() time.Time
	repoName string

	mode  Mode
	scope git.Scope

	commits    []git.Commit
	wip        []git.FileChange
	track      map[string]git.Track
	prs        map[string]github.PR
	branch     string
	rows       []Row
	labelWidth int // natural width of the widest label; set in rebuild
	// stats caches commit line totals by hash. A commit's totals never
	// change, so the cache survives reloads.
	stats map[string]git.Stat

	// layout is derived from commits in relayout only; cursor moves
	// re-render just the visible window of it (see View).
	layout graphLayout

	cursor, offset int
	userMoved      bool // until the user moves, the cursor is pinned to row 0
	width, height  int

	gen     int  // generation of the current full log load
	loading bool // a log load (full or page) is in flight
	loaded  bool // the first page of the current generation has arrived
	more    bool // the last page was full, so there may be more commits

	detail  detailState  // right-hand panel while mode is ModeDetail
	preview previewState // floating window while mode is ModePreview*
	search  searchState  // typed query and its matches (ModeSearch and after)
	helpOff int          // first visible line of the help overlay
	menu    menuState    // action menu (ModeMenu)
	confirm confirmState // yes/no question (ModeConfirm)
	notice  noticeState  // refusal dialog (ModeNotice)
	input   inputState   // branch-name prompt (ModeInput)
	picker  pickerState  // branch picker (ModeBranchPicker)
	jump    jumpState    // jump to a picked ref still loading pages

	acting bool // a repo-changing action is running in the background
	// actSeq counts actions started: a stash listing made before or during
	// one is stale (an action's own temporary stash entry must not be
	// announced as an interrupted checkout).
	actSeq int
	// stashSeen holds the gitree stash entries (by commit) already announced
	// or reported this session.
	stashSeen map[string]bool

	// pendingSelect is selected once the reload after an action brings it in.
	pendingSelect string
	// execProcess hands the terminal to another program (tea.ExecProcess;
	// tests record the command instead).
	execProcess func(*exec.Cmd, tea.ExecCallback) tea.Cmd

	status    string    // last message shown in the status bar
	statusSrc string    // what produced status; its next success clears it
	statusSeq int       // bumped by every status change; expiry timers check it
	lastFetch time.Time // when the last fetch finished (zero: none yet)
	fetchErr  string    // why the last fetch failed ("" when it did not)
	prNote    string    // why PRs are unavailable ("" when they are or are not applicable)

	fetchPRs  func(root string) (map[string]github.PR, error)
	fetching  bool // a git fetch is in flight
	autoFetch bool // fetch every AutoFetchInterval (see EnableAutoFetch)
	external  bool // lazygit has the terminal: auto-fetch ticks are skipped

	spin      spinner.Model // busy indicator in the status bar and panels
	spinOn    bool          // animate it (see EnableSpinner)
	animating bool          // a spinner tick chain is running

	version    string // shown at the right of the status bar ("" = dev)
	statusOK   bool   // status is a confirmation (set by flash)
	statusWarn bool   // status is a warning (set by flashWarn)

	polling     bool   // follow outside changes (see EnablePolling)
	fingerprint string // refs fingerprint of the last full log load
}

// New returns a model for repo; Init starts loading.
func New(repo *git.Repo, sys System) Model {
	return Model{
		repo:     repo,
		sys:      sys,
		now:      time.Now,
		repoName: filepath.Base(repo.Root()),
		mode:     ModeList,
		scope:    git.ScopeDefault,
		width:    80,
		height:   24,
		gen:      1,
		loading:  true,
		fetchPRs: github.FetchPRs,
		spin:     newSpinner(),

		execProcess: tea.ExecProcess,
		stashSeen:   map[string]bool{},
		stats:       map[string]git.Stat{},
	}
}

// Init loads the log, working tree, tracking info and current branch in parallel.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadLog(0, false), m.loadWIP(), m.loadTrack(), m.loadBranch(), m.loadPRs(), m.loadStashes()}
	if m.autoFetch {
		cmds = append(cmds, m.autoFetchTick())
	}
	cmds = append(cmds, m.refsPollTick(), m.wipPollTick())
	return tea.Batch(cmds...)
}

func (m Model) loadLog(skip int, appendPage bool) tea.Cmd {
	return m.loadLogN(skip, PageSize, appendPage)
}

func (m Model) loadLogN(skip, limit int, appendPage bool) tea.Cmd {
	repo, scope, gen := m.repo, m.scope, m.gen
	return func() tea.Msg {
		// Taken before the log is read: a change racing the load is seen
		// by the next poll (at worst one extra reload), never missed.
		var fp string
		if !appendPage {
			fp, _ = repo.Fingerprint()
		}
		commits, err := repo.Log(scope, skip, limit)
		return logLoadedMsg{gen: gen, scope: scope, limit: limit, skip: skip, commits: commits, append: appendPage, err: err, fp: fp}
	}
}

// missingStats lists the hashes of commits whose stats are not cached.
func (m Model) missingStats(commits []git.Commit) []string {
	var out []string
	for _, c := range commits {
		if _, ok := m.stats[c.Hash]; !ok {
			out = append(out, c.Hash)
		}
	}
	return out
}

// loadStats fetches the line totals of commits not cached yet (nil when all are).
func (m Model) loadStats(commits []git.Commit) tea.Cmd {
	hashes := m.missingStats(commits)
	if len(hashes) == 0 {
		return nil
	}
	repo := m.repo
	return func() tea.Msg {
		st, err := repo.Stats(hashes)
		return statsLoadedMsg{stats: st, err: err}
	}
}

func (m Model) loadWIP() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		files, err := repo.WorkingFiles()
		if err != nil && unborn(repo) {
			// WorkingFiles fails without a HEAD commit; an empty repo simply has no WIP row.
			return wipLoadedMsg{}
		}
		return wipLoadedMsg{files: files, err: err}
	}
}

// unborn reports whether repo has no commits to log at all.
func unborn(repo *git.Repo) bool {
	commits, err := repo.Log(git.ScopeDefault, 0, 1)
	return err == nil && len(commits) == 0
}

func (m Model) loadTrack() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		t, err := repo.Tracking()
		return trackLoadedMsg{track: t, err: err}
	}
}

func (m Model) loadBranch() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		b, err := repo.CurrentBranch()
		return branchLoadedMsg{branch: b, err: err}
	}
}

// reloadLog starts a new generation: in-flight pages of the old one are ignored.
func (m *Model) reloadLog() tea.Cmd {
	m.gen++
	m.loading, m.loaded, m.more = true, false, false
	return m.loadLog(0, false)
}

// maybeLoadMore requests the next page when the cursor nears the end.
func (m *Model) maybeLoadMore() tea.Cmd {
	if !m.more || m.loading || len(m.rows)-1-m.cursor >= pageAhead {
		return nil
	}
	m.loading = true
	return m.loadLog(len(m.commits), true)
}

// Update routes a message and starts the spinner if the model became busy.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm := next.(Model)
	if spin := nm.startSpin(); spin != nil {
		return nm, tea.Batch(cmd, spin)
	}
	return nm, cmd
}

// update routes messages: load results are handled in every mode, keys by mode.
func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		return m.onSpinTick(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
		m.scrollToCursor()
		if m.mode == ModeDetail {
			return m, tea.Batch(m.maybeLoadMore(), m.resizeDetail())
		}
		if m.mode == ModePreviewDiff {
			return m, tea.Batch(m.maybeLoadMore(), m.resizePreview())
		}
		return m, m.maybeLoadMore()
	case detailLoadedMsg:
		return m.onDetail(msg)
	case detailDebounceMsg:
		return m.onDetailDebounce(msg)
	case statusExpireMsg:
		if msg.seq == m.statusSeq {
			m.status, m.statusSrc = "", ""
		}
		return m, nil
	case previewFilesMsg:
		return m.onPreviewFiles(msg)
	case previewDiffMsg:
		return m.onPreviewDiff(msg)
	case logLoadedMsg:
		nm, cmd := m.onLog(msg)
		// Stats for the page load in the background; the graph never waits.
		return nm, tea.Batch(cmd, nm.(Model).loadStats(msg.commits))
	case statsLoadedMsg:
		if msg.err != nil {
			return m, nil // the column keeps its loading look; never an error
		}
		if m.stats == nil {
			m.stats = map[string]git.Stat{}
		}
		for h, st := range msg.stats {
			m.stats[h] = st
		}
		return m, nil
	case refsLoadedMsg:
		return m.onRefs(msg)
	case wipLoadedMsg:
		if msg.err != nil {
			m.setStatus("wip", "error: "+msg.err.Error())
			return m, nil
		}
		m.clearStatus("wip")
		if slices.Equal(msg.files, m.wip) {
			return m, nil // a poll found nothing new: no rebuild, no flicker
		}
		if msg.poll && m.loaded {
			// The WIP row appearing must not pull the cursor to row 0.
			m.userMoved = true
		}
		m.wip = msg.files
		m.rebuild()
		if msg.poll && m.selectedKey() == wipKey {
			// No log reload follows a poll: refresh an open WIP panel now.
			return m, m.refreshPanels()
		}
		return m, m.syncPanels()
	case trackLoadedMsg:
		if msg.err != nil {
			m.setStatus("track", "error: "+msg.err.Error())
			return m, nil
		}
		m.clearStatus("track")
		m.track = msg.track
		m.rebuild()
		return m, m.syncPanels()
	case branchLoadedMsg:
		if msg.err != nil {
			m.setStatus("branch", "error: "+msg.err.Error())
			return m, nil
		}
		m.clearStatus("branch")
		m.branch = msg.branch
		return m, nil
	case fetchDoneMsg:
		return m.onFetchDone(msg)
	case prsLoadedMsg:
		return m.onPRs(msg)
	case autoFetchTickMsg:
		return m.onAutoFetchTick()
	case refsPollTickMsg:
		return m.onRefsPollTick()
	case wipPollTickMsg:
		return m.onWIPPollTick()
	case fingerprintMsg:
		return m.onFingerprint(msg)
	case tea.FocusMsg:
		return m.onFocus()
	case netActionDoneMsg:
		return m.onNetActionDone(msg)
	case actionDoneMsg:
		return m.onActionDone(msg)
	case lazygitDoneMsg:
		return m.onLazygitDone(msg)
	case stashesLoadedMsg:
		return m.onStashes(msg)
	case tea.KeyMsg:
		if m.acting && m.quitKey(msg) {
			if m.mode == ModeConfirm && m.confirm.quit {
				if msg.Type == tea.KeyCtrlC {
					return m, tea.Quit // a second ctrl+c forces
				}
			} else {
				return m.askQuit()
			}
		}
		switch m.mode {
		case ModeDetail:
			return m.updateDetail(msg)
		case ModePreviewFiles, ModePreviewDiff:
			return m.updatePreview(msg)
		case ModeSearch:
			return m.updateSearch(msg)
		case ModeHelp:
			return m.updateHelp(msg)
		case ModeMenu:
			return m.updateMenu(msg)
		case ModeConfirm:
			return m.updateConfirm(msg)
		case ModeNotice:
			return m.updateNotice(msg)
		case ModeInput:
			return m.updateInput(msg)
		case ModeBranchPicker:
			return m.updatePicker(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) onLog(msg logLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.gen || (msg.append && msg.skip != len(m.commits)) {
		return m, nil // stale: superseded by a newer reload
	}
	m.loading = false
	if msg.err != nil {
		m.cancelJump()
		m.setStatus("log", "error: "+msg.err.Error())
		return m, nil
	}
	m.clearStatus("log")
	if msg.append {
		m.commits = append(m.commits, msg.commits...)
	} else {
		m.commits = msg.commits
		if msg.fp != "" {
			m.fingerprint = msg.fp
		}
	}
	m.loaded = true
	limit := msg.limit
	if limit == 0 {
		limit = PageSize
	}
	m.more = len(msg.commits) == limit
	m.relayout()
	if m.jump.hash != "" {
		cmd, done := m.continueJump()
		if msg.append || done {
			return m, cmd // done: continueJump refreshed the panels
		}
		return m, tea.Batch(cmd, m.refreshPanels())
	}
	if !msg.append {
		m.pendingSelect = "" // the reload is in: selected, or gone
	}
	if msg.append {
		return m, m.maybeLoadMore()
	}
	return m, tea.Batch(m.maybeLoadMore(), m.refreshPanels())
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Down):
		return m.moveTo(m.cursor + 1)
	case key.Matches(msg, keys.Up):
		return m.moveTo(m.cursor - 1)
	case key.Matches(msg, keys.Top):
		return m.moveTo(0)
	case key.Matches(msg, keys.Bottom):
		return m.moveTo(len(m.rows) - 1)
	case key.Matches(msg, keys.HalfDown):
		return m.moveTo(m.cursor + max(m.listHeight()/2, 1))
	case key.Matches(msg, keys.HalfUp):
		return m.moveTo(m.cursor - max(m.listHeight()/2, 1))
	case key.Matches(msg, keys.Enter):
		return m.openDetail()
	case key.Matches(msg, keys.Space):
		return m.openPreview()
	case key.Matches(msg, keys.Search):
		return m.startSearch()
	case key.Matches(msg, keys.Branch):
		return m.openPicker()
	case key.Matches(msg, keys.Next):
		return m.jumpSearch(1)
	case key.Matches(msg, keys.Prev):
		return m.jumpSearch(-1)
	case key.Matches(msg, keys.Help):
		m.mode, m.helpOff = ModeHelp, 0
		return m, nil
	case key.Matches(msg, keys.Esc) && m.jump.hash != "":
		m.cancelJump()
		return m, nil
	case key.Matches(msg, keys.Esc) && m.search.query != "":
		m.search = searchState{}
		return m, nil
	case key.Matches(msg, keys.Fetch):
		if m.acting {
			return m, m.flash("action", statusBusy)
		}
		return m, m.startFetch()
	case key.Matches(msg, keys.Pull):
		return m.askPull()
	case key.Matches(msg, keys.Push):
		return m.askPush()
	case key.Matches(msg, keys.Reload):
		return m, m.reloadAll()
	case key.Matches(msg, keys.Menu):
		return m.openMenu()
	case key.Matches(msg, keys.Checkout):
		return m.checkoutSelected()
	case key.Matches(msg, keys.Copy):
		return m.copySelected()
	case key.Matches(msg, keys.OpenPR):
		return m.openSelectedPR()
	case key.Matches(msg, keys.Lazygit):
		return m.openLazygit()
	case key.Matches(msg, keys.Scope):
		m.cancelJump()
		if m.scope == git.ScopeAll {
			m.scope = git.ScopeDefault
		} else {
			m.scope = git.ScopeAll
		}
		return m, m.reloadLog()
	}
	return m, nil
}

// moveTo puts the cursor on row i (clamped) and requests more commits if
// needed. It does no per-commit work: View re-renders the visible graph with
// the new selection, so a move costs the same with 300 or 30 000 commits.
func (m Model) moveTo(i int) (tea.Model, tea.Cmd) {
	m.userMoved = true
	m.cancelJump()
	i = max(min(i, len(m.rows)-1), 0)
	if i == m.cursor {
		return m, nil
	}
	m.cursor = i
	m.scrollToCursor()
	return m, m.maybeLoadMore()
}

// selectedHash is the hash of the selected commit row, "" for WIP or nothing.
func (m Model) selectedHash() string {
	if r := m.Selected(); r != nil && r.Commit != nil {
		return r.Commit.Hash
	}
	return ""
}

// relayout recomputes the graph layout after the commits changed, then the rows.
func (m *Model) relayout() {
	m.layout = newGraphLayout(m.commits)
	m.rebuild()
}

// rebuild recomputes the rows (labels and the WIP row) after a data change,
// reusing the graph layout: it is linear in the commits with no graph work,
// so WIP, tracking and PR results stay cheap. The cursor stays on row 0 until the user has moved (so the
// first selection does not depend on whether log or WIP arrived first), and
// afterwards stays on the same commit, or on WIP if that was selected. After
// an action, pendingSelect (the result) wins once it is loaded.
func (m *Model) rebuild() {
	hash := m.selectedHash()
	onWIP := m.Selected() != nil && m.Selected().Kind == RowWIP
	if m.pendingSelect != "" {
		hash, onWIP = m.pendingSelect, false
	}
	m.rows = layoutRows(m.commits, m.layout, m.wip, m.track, m.prs)
	m.labelWidth = 0
	for i := range m.rows {
		m.labelWidth = max(m.labelWidth, NaturalLabelWidth(m.rows[i].Labels))
	}
	m.refreshSearch()
	switch {
	case !m.userMoved:
		m.cursor = 0
	case onWIP && len(m.rows) > 0 && m.rows[0].Kind == RowWIP:
		m.cursor = 0
	case hash != "":
		for i, r := range m.rows {
			if r.Commit != nil && r.Commit.Hash == hash {
				m.cursor = i
				break
			}
		}
	}
	m.cursor = max(min(m.cursor, len(m.rows)-1), 0)
	m.scrollToCursor()
}

// visibleGraphWidth is the widest graph line among the rows on screen: the
// graph column is sized by what is visible, not by the widest row loaded.
func (m Model) visibleGraphWidth() int {
	if len(m.rows) == 0 {
		return 0
	}
	from, to := m.offset, min(m.offset+m.listHeight(), len(m.rows))
	n, wip := 0, 0
	if m.rows[0].Kind == RowWIP {
		wip = 1
		if from == 0 {
			n = wipGraphWidth(m.rows[0].Lane)
		}
	}
	return max(n, m.layout.maxWidth(from-wip, to-wip))
}

// visibleAuthorWidth is the width of the widest author name on screen.
func (m Model) visibleAuthorWidth() int {
	n := 0
	for i := m.offset; i < min(m.offset+m.listHeight(), len(m.rows)); i++ {
		if c := m.rows[i].Commit; c != nil {
			n = max(n, ansi.StringWidth(authorName(c.AuthorName)))
		}
	}
	return n
}

// listHeight is the number of rows the list shows.
func (m Model) listHeight() int {
	return max(m.height-headerLines-statusLines, 1)
}

// scrollToCursor adjusts offset so the cursor is visible.
func (m *Model) scrollToCursor() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(min(m.offset, len(m.rows)-h), 0)
}

// Selected is the row under the cursor, or nil when there are no rows.
func (m Model) Selected() *Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m Model) scopeLabel() string {
	if m.scope == git.ScopeAll {
		return "all branches"
	}
	return "local branches"
}

var noNewlines = strings.NewReplacer("\r", " ", "\n", " ")

// Status is the plain text of the status bar.
func (m Model) Status() string {
	branch := m.branch
	if branch == "" {
		branch = "detached HEAD"
	}
	if m.mode == ModeSearch {
		return " " + m.search.input.View()
	}
	parts := []string{m.repoName, branch, m.scopeLabel()}
	switch {
	case m.status != "" && m.acting && m.statusSrc == "action" && strings.HasSuffix(m.status, "…"):
		parts = append(parts, m.spinning(m.status)) // "pulling…" → "pulling ⠋"
	case m.status != "":
		parts = append(parts, m.status)
	}
	switch {
	case m.search.query != "":
		parts = append(parts, m.searchStatus())
	case m.loading:
		parts = append(parts, m.spinning("loading…"))
	case m.loaded && len(m.commits) == 0:
		parts = append(parts, "no commits")
	}
	if m.prNote != "" {
		parts = append(parts, m.prNote)
	}
	switch {
	case m.fetching:
		parts = append(parts, m.spinning("fetching…"))
	case m.fetchErr != "":
		parts = append(parts, m.fetchErr)
	}
	if !m.lastFetch.IsZero() {
		parts = append(parts, "last fetch "+m.lastFetch.Format("15:04"))
	}
	return " " + noNewlines.Replace(strings.Join(parts, "  │  "))
}

// View renders the header, the visible rows and the status bar in exactly
// width × height cells (fewer lines only when the terminal is tiny).
func (m Model) View() string {
	lw := m.listWidth()
	w := ComputeWidths(lw, m.visibleGraphWidth(), m.labelWidth, m.visibleAuthorWidth())
	lines := strings.Split(RenderHeader(w), "\n")
	h := m.listHeight()
	switch {
	case len(m.rows) == 0:
		text := m.spinning("loading…")
		if m.loaded {
			text = "no commits"
		}
		gray := lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray))
		lines = append(lines, gray.Render(ansi.Truncate(" "+text, lw, "")))
	default:
		now := m.now()
		end := min(m.offset+h, len(m.rows))
		// Row i shows commit i-wip; re-render just those graph lines with the
		// current selection highlighted.
		wip := 0
		if m.rows[0].Kind == RowWIP {
			wip = 1
		}
		from := max(m.offset-wip, 0)
		graphLines := m.layout.render(from, end-wip, m.cursor-wip)
		for i := m.offset; i < end; i++ {
			r := m.rows[i]
			if k := i - wip - from; r.Kind == RowCommit && k >= 0 && k < len(graphLines) {
				r.Graph = graphLines[k]
			}
			if r.Kind == RowCommit {
				if st, ok := m.stats[r.Commit.Hash]; ok {
					r.Stat = &st
				}
			}
			lines = append(lines, RenderRow(r, w, i == m.cursor, now))
		}
	}
	for len(lines) < headerLines+h {
		lines = append(lines, "")
	}
	if keep := m.height - statusLines; len(lines) > keep {
		lines = lines[:max(keep, 0)]
	}
	if m.mode == ModeDetail {
		lines = m.joinDetail(lines, lw)
	}
	lines = m.overlayFloat(lines)
	bar := m.statusBar()
	if m.mode == ModeSearch { // the query being typed
		bar = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colors.Fg)).
			Background(lipgloss.Color(colors.DimBg)).
			Width(m.width).
			Render(ansi.Truncate(m.Status(), m.width, "…"))
	}
	return strings.Join(append(lines, bar), "\n")
}
