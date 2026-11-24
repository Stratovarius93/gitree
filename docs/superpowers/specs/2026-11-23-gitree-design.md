# gitree: design

Date: 2026-11-23 · Status: approved

> **Note:** pull and push are no longer non-goals: the round-3 design
> ([2026-11-23-gitree-round3-design.md](2026-11-23-gitree-round3-design.md))
> supersedes them with `p` (fast-forward pull) and `P` (non-forcing push).
> The AUTHOR column also shows full names now instead of initials.

## Intent and why

The user wants **"a GitKraken as a CLI, using lazygit as the base"**.

They tried `lazygit log` and love the graph: lines with rounded corners,
circles and colors. What it lacks compared to GitKraken:
- seeing **branch names** in their own column, instead of the hash;
- a table-like view with columns;
- the actions they used in GitKraken when selecting a branch or a commit;
- seeing the PRs of each branch.

They want **a separate app**, not something inside lazygit.

Already ruled out:
- `git-graph` (Rust): looks confusing compared to lazygit.
- Configuring lazygit: it only allows hiding the hash (`commitHashLength: 0`);
  it cannot show branch names.

Success: they open `gitree` in a large real-world repository (~20k commits) and see the tree as clearly
as in lazygit, with branches labelled and colored as in GitKraken. From there
they can review a commit and its diff, check out, merge, cherry-pick, create a
branch, copy data and open a branch's PR.

## Goals

1. Commit graph with the same algorithm and the same characters as lazygit
   (rounded corners, commit and merge circles).
2. GitKraken-style table view: columns Branch/Tag, Graph, Message, Author,
   Date, Hash, with a header and faint separators.
3. **Branch labels only at the tip of each branch**, aligned in their own
   column on the left and separated from the graph.
4. **Color per graph column**, as in GitKraken.
5. `// WIP` row for uncommitted changes; new remote commits via fetch.
6. GitHub PR status next to each branch that has one, with an icon according to
   its state.
7. Actions: checkout, merge, cherry-pick, create a branch from a commit, copy
   name/hash/message, copy the PR URL, open the PR in the browser.
8. Floating preview (`space`): list of files changed by a commit, and the diff
   of a file with another `space`.
9. A key that opens lazygit on the same repo for everything else.
10. Gruvbox palette (the same as the gruvbox-flutter-theme theme).

## Non-goals

- Other git actions: rebase, reset, revert, push, pull, deleting branches,
  stash, creating tags, resolving conflicts. That is what lazygit is for (`L`).
- Creating, approving or commenting on PRs. They are only shown and opened.
- Providers other than GitHub (GitLab, Bitbucket).
- Computing the "origin branch" of each commit (dropped: not reliable).
- Coloring by author (as lazygit does) or a fixed color per branch name.
- Drawing the graph with images (Kitty/iTerm2 protocols). Unicode text only.
- Mouse use. Windows.

## Anti-goals (would fail even if it works)

- Looking more confusing or cluttered than `lazygit log`.
- Being slow to open on a large real-world repository (~20k
  commits). It has to show something in under ~1 second. Fetch and PRs never
  block startup.
- An action changing the repo without the user having confirmed it, or losing
  uncommitted changes.

## Constraints

- Go (to reuse lazygit's code). Go is **not installed** yet: `brew install go`,
  and add it to the dotfiles `Brewfile`.
- lazygit's graph package is MIT: it is copied with its copyright notice and
  license, and the origin is stated in the README.
- Terminal: Warp with Cartograph Nerd Font (Nerd Font icons available).
- Requires `git`. Optional: `delta` (colored diff), `gh` (PRs), `lazygit`
  (`L` key). If an optional tool is missing, that feature is disabled with a
  notice; the app does not fail.
- The user has two `gh` accounts (work and personal). If the active account has
  no access to the repo, no PRs are shown and the status bar says
  "no access to PRs: check `gh auth status`".

## Approaches considered

1. **New Go + Bubble Tea app, copying lazygit's graph package.**
   ← **chosen.** The graph is identical to lazygit's and the rest (table,
   labels, colors, actions, PRs) is free. The cost is adapting ~800 lines that
   depend on a few internal lazygit types, and bringing in future graph
   improvements by hand (it changes little).
2. **Trimmed fork of lazygit.** Rejected: lazygit is large and tightly coupled;
   trimming it costs more than building something new and leaves a fork that is
   hard to maintain.
3. **New app that reads `git log --graph`.** Rejected: it would look like ASCII
   `git log --graph`, which is exactly what they did not like.

"Lazygit as the base" means: lazygit's graph, its aesthetics and its vim-style
keys. The actions use `git` and `gh` directly, not lazygit code.

