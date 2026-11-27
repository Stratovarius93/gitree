# CHANGES Column Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a CHANGES column between MESSAGE and AUTHOR with a fixed-length green/red `▬` bar and colored `+N -N` counts, loaded in the background.

**Architecture:** `git.Repo.Stats` returns per-commit line totals from one `git log --no-walk --numstat` call. The UI model caches them by hash and fetches missing hashes with a `tea.Cmd` after every log page; `View` attaches the cached stat to each row, and `RenderRow` draws the cell (a dim placeholder while the stat is missing).

**Tech Stack:** Go, Bubble Tea, lipgloss, testify, `internal/testutil` repos.

**Spec:** `docs/superpowers/specs/2026-11-27-gitree-changes-column-design.md`

## Global Constraints

- Bar glyph `▬`, always 10 cells; green `colors.Green`, red `colors.Red`, gray `colors.Gray`, loading `colors.Separator`.
- Column width 22: bar(10) + " " + plus slot(5) + " " + minus slot(5); each slot left-aligned, padded with spaces.
- Header title `CHANGES`.
- Shown only when total width ≥ 120; it is the first column hidden.
- Counts ≥ 10000 abbreviated: `10k` … `999k`, then `1M`… (integer division, no decimals).
- Stats errors never surface in the status bar and never block the graph.
- Public repo: no employer names in code, tests, or docs.

## Review Focus

- Merge / empty / binary-only commit: gray bar and gray `+0 -0`, never stuck on the loading placeholder → Task 1 `TestStatsMergeAndEmpty`, Task 3 `TestChangesCellZero`.
- Very large counts (`+125000`) must not overflow the 22-cell column → Task 3 `TestChangesCellLargeCounts`.
- Selected row: the cell keeps the selection background across its colored segments → Task 3 `TestRenderRowChangesSelected`.
- Narrow terminal (< 120, and 120–140 with a wide graph): CHANGES hides before the message drops below 30 or the graph is clipped; width invariants still hold → Task 2 `TestComputeWidthsChangesHidesFirst`.
- Live refresh: cached stats are reused, only new hashes fetched; a failed fetch leaves the placeholder and no status message → Task 4 `TestStatsCachedAcrossReload`, `TestStatsErrorIsSilent`.

---

### Task 1: `git.Repo.Stats`

**Files:**
- Create: `internal/git/stats.go`
- Test: `internal/git/stats_test.go`

**Interfaces:**
- Produces: `type Stat struct{ Added, Deleted int }`; `func (r *Repo) Stats(hashes []string) (map[string]Stat, error)` — every requested hash that git reports gets an entry (zero Stat when it has no diff); `len(hashes)==0` returns an empty map without running git.

- [ ] **Step 1: Write the failing tests**

```go
func TestStats(t *testing.T) {
	tr := testutil.NewRepo(t)
	h1 := tr.Commit("init", map[string]string{"a": "1\n2\n"})
	h2 := tr.Commit("two", map[string]string{"a": "1\n", "b": "x\ny\nz\n"})
	r, _ := Open(tr.Dir)
	st, err := r.Stats([]string{h1, h2})
	require.NoError(t, err)
	assert.Equal(t, Stat{Added: 2}, st[h1])            // root commit vs empty tree
	assert.Equal(t, Stat{Added: 3, Deleted: 1}, st[h2]) // a: -1, b: +3
}

func TestStatsMergeAndEmpty(t *testing.T) {
	// merge commit (as in TestCommitFilesMergeCommit) and
	// tr.Git("commit", "--allow-empty", "-m", "e"): both present in the map as Stat{}.
	// A binary-only commit (file content "\x00\x01") is also Stat{}.
}

func TestStatsNoHashes(t *testing.T) {
	// Stats(nil) → empty non-nil map, nil error.
}
```

- [ ] **Step 2: Run** `go test ./internal/git -run TestStats -v` — expect FAIL (undefined `Stats`).

- [ ] **Step 3: Implement `Stats` in `internal/git/stats.go`**

Run `r.run("log", "--no-walk=unsorted", "--format=%x1e%H", "--numstat", hashes..., "--")`. Split on `\x1e`; first line of each record is the hash (set `res[hash] = Stat{}`), remaining non-empty lines are `added\tdeleted\tpath` — `strconv.Atoi` each, a failed parse (binary `-`) counts 0, sum into the hash's Stat.

- [ ] **Step 4: Run** `go test ./internal/git -v` — expect PASS.

- [ ] **Step 5: Commit** `feat(git): per-commit line stats`

---

### Task 2: CHANGES column in the layout

