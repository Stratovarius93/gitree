package ui

import (
	"fmt"
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Messages of the floating preview. key identifies the previewed row (the
// commit hash, or wipKey); a message that no longer matches is dropped.
type (
	previewFilesMsg struct {
		key     string
		files   []git.FileChange
		meta    *git.CommitMeta // nil for the working tree or when unreadable
		err     error
		refresh bool // reload of an open window: keep selection
	}
	previewDiffMsg struct {
		key, path, diff string
		err             error
	}
)

// wipKey is the preview key of the // WIP row (never a valid commit hash).
const wipKey = "wip"

// Overlay size: 65 % × 70 % of the screen, but never smaller than these
// minimums unless the terminal itself is smaller.
const (
	previewWidthPct  = 65
	previewHeightPct = 70
	previewMinWidth  = 34
	previewMinHeight = 10
)

// previewState is the floating window over the table (ModePreviewFiles and
// ModePreviewDiff).
type previewState struct {
	key     string // wipKey or the commit hash
	title   string
	commit  *git.Commit     // the previewed commit; nil for the working tree
	labels  []Label         // its branch and tag pills
	lane    int             // its graph column, which colors the pills
	meta    *git.CommitMeta // committer and co-authors, once loaded
	files   []git.FileChange
	sel     int // selected file
	off     int // first visible file
	err     string
	loading bool // the file list is loading

	vp          viewport.Model // diff of files[sel] in ModePreviewDiff
	diffLoading bool
}

// hash is the commit the preview shows, "" for the working tree.
func (p previewState) hash() string {
	if p.key == wipKey {
		return ""
	}
	return p.key
}

// previewBox is the overlay's outer size and position. ok is false when the
// screen is too small to draw a bordered window at all.
func (m Model) previewBox() (x, y, w, h int, ok bool) {
	avail := max(m.height-statusLines, 0) // the status bar stays visible
	w = max(m.width*previewWidthPct/100, min(m.width, previewMinWidth))
	h = max(avail*previewHeightPct/100, min(avail, previewMinHeight))
	if w < 3 || h < 3 {
		return 0, 0, 0, 0, false
	}
	return (m.width - w) / 2, (avail - h) / 2, w, h, true
}

// previewInner is the area inside the border.
func (m Model) previewInner() (w, h int) {
	_, _, bw, bh, _ := m.previewBox()
	return max(bw-2, 1), max(bh-2, 1)
}

// previewFooter is how many bottom lines (blank + footer) the window keeps
// below the diff. The file list has none: its header already shows the totals.
func (m Model) previewFooter(innerH int) int {
	if m.mode != ModePreviewDiff {
		return 0
	}
	switch {
	case innerH >= 4:
		return 2
	case innerH >= 2:
		return 1
	}
	return 0
}

// previewBodyHeight is the number of lines for the file list or the diff.
func (m Model) previewBodyHeight() int {
	_, ih := m.previewInner()
	return max(ih-m.previewFooter(ih), 1)
}

// openPreview opens the window on the selected row and loads its files.
func (m Model) openPreview() (tea.Model, tea.Cmd) {
	r := m.Selected()
	if r == nil {
		return m, nil
	}
	p := previewState{key: wipKey, title: "// WIP", loading: true}
	if r.Commit != nil {
		p.key = r.Commit.Hash
		p.title = shortHash(r.Commit.Hash) + " " + r.Commit.Subject
		c := *r.Commit
		p.commit, p.labels, p.lane = &c, r.Labels, r.Lane
	}
	m.preview = p
	m.mode = ModePreviewFiles
	return m, m.loadPreviewFiles(false)
}

// loadPreviewFiles loads the file list of the previewed row. refresh marks a
// reload of an open window, which keeps the file selection.
func (m Model) loadPreviewFiles(refresh bool) tea.Cmd {
	repo, hash, k := m.repo, m.preview.hash(), m.preview.key
	return func() tea.Msg {
		var files []git.FileChange
		var meta *git.CommitMeta
		var err error
		if hash == "" {
			files, err = repo.WorkingFiles()
		} else {
			files, err = repo.CommitFiles(hash)
			// The header falls back to the log's fields without it.
			if cm, merr := repo.CommitMeta(hash); merr == nil {
				meta = &cm
			}
		}
		return previewFilesMsg{key: k, files: files, meta: meta, err: err, refresh: refresh}
	}
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func (m Model) onPreviewFiles(msg previewFilesMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeList || msg.key != m.preview.key {
		return m, nil
	}
	m.preview.loading = false
	if msg.meta != nil {
		m.preview.meta = msg.meta
	}
	if msg.err != nil {
		m.preview.err = msg.err.Error()
		return m, nil
	}
	m.preview.err = ""
	if msg.refresh {
		p := &m.preview
		p.files = msg.files
		p.sel = max(min(p.sel, len(p.files)-1), 0)
		p.off = visibleOffset(p.off, p.sel, m.previewListRows(), len(p.files))
		if m.mode == ModePreviewDiff {
			if len(p.files) == 0 {
				m.mode = ModePreviewFiles
				return m, nil
			}
			return m, m.resizePreview()
		}
		return m, nil
	}
	m.preview.files = msg.files
	m.preview.sel, m.preview.off = 0, 0
	return m, nil
}

// openPreviewDiff switches to the second level for the selected file.
func (m Model) openPreviewDiff() (tea.Model, tea.Cmd) {
	if m.preview.sel >= len(m.preview.files) {
		return m, nil
	}
	m.mode = ModePreviewDiff
	return m, m.resizePreview()
}

// resizePreview (re)creates the diff viewport for the current window size and
// reloads the diff, which is rendered for the window width.
func (m *Model) resizePreview() tea.Cmd {
	iw, _ := m.previewInner()
	m.preview.vp = viewport.New(iw, m.previewBodyHeight())
	m.preview.vp.SetContent(" loading…")
	m.preview.diffLoading = true
	f := m.preview.files[m.preview.sel]
	repo, k, hash := m.repo, m.preview.key, m.preview.hash()
	return func() tea.Msg {
		d, err := repo.Diff(git.DiffTarget{Hash: hash, OldPath: f.OldPath}, f.Path, iw-1)
		return previewDiffMsg{key: k, path: f.Path, diff: d, err: err}
	}
}

func (m Model) onPreviewDiff(msg previewDiffMsg) (tea.Model, tea.Cmd) {
	p := &m.preview
	if m.mode != ModePreviewDiff || msg.key != p.key || p.sel >= len(p.files) || msg.path != p.files[p.sel].Path {
		return m, nil
	}
	p.diffLoading = false
	if msg.err != nil {
		p.vp.SetContent(" error: " + msg.err.Error())
		return m, nil
	}
	iw, _ := m.previewInner()
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(msg.diff, "\n"), "\n") {
		lines = append(lines, " "+ansi.Truncate(l, iw-1, "…"))
	}
	p.vp.SetContent(strings.Join(lines, "\n"))
	p.vp.GotoTop()
	return m, nil
}

// updatePreview routes keys while the window is open.
func (m Model) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Quit) {
		return m, tea.Quit
	}
	if m.mode == ModePreviewDiff {
		vp := &m.preview.vp
		switch {
		case key.Matches(msg, keys.Esc):
			m.mode = ModePreviewFiles
		case key.Matches(msg, keys.Down):
			vp.LineDown(1)
		case key.Matches(msg, keys.Up):
			vp.LineUp(1)
		case key.Matches(msg, keys.HalfDown):
			vp.HalfViewDown()
		case key.Matches(msg, keys.HalfUp):
			vp.HalfViewUp()
		case key.Matches(msg, keys.Top):
			vp.GotoTop()
		case key.Matches(msg, keys.Bottom):
			vp.GotoBottom()
		}
		return m, nil
	}
	p := &m.preview
	switch {
	case key.Matches(msg, keys.Esc):
		m.mode = ModeList
		m.preview = previewState{}
		return m, nil
	case key.Matches(msg, keys.Space), key.Matches(msg, keys.Enter):
		return m.openPreviewDiff()
	case key.Matches(msg, keys.Down):
		p.sel++
	case key.Matches(msg, keys.Up):
		p.sel--
	case key.Matches(msg, keys.Top):
		p.sel = 0
	case key.Matches(msg, keys.Bottom):
		p.sel = len(p.files) - 1
	case key.Matches(msg, keys.HalfDown):
		p.sel += max(m.previewListRows()/2, 1)
	case key.Matches(msg, keys.HalfUp):
		p.sel -= max(m.previewListRows()/2, 1)
	}
	p.sel = max(min(p.sel, len(p.files)-1), 0)
	p.off = visibleOffset(p.off, p.sel, m.previewListRows(), len(p.files))
	return m, nil
}

