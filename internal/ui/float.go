package ui

import (
	"strings"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// floatWin is a Neovim-style floating window (NormalFloat/FloatBorder of the
// Gruvbox Flutter theme): every overlay is described by one and drawn by
// render, so they all share the same look and width safety.
type floatWin struct {
	title, right string   // border title (left) and note (right)
	body         []string // inner lines; padded or truncated to the inner width
	sel          int      // body line drawn with the selection background; -1 none
	x, y, w, h   int      // outer position and size, borders included
}

// floatWindow is the window of the open overlay; ok is false when no
// overlay is open or the screen is too small to draw one.
func (m Model) floatWindow() (floatWin, bool) {
	switch m.mode {
	case ModePreviewFiles, ModePreviewDiff:
		return m.previewWindow()
	case ModeHelp:
		return m.helpWindow()
	case ModeMenu, ModeConfirm, ModeInput, ModeNotice:
		return m.dialogWindow()
	case ModeBranchPicker:
		return m.pickerWindow()
	}
	return floatWin{}, false
}

// overlayFloat composites the open overlay over the table lines.
func (m Model) overlayFloat(lines []string) []string {
	win, ok := m.floatWindow()
	if !ok {
		return lines
	}
	return spliceBox(lines, win.render(), win.x, win.y, win.w, win.h)
}

// fitFloat sizes a window to title and content (each line plus extra cells),
// clamped to the screen and centred above the status bar.
func (m Model) fitFloat(title string, content []string, extra int) (floatWin, bool) {
	avail := max(m.height-statusLines, 0)
	w := lipgloss.Width(title) + 6
	for _, l := range content {
		w = max(w, lipgloss.Width(l)+extra)
	}
	w, h := min(w, m.width), min(len(content)+2, avail)
	if w < 3 || h < 3 {
		return floatWin{}, false
	}
	return floatWin{title: title, sel: -1, x: (m.width - w) / 2, y: (avail - h) / 2, w: w, h: h}, true
}

// Float styles.
var (
	floatBorder = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Bg5)).Background(lipgloss.Color(colors.Bg3))
	floatTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.FgFloat)).Background(lipgloss.Color(colors.Bg3))
	floatMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Gray))
	floatText   = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.FgFloat))
	floatBold   = floatText.Bold(true)
	floatMatch  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.BrightOrange))
)

// render draws the window: exactly h lines of exactly w cells.
func (f floatWin) render() []string {
	bd := lipgloss.RoundedBorder()
	iw := f.w - 2
	out := make([]string, 0, f.h)
	out = append(out, floatFill(borderTop(f.title, f.right, f.w, bd, floatBorder)))
	for i := 0; i < f.h-2; i++ {
		l := ""
		if i < len(f.body) {
			l = f.body[i]
		}
		l = padTo(l, iw)
		if i == f.sel {
			l = withBg(l, colors.Bg5)
		}
		out = append(out, floatFill(floatBorder.Render(bd.Left)+l+floatBorder.Render(bd.Right)))
	}
	out = append(out, floatFill(floatBorder.Render(bd.BottomLeft+strings.Repeat(bd.Bottom, iw)+bd.BottomRight)))
	return out
}

// floatEsc is the escape that sets the float background and text color;
// floatBgEsc sets the background only. Both are "" without color support.
func floatEsc() (both, bg string) {
	bg, _, _ = strings.Cut(lipgloss.NewStyle().Background(lipgloss.Color(colors.Bg3)).Render("x"), "x")
	fg, _, _ := strings.Cut(floatText.Render("x"), "x")
	return bg + fg, bg
}

// floatFill paints s on the float background with the float text color,
// re-applying them after every reset inside s so styled content never leaves
// a hole of terminal-default color.
func floatFill(s string) string {
	esc, bg := floatEsc()
	if esc == "" {
		return s
	}
	s = strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+esc,
		"\x1b[m", "\x1b[m"+esc,
		"\x1b[49m", "\x1b[49m"+bg,
	).Replace(s)
	return esc + s + "\x1b[0m"
}

// borderTop is "╭─ title ──── right ─╮" in exactly w cells.
func borderTop(title, right string, w int, bd lipgloss.Border, style lipgloss.Style) string {
	inner := w - 2
	if right != "" {
		right = " " + right + " "
		if lipgloss.Width(right)+4 > inner {
			right = ""
		}
	}
	title = ansi.Truncate(" "+title+" ", max(inner-lipgloss.Width(right)-1, 0), "…")
	fill := max(inner-1-lipgloss.Width(title)-lipgloss.Width(right), 0)
	if right != "" {
		right = floatTitle.Render(right)
	}
	return style.Render(bd.TopLeft+bd.Top) + floatTitle.Render(title) +
		style.Render(strings.Repeat(bd.Top, fill)) + right + style.Render(bd.TopRight)
}

// spliceBox composites box (w cells wide, h lines) over lines at (x, y).
func spliceBox(lines, box []string, x, y, w, h int) []string {
	for len(lines) < y+h {
		lines = append(lines, "")
	}
	out := append([]string(nil), lines...)
	for i, b := range box {
		row := out[y+i]
		left := ansi.Truncate(row, x, "")
		left += strings.Repeat(" ", max(x-lipgloss.Width(left), 0))
		out[y+i] = left + b + ansi.TruncateLeft(row, x+w, "")
	}
	return out
}

// highlightMatch renders name in base, truncated and padded to w cells, with
// the first case-insensitive occurrence of q in floatMatch.
func highlightMatch(name, q string, w int, base lipgloss.Style) string {
	name = ansi.Truncate(name, w, "…")
	q = strings.TrimSpace(q)
	rs, qs := []rune(name), []rune(strings.ToLower(q))
	at := -1
	if len(qs) > 0 {
		lower := []rune(strings.ToLower(name))
		if len(lower) == len(rs) {
			for i := 0; i+len(qs) <= len(lower); i++ {
				if string(lower[i:i+len(qs)]) == string(qs) {
					at = i
					break
				}
			}
		}
	}
	pad := strings.Repeat(" ", max(w-lipgloss.Width(name), 0))
	if at < 0 {
		return base.Render(name + pad)
	}
	match := floatMatch
	if bg := base.GetBackground(); bg != (lipgloss.NoColor{}) {
		match = match.Background(bg)
	}
	return base.Render(string(rs[:at])) + match.Render(string(rs[at:at+len(qs)])) + base.Render(string(rs[at+len(qs):])+pad)
}
