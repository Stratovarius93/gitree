package ui

import (
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/github"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func itemKeys(items []MenuItem) []string {
	var ks []string
	for _, it := range items {
		if !it.Separator {
			ks = append(ks, it.Key)
		}
	}
	return ks
}

func itemLabel(items []MenuItem, k string) string {
	for _, it := range items {
		if it.Key == k && !it.Separator {
			return it.Label
		}
	}
	return ""
}

func TestBranchMenuItems(t *testing.T) {
	pr := &github.PR{Number: 142, URL: "https://github.com/o/r/pull/142"}
	l := Label{Name: "feature/login", Kind: LabelBranch, Local: true, PR: pr}
	items := BranchMenu(l, "develop")
	assert.Equal(t, []string{"c", "m", "b", "y", "h", "u", "o"}, itemKeys(items))
	assert.Equal(t, "Merge into develop", itemLabel(items, "m"))
	assert.Equal(t, "Copy PR URL #142", itemLabel(items, "u"))
	assert.Equal(t, "Open PR #142 in browser", itemLabel(items, "o"))
	assert.Equal(t, ActMerge, items[1].Action)

	l.PR = nil
	assert.Equal(t, []string{"c", "m", "b", "y", "h"}, itemKeys(BranchMenu(l, "develop")))

	cur := Label{Name: "develop", Kind: LabelBranch, Local: true, IsHead: true}
	assert.Equal(t, []string{"c", "b", "y", "h"}, itemKeys(BranchMenu(cur, "develop")))
	// detached HEAD: nothing to merge into
	assert.Equal(t, []string{"c", "b", "y", "h"}, itemKeys(BranchMenu(l, "")))
}

func TestCommitMenuItems(t *testing.T) {
	c := git.Commit{Hash: "9b3bf3b0000000000000000000000000000000", Subject: "login form"}
	items := CommitMenu(c, "develop")
	assert.Equal(t, []string{"c", "p", "b", "y", "Y", "M"}, itemKeys(items))
	assert.Equal(t, "Check out (detached)", itemLabel(items, "c"))
	assert.Equal(t, "Cherry-pick onto develop", itemLabel(items, "p"))
	assert.Equal(t, []string{"c", "b", "y", "Y", "M"}, itemKeys(CommitMenu(c, "")))
	merge := git.Commit{Hash: c.Hash, Parents: []string{"a1", "b2"}, Subject: "Merge x"}
	assert.Equal(t, []string{"c", "b", "y", "Y", "M"}, itemKeys(CommitMenu(merge, "develop")), "no cherry-pick for merges")
}

// selectWhere moves the cursor to the first row satisfying pred.
func selectWhere(t *testing.T, m Model, pred func(Row) bool) Model {
	t.Helper()
	for i, r := range m.rows {
		if pred(r) {
			nm, cmd := m.moveTo(i)
			return settle(t, nm.(Model), cmd)
		}
	}
	t.Fatal("no matching row")
	return m
}

func hasLabel(name string) func(Row) bool {
	return func(r Row) bool {
		for _, l := range r.Labels {
			if l.Name == name {
				return true
			}
		}
		return false
	}
}

func atHash(h string) func(Row) bool {
	return func(r Row) bool { return r.Commit != nil && r.Commit.Hash == h }
}

func TestMenuAskWhichBranch(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("base", map[string]string{"a": "1"})
	r.Branch("alpha")
	r.Branch("beta")
	r.Commit("main moves", map[string]string{"a": "2"})
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("alpha"))

	m = press(t, m, "x")
	require.Equal(t, ModeMenu, m.mode)
	require.Len(t, m.menu.choices, 2)
	v := viewText(m)
	assert.Contains(t, v, "alpha")
	assert.Contains(t, v, "beta")

	second := m.menu.choices[1].Name
	assert.ElementsMatch(t, []string{"alpha", "beta"}, []string{m.menu.choices[0].Name, second})
	m = press(t, m, "2")
	assert.Equal(t, ModeMenu, m.mode)
	assert.Nil(t, m.menu.choices)
	assert.Equal(t, second, m.menu.label.Name)
	assert.Equal(t, []string{"c", "m", "b", "y", "h"}, itemKeys(m.menu.items))

	m = press(t, m, "esc")
	assert.Equal(t, ModeList, m.mode)
}

func TestMenuOnCommitAndWIP(t *testing.T) {
	r := testutil.NewRepo(t)
	first := r.Commit("first", map[string]string{"a": "1"})
	r.Commit("second", map[string]string{"a": "2"})
	writeFile(t, r, "a", "dirty")
	m := loaded(t, r)
	require.Equal(t, RowWIP, m.rows[0].Kind)
	m = press(t, m, "x")
	assert.Equal(t, ModeList, m.mode, "no menu on the WIP row")

	m = selectWhere(t, m, atHash(first))
	m = press(t, m, "x")
	require.Equal(t, ModeMenu, m.mode)
	assert.Nil(t, m.menu.label)
	assert.Equal(t, []string{"c", "p", "b", "y", "Y", "M"}, itemKeys(m.menu.items))
	assert.Contains(t, viewText(m), "Cherry-pick onto main")
}