## Design

### Main view

Full screen by default. Table with a fixed header and `┊` separators in dark
grey (so they are not confused with the graph lanes):

```
 BRANCH / TAG              ┊ GRAPH ┊ MESSAGE                    ┊ AUTHOR ┊ DATE  ┊ HASH
 ──────────────────────────┼───────┼────────────────────────────┼───────┼───────┼────────
  // WIP                   ┊ ◌     ┊ 3 files changed            ┊       ┊       ┊
   origin/develop          ┊ ●     ┊ fix api timeout            ┊ AP    ┊ 5m    ┊ a1b2c3d
   develop ↓1              ┊ ●     ┊ dev work                   ┊ JC    ┊ 2h    ┊ 2ffd523
   main  ☁                 ┊ ⏣─╮   ┊ Merge login                ┊ AP    ┊ 1d    ┊ 8e5cf15
   feature/login  #142   ┊ │ ●   ┊ login form                 ┊ JC    ┊ 2d    ┊ 9b3bf3b
                           ┊ ● │   ┊ main 3                     ┊ AP    ┊ 3d    ┊ e7b3d69
                           ┊ ●─╯   ┊ main 2                     ┊ JC    ┊ 4d    ┊ 5f2b412
   v1.0                    ┊ ●     ┊ main 1                     ┊ JC    ┊ 5d    ┊ 4f48fd5
```

| Column | Width | Content |
|---|---|---|
| Branch / Tag | fixed ~24 | Label(s), left-aligned. Empty if the commit is not the tip of any branch. |
| Graph | variable | As much as the lanes need (like lazygit). |
| Message | flexible | Takes the remaining space, cut with `…`. |
| Author | 4 | Initials. |
| Date | 5 | Relative (`5m`, `2h`, `3d`, `2w`), dimmed. |
| Hash | 7 | Short hash, dimmed. |

With little width (for example, with the detail panel open) these are hidden,
in order: Date, Author, Hash. Branch, Graph and Message are never hidden.

### Branch / Tag column

- "Pill"-style label: background in the color of the graph column where the
  branch tip is, dark text.
- Nerd Font icons: computer = local branch, cloud = remote. If the local branch
  and its remote point to the same commit, **a single label** with both icons.
  If they point to different commits, each has its own label; the remote one
  dimmed.
- Current branch (HEAD): bold, with a marker.
- Ahead/behind relative to upstream: `↑N` (commits not pushed), `↓N` (remote
  commits missing).
- Tags: Gruvbox yellow, with a tag icon.
- Several labels on one commit: the most important one is shown (HEAD > local >
  remote > tags) followed by `+N`. The detail lists them all.
- **PR:** if the branch has a PR, next to the name go an **icon showing its
  state** and the number (`#142`):

  | PR state | Icon (Nerd Font, octicons) | Color |
  |---|---|---|
  | Open | pull request | green |
  | Draft | pull request draft | grey |
  | Approved | pull request + check | bright green |
  | Changes requested | pull request + cross | red |
  | Merged | merge | purple |
  | Closed without merge | closed pull request | dimmed red |

  If a branch has several PRs, the most recent open one is shown; if none is
  open, the most recent one. The exact icons are chosen by the builder, within
  Nerd Font octicons, and documented in the help (`?`).

### Color per column

- Each graph column has a color, cycling over the Gruvbox palette (blue,
  green, purple, yellow, aqua, orange, red; final order at the builder's
  discretion, aiming for neighbours to be distinguishable).
- A vertical line takes the color of its column. Diagonals or curves (forks
  and merges) take the color of the column of the branch that splits off or
  arrives. The commit circle takes the color of its column.
- The label uses the color of the column of its tip commit.
- Accepted: a branch may change color between runs if it changes column.
- In lazygit the color comes from `getStyle(commit)` and is fixed when each
  line is created. The color logic of the copied package has to change to
  assign the style by position. The lane algorithm is not touched.

### Local changes (WIP)

- If there are uncommitted changes (including new untracked files), the
  `// WIP` row appears at the very top with a hollow circle on the current
  branch's lane and the summary ("3 files changed").
- When selected, the detail panel shows the files and the diff of the changes.
- It updates on reload (`r`), when returning from lazygit and after every
  action.

### Remote and fetch

- `f` runs `git fetch --all --prune` in the background. There is also an
  automatic fetch every 5 minutes while the app is open.
- Fetch only updates remote references; it never touches local branches or
  files.
- After the fetch, the view reloads keeping the selected commit. New remote
  commits appear as normal rows with their `origin/...` label, and the local
  branch shows `↓N`.
