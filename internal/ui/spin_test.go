package ui

import (
	"testing"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusShowsSpinnerWhileFetching(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	m, _ = update(m, keyMsg("f"))
	assert.Contains(t, m.Status(), "fetching ⠋")
	assert.NotContains(t, m.Status(), "fetching…")
}

func TestStatusShowsSpinnerWhilePulling(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)

	m.acting = true
	m.setStatus("action", "pulling…")
	assert.Contains(t, m.Status(), "pulling ⠋")

	m.acting = false
	m.setStatus("action", "pulled 2 commits")
	assert.NotContains(t, m.Status(), "⠋")
}

func TestStatusShowsSpinnerWhileReloading(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	assert.NotContains(t, m.Status(), "loading")

	m, _ = update(m, keyMsg("r"))
	assert.Contains(t, m.Status(), "loading ⠋")
}

func TestSpinnerTicksOnlyWhileBusy(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r).EnableSpinner()
	require.False(t, m.busy())

	// idle: a stray tick is dropped and ends the animation
	m2, cmd := update(m, m.spin.Tick())
	assert.Nil(t, cmd)
	assert.False(t, m2.animating)

	// becoming busy starts the animation once
	m, cmd = update(m, keyMsg("f"))
	require.True(t, m.animating)
	require.NotNil(t, cmd)
	_, cmd = update(m, keyMsg("j"))
	assert.Nil(t, cmd, "already animating: no second tick chain")
}

func TestSpinnerFrameAdvances(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r).EnableSpinner()
	m, _ = update(m, keyMsg("f"))
	require.Contains(t, m.Status(), "⠋")

	tick := spinner.TickMsg{ID: m.spin.ID()}
	m, cmd := update(m, tick)
	assert.NotNil(t, cmd, "busy: the chain continues")
	assert.Contains(t, m.Status(), "fetching ⠙")
}

func TestSpinnerDisabledNeverTicks(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m, _ = update(m, keyMsg("f"))
	assert.False(t, m.animating)
}

func TestPanelPlaceholderSpins(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m, _ = update(m, keyMsg("enter")) // detail opens, loading
	require.Equal(t, ModeDetail, m.mode)
	assert.Contains(t, ansi.Strip(m.View()), "loading ⠋")
	var _ tea.Model = m
}

func TestStatusShowsSpinnerWhileCheckingOut(t *testing.T) {
	r, _ := mergeRepo(t)
	m := loaded(t, r)
	m = selectWhere(t, m, hasLabel("feature/login"))
	m = press(t, m, "c")
	require.Equal(t, ModeConfirm, m.mode)

	m, cmd := update(m, keyMsg("y"))
	assert.Contains(t, m.Status(), "checking out ⠋")
	m = settle(t, m, cmd)
	assert.NotContains(t, m.Status(), "checking out")
}

func TestCheckingOutStatusClearedWhenRefused(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Commit("first", map[string]string{"a.txt": "a"})
	m := loaded(t, r)
	m.acting = true
	m.setStatus("action", "checking out…")

	m, _ = update(m, actionDoneMsg{op: "checkout", err: git.ErrDirty})
	assert.NotContains(t, m.Status(), "checking out")
}