**Files:**
- Modify: `internal/ui/table.go` (`Widths`, `ComputeWidths`, `layout`, `columns`, `sum`, `cells`, `RenderHeader`)
- Test: `internal/ui/table_test.go`

**Interfaces:**
- Produces: `Widths.Changes int`, `Widths.ShowChanges bool`, cell key `"changes"` ordered between `"message"` and `"author"`, constant `changesWidth = 22`, `minTotalChanges = 120`.

- [ ] **Step 1: Write the failing tests**

```go
func TestComputeWidthsChanges(t *testing.T) {
	w := ComputeWidths(160, 6, 0, 18)
	assert.True(t, w.ShowChanges)
	assert.Equal(t, 22, w.Changes)
	assert.Equal(t, 160-24-6-22-18-5-7-6*3, w.Message)
	assert.Contains(t, ansi.Strip(RenderHeader(w)), "CHANGES")
	keys := []string{}
	for _, c := range w.cells() { keys = append(keys, c.key) }
	assert.Equal(t, []string{"label", "graph", "message", "changes", "author", "date", "hash"}, keys)
}

func TestComputeWidthsChangesHidesFirst(t *testing.T) {
	assert.False(t, ComputeWidths(119, 6, 0, 18).ShowChanges)
	assert.True(t, ComputeWidths(119, 6, 0, 18).ShowDate)
	for total := 120; total <= 200; total++ {
		for _, g := range []int{6, 30, 60} {
			w := ComputeWidths(total, g, 0, 18)
			if w.ShowChanges {
				assert.GreaterOrEqual(t, w.Message, 30, "total %d graph %d", total, g)
				assert.Equal(t, g, w.Graph, "total %d graph %d", total, g)
			}
		}
	}
}
```

Also update `TestComputeWidthsWide` (expect the CHANGES column at 160) and add `w.Changes` to the non-negative loop in `TestComputeWidthsInvariants`. Do not loosen any other assertion.

- [ ] **Step 2: Run** `go test ./internal/ui -run 'ComputeWidths|RenderHeader' -v` — expect FAIL.

- [ ] **Step 3: Implement**

`layout` gains a `changes bool` parameter: when true and `total >= minTotalChanges`, `ShowChanges = true, Changes = 22`. `ComputeWidths` first lays out with CHANGES (running its existing author-shrink loop); if the result has `ShowChanges` and `Message < 30 || Graph < graphWidth`, it reruns the whole computation with `changes=false`. Header title `"CHANGES"`.

- [ ] **Step 4: Run** `go test ./internal/ui -v` — expect PASS (fix any other existing test whose expected widths assumed no CHANGES column at ≥120, updating the expected number only).

- [ ] **Step 5: Commit** `feat(ui): CHANGES column in the table layout`

---

### Task 3: Render the CHANGES cell

**Files:**
- Create: `internal/ui/changes.go`
- Modify: `internal/ui/rows.go` (`Row`, `RenderRow`)
- Test: `internal/ui/changes_test.go`

**Interfaces:**
- Consumes: `git.Stat` (Task 1), `Widths.Changes`, cell key `"changes"` (Task 2).
- Produces: `Row.Stat *git.Stat` (nil = not loaded); `func changesCell(st *git.Stat, base lipgloss.Style) string` (exactly 22 cells); `func greenCells(added, deleted int) int`; `func fmtCount(n int) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestGreenCells(t *testing.T) {
	cases := []struct{ a, d, want int }{
		{42, 18, 7}, {6, 0, 10}, {0, 125, 0}, {310, 47, 9}, {1, 1, 5},
		{300, 2, 9},  // a deleted line still gets a red cell
		{2, 300, 1},  // an added line still gets a green cell
		{0, 0, 0},
	}
	for _, c := range cases { assert.Equal(t, c.want, greenCells(c.a, c.d), "%+v", c) }
}

func TestFmtCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 9999: "9999", 10000: "10k", 125000: "125k", 999999: "999k", 1000000: "1M", 25000000: "25M"} {
		assert.Equal(t, want, fmtCount(n))
	}
}

func TestChangesCell(t *testing.T) {
	s := changesCell(&git.Stat{Added: 42, Deleted: 18}, lipgloss.NewStyle())
	assert.Equal(t, 22, lipgloss.Width(s))
	assert.Equal(t, "▬▬▬▬▬▬▬▬▬▬ +42   -18  ", ansi.Strip(s))
}

func TestChangesCellZero(t *testing.T)        { /* Stat{} → "▬▬▬▬▬▬▬▬▬▬ +0    -0   ", width 22 */ }
func TestChangesCellLoading(t *testing.T)     { /* nil → 10 bar cells + 12 spaces, width 22 */ }
func TestChangesCellLargeCounts(t *testing.T) { /* {125000, 3} → "+125k -3   ", width 22 */ }

func TestRenderRowChangesSelected(t *testing.T) {
	// A commit Row with Stat set, w := ComputeWidths(160, 6, 0, 18):
	// RenderRow(..., selected=true) has width w.sum(), contains "+42" after the
	// message and before the author, and the SelBg background escape appears
	// inside the CHANGES cell segment.
}
```