- Status bar: "fetching…", the time of the last fetch, or the error if it
  failed (no network, no credentials). A failure does not interrupt use.

### PRs (GitHub)

- If `origin` is GitHub and `gh` is available, the PRs of the visible branches
  are queried with `gh pr list --state all --json
  number,title,state,isDraft,reviewDecision,headRefName,baseRefName,url` (or
  equivalent), in the background on open and after each fetch.
- While loading, labels are shown without PR; when it arrives, they are added.
- Detail of a branch with a PR: title, number, state, review, target branch.

### Detail and diff

- `enter` splits the screen: list on the left (narrower), detail on the right.
- Commit: full hash, author and email, date, all labels, full message, changed
  files and diff.
- Diff: if `delta` exists, it goes through `delta` with the same Gruvbox flags
  as dotfiles' `lazygit/config.yml`, **fixed inside gitree**, with
  `--side-by-side` if the panel is wider than 160 columns. Otherwise `git show
  --color`.
- `tab` switches focus list ↔ detail. `esc` closes the detail.

### Floating preview (`space`)

A quick glance that floats over the table and closes, unlike the fixed side
panel of `enter`.

```
 List (space on a commit)               Inside the window (space on a file)
 ╭─ 9b3bf3b login form ─────────────╮   ╭─ lib/login/login_page.dart ──────────── +12 −3 ─╮
 │  M  lib/login/login_page.dart    │   │  14   │ class LoginPage extends StatelessW…    │
 │  A  lib/login/login_bloc.dart    │   │  15 - │   final String title;                  │
 │  D  lib/old/auth.dart            │   │  15 + │   final String title, subtitle;        │
 │  R  lib/a.dart → lib/b.dart      │   │  …                                             │
 │                                  │   │                                                │
 │  4 files  +58 −21                │   │  esc back to list                              │
 ╰──────────────────────────────────╯   ╰────────────────────────────────────────────────╯
```

- First level (list only): each file with its status: `M` modified (yellow),
  `A` added (green), `D` deleted (red), `R` renamed (blue, `old → new`).
  Footer with total files and `+N −N` lines.
- `j`/`k` to move; `space` on a file opens the second level: that file's diff
  (with `delta` if it exists, same flags as the detail panel), scrollable with
  `j`/`k`, `ctrl+d`/`ctrl+u`.
- `esc` in the diff returns to the list; `esc` in the list closes the window.
- Works on a commit, on the `// WIP` row (uncommitted changes, with `?` for
  untracked files) and on a branch (its tip commit).
- Merge commits: files are compared against the first parent (builder's
  decision if something else fits better, documenting it).
- Size: ~60–70 % of the screen, centred, rounded border like the rest.

### Actions menu (`x`, like GitKraken's right click)

The menu depends on the selected row. "Branch" = the row has at least one
branch label (if it has several, the menu asks which one first).

```
 On a branch:                          On a commit:
 ╭─ feature/login ───────────────╮     ╭─ 9b3bf3b login form ──────────╮
 │ c  Checkout                   │     │ c  Checkout (detached)        │
 │ m  Merge into develop         │     │ p  Cherry-pick onto develop   │
 │ b  Create branch here         │     │ b  Create branch here         │
 │ ───────────────────────────── │     │ ───────────────────────────── │
 │ y  Copy branch name           │     │ y  Copy hash                  │
 │ h  Copy hash                  │     │ Y  Copy short hash            │
 │ u  Copy PR URL #142           │     │ M  Copy message               │
 │ o  Open PR #142 in browser    │     ╰───────────────────────────────╯
 ╰───────────────────────────────╯
```

("develop" = the current branch, HEAD.)

Action rules:
- **Checkout of a local branch:** `git checkout <branch>`. Of a remote branch
  with no local copy: creates the local one tracking it (`git checkout
  --track`). Of a commit: detached checkout, with a notice of what it means.
- **Merge:** merges the selected branch into the current branch (`git merge
  <branch>`). Fast-forward if possible, as git does by default.
- **Cherry-pick:** applies the selected commit onto the current branch.
- **Create branch here:** asks for the name and creates the branch at the
  selected commit. Then asks whether to check it out.
- **Copy:** to the macOS clipboard (`pbcopy`), with a "copied" notice in the
  status bar.
- **Open PR:** in the default browser (`open <url>`).
- **Always confirm** before checkout, merge and cherry-pick, showing what is
  going to happen ("Merge feature/login → develop").
- **Uncommitted changes:** if the action would affect them, the user is warned
  and it is not run. Never an automatic stash.
