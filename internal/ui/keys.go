package ui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds the key bindings. Help text is reused by the help overlay.
type keyMap struct {
	Up, Down, Top, Bottom key.Binding
	HalfDown, HalfUp      key.Binding
	Scope                 key.Binding
	Enter, Tab, Esc       key.Binding
	Space                 key.Binding
	Search, Next, Prev    key.Binding
	Branch                key.Binding
	Menu, Copy, OpenPR    key.Binding
	Checkout              key.Binding
	Fetch, Pull, Push     key.Binding
	Lazygit               key.Binding
	Reload, Help          key.Binding
	Quit                  key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
	Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "half page down")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "half page up")),
	Scope:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "local / all branches")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/close details")),
	Space:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "preview")),
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch focus")),
	Esc:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search message, author, branches and hash")),
	Branch:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "find branch")),
	Next:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next result")),
	Prev:     key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "previous result")),
	Menu:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "actions menu")),
	Checkout: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "check out branch (or commit, detached)")),
	Copy:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy branch name or hash")),
	OpenPR:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open PR in browser")),
	Fetch:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fetch")),
	Pull:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull (fast-forward only)")),
	Push:     key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "push")),
	Lazygit:  key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "open lazygit in this repo")),
	Reload:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// helpBindings is the order of the key table in the help overlay.
func (k keyMap) helpBindings() []key.Binding {
	return []key.Binding{
		k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp,
		k.Search, k.Next, k.Prev, k.Branch,
		k.Enter, k.Tab, k.Esc, k.Space,
		k.Menu, k.Checkout, k.Copy, k.OpenPR, k.Fetch, k.Pull, k.Push, k.Scope, k.Lazygit, k.Reload,
		k.Help, k.Quit,
	}
}
