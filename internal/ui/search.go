package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// searchState is the search over the loaded rows: input while typing, the
// committed query, and the indexes of the rows that match it.
type searchState struct {
	input textinput.Model
	query string
	hits  []int // ascending row indexes
}

// matches reports whether q appears (case-insensitively) in the row's subject,
// author name or label names, or is a prefix of its hash. WIP rows never match.
func matches(r Row, q string) bool {
	if r.Commit == nil || q == "" {
		return false
	}
	q = strings.ToLower(q)
	c := r.Commit
	if strings.HasPrefix(strings.ToLower(c.Hash), q) ||
		strings.Contains(strings.ToLower(c.Subject), q) ||
		strings.Contains(strings.ToLower(c.AuthorName), q) {
		return true
	}
	for _, l := range r.Labels {
		if strings.Contains(strings.ToLower(l.Name), q) {
			return true
		}
	}
	return false
}

// refreshSearch recomputes the hits after the rows changed.
func (m *Model) refreshSearch() {
	m.search.hits = nil
	if m.search.query == "" {
		return
	}
	for i, r := range m.rows {
		if matches(r, m.search.query) {
			m.search.hits = append(m.search.hits, i)
		}
	}
}

func (m Model) startSearch() (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Focus()
	m.search.input = ti
	m.mode = ModeSearch
	return m, nil
}

// updateSearch routes keys while the query is typed in the status line.
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case key.Matches(msg, keys.Esc):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, keys.Enter):
		m.mode = ModeList
		m.search.query = strings.TrimSpace(m.search.input.Value())
		m.refreshSearch()
		return m.jumpSearch(1)
	}
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	return m, cmd
}

// jumpSearch moves to the next (dir > 0) or previous hit, wrapping around.
// Without hits the cursor stays where it is.
func (m Model) jumpSearch(dir int) (tea.Model, tea.Cmd) {
	hits := m.search.hits
	if len(hits) == 0 {
		return m, nil
	}
	var at int
	if dir > 0 {
		at = sort.SearchInts(hits, m.cursor+1) % len(hits)
	} else {
		at = (sort.SearchInts(hits, m.cursor) - 1 + len(hits)) % len(hits)
	}
	return m.moveTo(hits[at])
}

// searchStatus is "/query i/n" or "/query no results".
func (m Model) searchStatus() string {
	n := len(m.search.hits)
	if n == 0 {
		return "/" + m.search.query + " no results"
	}
	i := sort.SearchInts(m.search.hits, m.cursor)
	if i < n && m.search.hits[i] == m.cursor {
		return fmt.Sprintf("/%s %d/%d", m.search.query, i+1, n)
	}
	return fmt.Sprintf("/%s -/%d", m.search.query, n)
}
