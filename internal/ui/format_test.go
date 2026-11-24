package ui

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{3 * day, "3d"},
		{15 * day, "2w"},
		{70 * day, "2mo"},
		{400 * day, "1y"},
		{-time.Hour, "now"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RelativeTime(now.Add(-c.ago), now), c.want)
	}
}

// deferredMsg is what a timer (after) yields in tests: msg, due after d.
// collect delivers short timers (debounces) at once and drops long ones
// (status expiry), so settling never sleeps.
type deferredMsg struct {
	d   time.Duration
	msg tea.Msg
}

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	after = func(d time.Duration, msg tea.Msg) tea.Cmd {
		return func() tea.Msg { return deferredMsg{d: d, msg: msg} }
	}
	os.Exit(m.Run())
}