Color is asserted by the cell's segments when lipgloss renders color in tests (follow how `tint_test.go` checks colors); otherwise the stripped-text assertions above are the contract.

- [ ] **Step 2: Run** `go test ./internal/ui -run 'GreenCells|FmtCount|ChangesCell|RenderRowChanges' -v` — expect FAIL.

- [ ] **Step 3: Implement in `internal/ui/changes.go`**

`greenCells`: total 0 → 0; else `n = (10*a + t/2) / t`; then `if a > 0 && n == 0 { n = 1 }`; `if d > 0 && n == 10 { n = 9 }`. `changesCell`: nil → bar in `colors.Separator` + padding; total 0 → gray bar; else green then red segments; numbers `"+"+fmtCount(a)` / `"-"+fmtCount(d)`, each padded to 5, gray when 0. Every segment uses `base.Foreground(...)` so the selection background carries through. In `RenderRow`: WIP rows `text("", w.Changes, colors.Gray)`; commit rows `fit(changesCell(r.Stat, base), w.Changes)`; add `"changes"` to `byKey`.

- [ ] **Step 4: Run** `go test ./internal/ui -v` — expect PASS.

- [ ] **Step 5: Commit** `feat(ui): CHANGES cell with ratio bar and counts`

---

### Task 4: Load stats in the background

**Files:**
- Modify: `internal/ui/model.go` (`Model`, `New`, log-loaded handler, `View`, message types)
- Test: `internal/ui/changes_model_test.go`

**Interfaces:**
- Consumes: `Repo.Stats` (Task 1), `Row.Stat` (Task 3).
- Produces: `Model.stats map[string]git.Stat` (initialized in `New`); `statsLoadedMsg{stats map[string]git.Stat; err error}`; `func (m Model) loadStats(commits []git.Commit) tea.Cmd` (nil when every hash is cached).

- [ ] **Step 1: Write the failing tests**

```go
func TestStatsLoadAfterLog(t *testing.T) {
	r := testutil.NewRepo(t)
	h := r.Commit("first", map[string]string{"a.txt": "1\n2\n3\n"})
	m := loaded(t, r)
	assert.Equal(t, git.Stat{Added: 3}, m.stats[h])
	m, _ = update(m, tea.WindowSizeMsg{Width: 160, Height: 20})
	assert.Contains(t, ansi.Strip(m.View()), "+3")
}

func TestStatsPlaceholderBeforeLoad(t *testing.T) {
	// Feed only the logLoadedMsg (not the stats cmd result): View at width 160
	// shows the CHANGES header and no "+" count on the commit row.
}

func TestStatsCachedAcrossReload(t *testing.T) {
	// loaded → commit again → full reload settles: loadStats for the reloaded
	// page requests only the new hash (assert loadStats(m.commits) is nil after
	// settling, and m.stats has both hashes).
}

func TestStatsErrorIsSilent(t *testing.T) {
	// update(m, statsLoadedMsg{err: errors.New("boom")}): status unchanged,
	// m.stats unchanged, no cmd.
}
```

- [ ] **Step 2: Run** `go test ./internal/ui -run TestStats -v` — expect FAIL.

- [ ] **Step 3: Implement**

`loadStats` collects hashes not in `m.stats`, returns a cmd calling `repo.Stats` → `statsLoadedMsg`. In the `logLoadedMsg` handler, after `m.relayout()`, compute `sc := m.loadStats(msg.commits)` and include it in every `tea.Batch`/return on the success path. Handle `statsLoadedMsg`: on error return `m, nil`; else merge into `m.stats`. In `View`, before `RenderRow`, for commit rows set `r.Stat` from `m.stats` when present (copy to a local).

- [ ] **Step 4: Run** `go test ./... ` — expect PASS. Then `go build -o gitree . && ./gitree` in this repo at ≥120 columns: column appears, fills in, colors match the prototype.

- [ ] **Step 5: Commit** `feat(ui): load commit stats in the background`
