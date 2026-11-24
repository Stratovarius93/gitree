package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Stratovarius93/gitree/internal/colors"
	"github.com/Stratovarius93/gitree/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sgr is the escape lipgloss emits for style (TrueColor is forced in TestMain).
func sgr(style lipgloss.Style) string {
	esc, _, _ := strings.Cut(style.Render("x"), "x")
	return esc
}

func bg3Esc() string { return sgr(lipgloss.NewStyle().Background(lipgloss.Color(colors.Bg3))) }

// floatModes opens every kind of overlay on a small repo and returns the
// model in each, keyed by name.
func floatModes(t *testing.T, w, h int) map[string]Model {
	t.Helper()
	r, hash := mixedRepo(t)
	r.Branch("feature")
	base := sized(loaded(t, r), w, h)
	out := map[string]Model{}
	out["preview files"] = press(t, base, "space")
	out["preview diff"] = press(t, base, "space", "space")
	out["menu"] = press(t, base, "x")
	out["picker"] = typeText(t, press(t, base, "b"), "feat")
	out["help"] = press(t, base, "?")
	c, _ := base.askConfirm("Merge", "Merge feature into main?", func(m Model) (Model, tea.Cmd) { return m, nil })
	out["confirm"] = c.(Model)
	in, _ := base.openInput(hash)
	out["input"] = in.(Model)
	return out
}

func TestFloatFillsBackground(t *testing.T) {
	bg := bg3Esc()
	border := sgr(lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Bg5)))
	border = strings.TrimSuffix(strings.TrimPrefix(border, "\x1b["), "m")
	for name, m := range floatModes(t, 120, 30) {
		win, ok := m.floatWindow()
		require.True(t, ok, name)
		box := win.render()
		require.Len(t, box, win.h, name)
		assert.Contains(t, box[0], border, "%s: border color", name)
		for i, l := range box {
			assert.Equal(t, win.w, lipgloss.Width(l), "%s line %d", name, i)
			assert.True(t, strings.HasPrefix(l, bg), "%s line %d starts without bg3: %q", name, i, l)
			body := strings.TrimSuffix(l, "\x1b[0m")
			for _, reset := range []string{"\x1b[0m", "\x1b[m"} {
				for j := strings.Index(body, reset); j >= 0; {
					rest := body[j+len(reset):]
					assert.True(t, strings.HasPrefix(rest, bg), "%s line %d: reset not followed by bg3: %q", name, i, l)
					k := strings.Index(rest, reset)
					if k < 0 {
						break
					}
					j += len(reset) + k
				}
			}
		}
	}
}

func TestFloatTitleAndSelection(t *testing.T) {
	modes := floatModes(t, 120, 30)
	title := strings.TrimSuffix(sgr(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors.FgFloat))), "m")
	sel := strings.TrimSuffix(strings.TrimPrefix(sgr(lipgloss.NewStyle().Background(lipgloss.Color(colors.Bg5))), "\x1b["), "m")
	for _, name := range []string{"preview files", "menu", "picker"} {
		win, ok := modes[name].floatWindow()
		require.True(t, ok)
		box := win.render()
		assert.Contains(t, box[0], title, name)
		assert.Contains(t, strings.Join(box, "\n"), sel, "%s: selected row bg", name)
	}
}

func TestFloatViewsFit(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {200, 50}} {
		for name, m := range floatModes(t, sz[0], sz[1]) {
			t.Run(name, func(t *testing.T) { assertFits(t, m) })
		}
	}
}

var sgrBefore = regexp.MustCompile(`\x1b\[([0-9;]*)m(feat)`)

func TestPickerHighlightsMatch(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	r.Branch("my-feature")
	m := typeText(t, press(t, sized(loaded(t, r), 100, 20), "b"), "FEAT")
	raw := m.View()
	ms := sgrBefore.FindAllStringSubmatch(raw, -1)
	require.NotEmpty(t, ms, "match is styled")
	found := false
	for _, mm := range ms {
		params := ";" + mm[1] + ";"
		if strings.Contains(params, ";38;2;254;128;25;") && strings.Contains(params, ";1;") {
			found = true
		}
	}
	assert.True(t, found, "match bold #fe8019: %q", raw)
	assert.Contains(t, ansi.Strip(raw), "my-feature")
}

func TestFloatFillUnit(t *testing.T) {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Red)).Render("a") + " b \x1b[mc"
	out := floatFill(s)
	assert.Equal(t, "a b c", ansi.Strip(out))
	assert.True(t, strings.HasPrefix(out, bg3Esc()))
	assert.Equal(t, 2, strings.Count(out, "\x1b[0m"+bg3Esc())+strings.Count(out, "\x1b[m"+bg3Esc()))
}

func TestShortTerminalKeepsDialogFooter(t *testing.T) {
	r := testutil.NewRepo(t)
	hash := r.Commit("base", nil)
	long := strings.Repeat("this question is long enough to wrap onto several lines ", 6) + "?"
	for h := 4; h <= 12; h++ {
		base := sized(loaded(t, r), 60, h)
		c, _ := base.askConfirm("Confirm", long, func(m Model) (Model, tea.Cmd) { return m, nil })
		win, ok := c.(Model).floatWindow()
		require.True(t, ok, "h=%d", h)
		assert.Contains(t, ansi.Strip(strings.Join(win.render(), "\n")), "y yes · n/esc no", "h=%d", h)
		in, _ := base.openInput(hash)
		if h < 5 {
			continue // one body line: the name field and its hint cannot both fit
		}
		im := in.(Model)
		im.input.err = "invalid branch name"
		win, ok = im.floatWindow()
		require.True(t, ok, "h=%d", h)
		assert.Contains(t, ansi.Strip(strings.Join(win.render(), "\n")), "enter create · esc cancel", "h=%d", h)
	}
}

func TestWithBgSurvivesBackgroundReset(t *testing.T) {
	s := "a\x1b[49mb"
	out := withBg(s, colors.DimBg)
	esc := sgr(lipgloss.NewStyle().Background(lipgloss.Color(colors.DimBg)))
	assert.Contains(t, out, "\x1b[49m"+esc)
}

func TestShortTerminalKeepsDialogQuestion(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", nil)
	text := "Detached HEAD: commits made here belong to no branch.\nIt can be undone with checkout.\nCheck out abc1234?"
	for h := 5; h <= 8; h++ {
		base := sized(loaded(t, r), 60, h)
		c, _ := base.askConfirm("Confirm", text, func(m Model) (Model, tea.Cmd) { return m, nil })
		win, ok := c.(Model).floatWindow()
		require.True(t, ok, "h=%d", h)
		out := ansi.Strip(strings.Join(win.render(), "\n"))
		assert.Contains(t, out, "Check out abc1234?", "h=%d", h)
		assert.Contains(t, out, "y yes · n/esc no", "h=%d", h)
	}
}
