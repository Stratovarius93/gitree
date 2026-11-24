// Adapted from github.com/jesseduffield/lazygit pkg/gui/presentation/graph (MIT). See LICENSE in this directory.

package graph

import "github.com/gookit/color"

// Sprinter is the underlying gookit style; exported on Style so cachedSprint
// can type-switch on it.
type Sprinter interface {
	Sprint(a ...any) string
	Sprintf(format string, a ...any) string
}

// Style is an immutable foreground color plus optional bold, wrapping a
// gookit style.
type Style struct {
	basic *color.Color
	rgb   *color.RGBColor
	bold  bool

	Style Sprinter
}

var (
	FgDefault    = fromBasic(color.FgDefault)
	FgLightWhite = fromBasic(color.FgLightWhite)
	FgCyan       = fromBasic(color.FgCyan)
	FgRed        = fromBasic(color.FgRed)
	FgGreen      = fromBasic(color.FgGreen)
	FgYellow     = fromBasic(color.FgYellow)
	FgMagenta    = fromBasic(color.FgMagenta)

	// Nothing prints no escape codes at all.
	Nothing = Style{Style: color.Style(nil)}
)

func fromBasic(c color.Color) Style {
	s := Style{basic: &c}
	s.Style = s.derive()
	return s
}

// NewStyle returns a 24-bit foreground style from a hex color such as "#fabd2f".
func NewStyle(hex string) *Style {
	c := color.HEX(hex)
	s := Style{rgb: &c}
	s.Style = s.derive()
	return &s
}

func (s Style) Bold() Style {
	s.bold = true
	s.Style = s.derive()
	return s
}

func (s Style) Sprint(a ...any) string { return s.Style.Sprint(a...) }

func (s Style) derive() Sprinter {
	var opts color.Opts
	if s.bold {
		opts = color.Opts{color.OpBold}
	}
	if s.rgb != nil {
		st := &color.RGBStyle{}
		st.SetFg(*s.rgb)
		st.SetOpts(opts)
		return st
	}
	cs := color.Style{}
	if s.basic != nil {
		cs = append(cs, *s.basic)
	}
	for _, o := range opts {
		cs = append(cs, o)
	}
	return cs
}