- **Conflicts** (merge or cherry-pick): gitree warns and offers to open lazygit
  to resolve them. Conflict resolution is not rebuilt.
- Any git error is shown with its message, not hidden.
- After each action, it reloads and selects the result (the new commit or the
  branch).

Direct actions without the menu: `y` in the list copies the branch name (if the
row has a branch) or the hash (if not); `o` opens the PR if there is one.

### Keys (vim-style, like lazygit)

| Key | Action |
|---|---|
| `j`/`k`, arrows | move selection |
| `g`/`G` | top / bottom |
| `ctrl+d`/`ctrl+u` | half page |
| `/`, `n`/`N` | search message, author, branch names and hash |
| `enter` | open/close detail and diff |
| `tab` | switch focus list ↔ detail |
| `esc` | close detail, menu or search |
| `space` | floating preview (files → file diff) |
| `x` | actions menu |
| `y` | copy branch name or hash |
| `o` | open PR in the browser |
| `f` | fetch |
| `a` | toggle default view ↔ all branches |
| `L` | open lazygit on this repo; on exit, return to gitree and reload |
| `r` | reload |
| `?` | help (includes the icon legend) |
| `q` | quit |

### Data

- `git log --topo-order` with a custom format (hash, parents, `%D`
  decorations, author, date, subject), like lazygit
  (`pkg/commands/git_commands/commit_loader.go`).
- **Default view:** local branches and their upstreams, plus HEAD. No remote
  branches without a local copy, no other remotes, no `refs/stash`. Tags are
  shown only if they fall on visible commits.
- **"All" view (`a`):** `--all` without `refs/stash`.
- Paged loading: ~300 commits on open, more when approaching the end. The graph
  is recomputed for the total loaded.
- `↑N`/`↓N` with `git for-each-ref` (`%(upstream:track)`) or equivalent.
- Repo: the current directory or `gitree [path]`. If it is not a git repo, a
  clear message and exit with a code ≠ 0.

### Code structure (indicative)

```
gitree/
  main.go                 arguments, startup
  internal/git/           reading (log, refs, status) and actions (checkout, merge…)
  internal/github/        PRs via gh
  internal/graph/         copy of lazygit's package (+ lazygit's LICENSE)
  internal/colors/        Gruvbox palette and color per column
  internal/ui/            Bubble Tea model: table, detail, menu, dialogs, keys
```

The internal lazygit dependencies used by the graph (`models.Commit`,
`style.TextStyle`, `utils.Safe`, `utils.Partition`) are replaced by minimal
equivalents of our own. lazygit's graph tests (~970 lines) are copied and
adapted to check that the output is identical. The git actions are tested
against temporary repos created in the tests.

### Distribution

- Public repo `Stratovarius93/gitree`, at `~/Developer/gitree`.
- Install: `go install github.com/Stratovarius93/gitree@latest`. `go` is added
  to the `Brewfile` and the gitree install to dotfiles' `install.sh`.
- README in English (like dotfiles), with credits to lazygit.

## Decided by the user

- "GitKraken as a CLI using lazygit as the base", as a separate app.
- Labels only at the tip of each branch, aligned in their own column and
  separated from the graph.
- Table-like columns with separators (option B), hash in the last column.
- Color per column, as in GitKraken.
- Default view: local branches + their remotes; `a` shows all.
- Actions: checkout, merge, cherry-pick, create a branch from a commit; copy
  branch name, hash, PR URL; open the PR in the browser.
- PR state icon next to the branch name.
- `space` = floating preview of files, and another `space` = file diff; `x` =
  actions menu.
- A key to open lazygit.
- Go + Bubble Tea + lazygit's graph package.

## My proposals (correctable)

- Name `gitree`.
- `// WIP` row and fetch with `f` + automatic every 5 minutes.
- Merge and cherry-pick always into the current branch.
- Confirmation before each action; never an automatic stash; conflicts are
  resolved in lazygit.
- Merging the local + remote label when they point to the same commit; `+N`
  with several labels.
- PR states shown (open, draft, approved, changes requested, merged, closed)
  and their colors.
- Copy message and short hash as extras in the commit menu.
- Visible column header.

## Left to the builder

- Exact order of colors per column and how the color-by-position is
  implemented in the copied package.
- Exact Nerd Font glyphs for each icon.
- Load page size, exact column widths, date format.
- Internal structure of the Bubble Tea model and `bubbles` components.
- How to detect that an action "would affect" uncommitted changes (simplest and
  safest: block checkout, merge and cherry-pick if there are changes in tracked
  files).
- nvim integration (floating like lazygit): out of this first version.