// visibleOffset keeps sel inside a window of rows lines over n items.
func visibleOffset(off, sel, rows, n int) int {
	if sel < off {
		off = sel
	}
	if sel >= off+rows {
		off = sel - rows + 1
	}
	return max(min(off, n-rows), 0)
}

// statusColor is the color of a file status letter.
func statusColor(s byte) string {
	switch s {
	case 'M':
		return colors.Yellow
	case 'A':
		return colors.Green
	case 'D':
		return colors.Red
	case 'R':
		return colors.Blue
	}
	return colors.Gray
}

// fileLabel is "path" or "old → new" for a renamed file.
func fileLabel(f git.FileChange) string {
	if f.OldPath != "" {
		return f.OldPath + " → " + f.Path
	}
	return f.Path
}

// padTo truncates s to w cells and pads it with spaces to exactly w.
func padTo(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// previewFileLine renders one row of the file list, w cells wide.
func previewFileLine(f git.FileChange, w int) string {
	letter := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(statusColor(f.Status))).Render(string(f.Status))
	return " " + letter + "  " + padTo(fileLabel(f), max(w-5, 0)) + " "
}

// previewBody is the inner lines of the window (excluding the border) and
// the index of the selected line (-1 none).
func (m Model) previewBody(iw, ih int) (body []string, sel int) {
	p := m.preview
	gray := floatMuted
	sel = -1
	body = make([]string, 0, ih)
	rows := m.previewBodyHeight()
	foot := m.previewFooter(ih)

	if m.mode == ModePreviewDiff {
		body = append(body, strings.Split(p.vp.View(), "\n")...)
	} else {
		body = append(body, m.previewHeader(iw)...)
		listRows := m.previewListRows()
		switch {
		case p.loading:
			body = append(body, gray.Render(" loading…"))
		case p.err != "":
			body = append(body, " error: "+p.err)
		case len(p.files) == 0:
			body = append(body, gray.Render(" no changes"))
		default:
			off := visibleOffset(p.off, p.sel, listRows, len(p.files))
			for i := off; i < min(off+listRows, len(p.files)); i++ {
				if i == p.sel {
					sel = len(body)
				}
				body = append(body, previewFileLine(p.files[i], iw))
			}
		}
	}
	for len(body) < rows {
		body = append(body, "")
	}
	body = body[:rows]
	if foot > 0 {
		if foot == 2 {
			body = append(body, "")
		}
		body = append(body, gray.Render(" esc back to list"))
	}
	return body, sel
}

func previewTotals(files []git.FileChange) (added, deleted int) {
	for _, f := range files {
		added += f.Added
		deleted += f.Deleted
	}
	return
}

// previewWindow is the floating preview: the file list or a file's diff.
func (m Model) previewWindow() (floatWin, bool) {
	x, y, w, h, ok := m.previewBox()
	if !ok {
		return floatWin{}, false
	}
	win := floatWin{title: m.preview.title, sel: -1, x: x, y: y, w: w, h: h}
	if m.mode == ModePreviewDiff && m.preview.sel < len(m.preview.files) {
		f := m.preview.files[m.preview.sel]
		win.title = f.Path
		win.right = fmt.Sprintf("+%d −%d", f.Added, f.Deleted)
	}
	win.body, win.sel = m.previewBody(w-2, h-2)
	return win, true
}
