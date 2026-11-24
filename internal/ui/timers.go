package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// after returns a command that delivers msg once d has passed. Timed messages
// carry a sequence number and are ignored when they are no longer current.
// Tests replace it so that timers never block (see TestMain).
var after = func(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// detailDebounce is how long the cursor must rest before the detail panel
// loads the selected row: scrolling through the list does not run git for
// every row passed.
const detailDebounce = 120 * time.Millisecond

// statusTTL is how long a transient status message ("copied", warnings)
// stays in the status bar.
const statusTTL = 3 * time.Second
