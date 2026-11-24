# gitree round 3: English UI, Neovim-style floats, full author names, richer preview header, pull/push

Date: 2026-11-23 · Status: approved by the user

## Why
The user loves the tool and wants to share it with the community (English), match
their Neovim theme (Gruvbox Flutter: rounded floats), see full author names,
get a richer commit summary in the `space` preview, and pull/push from gitree.

## 1. English everywhere
All UI copy (status bar, dialogs, menus, help, errors, labels such as "// WIP",
"N files changed"), README, code comments, and the design/plan docs under
docs/superpowers/ are translated to English. Header: BRANCH / TAG ┊ GRAPH ┊
MESSAGE ┊ AUTHOR ┊ DATE ┊ HASH. Relative dates: "now", "5m", "2h", "3d", "2w",
"4mo", "1y".

## 2. Neovim-style floating windows
Every overlay (preview, diff, menu, branch picker, help, confirm/input dialogs)
uses the Gruvbox Flutter float style: background #3c3836 (bg3) filling the whole
box, rounded border #504945 (bg5) on that background, text #ebdbb2 (fg), title in
bold #ebdbb2 (or aqua), selected row background #504945, filter matches in bold
#fe8019 (bright_orange), muted text #928374. Main table unchanged.

## 3. Full author names
AUTHOR column shows the full name, width 18 (adapts down on narrow terminals),
truncated with "…". Hiding order unchanged (DATE → AUTHOR → HASH).

## 4. Preview header (`space`)
Above the file list, a summary block, aligned key/value rows, colored:
subject (bold), Author name <email>, Co-authors (from Co-authored-by trailers,
also committer if different), Date (absolute + relative), Branches/tags (pills),
Changes: +added (green) −deleted (red), "N files: X M · Y A · Z D · W R" with
status colors. A separator line, then the file list. For the WIP row: "Working
tree", counts only. Git has no "modified lines" metric: a changed line counts as
one deletion + one addition.

## 5. Pull (`p`) and push (`P`)
- Both need `y` confirmation, showing what will happen ("Pull 3 commits from
  origin/develop into develop", "Push 2 commits to origin/develop").
- Pull: `git pull --ff-only`. Blocked with tracked changes (same as other
  actions). If branches diverged → no-op + message suggesting lazygit (L).
- Push: current branch to its upstream; without upstream `git push -u origin
  <branch>`. Never force. Rejected push (remote ahead) → message suggesting pull.
  Only ever to a same-named branch: when the upstream has a different name or
  is local ("."), the confirm offers "Push B to origin/B (new branch) and set
  upstream?" and runs `git push -u origin refs/heads/B:refs/heads/B`. A gone
  upstream is recreated after "Push B to origin/B (remote branch was deleted —
  recreate it)?"; pull on a gone upstream is refused. The confirmed plan is
  passed to the push, which refuses ("branch changed — try again") if it no
  longer matches. Pull/push refuse while a fetch runs; fetches skip while an
  action runs.
- Detached HEAD: both refused with a message. In-flight guard (`acting`) applies.
- Network ops use the fetch-style isolation (no tty, timeout, child kill).
- After success: reload, keep selection.

## Round 3b — refusal dialog and opt-in autostash pull
- Refusals made before git runs (tracked changes, diverged, operation in
  progress, detached HEAD, no/gone upstream, nothing to push, fetch running,
  control-character branch name) for pull, push, checkout, merge and
  cherry-pick open a floating dialog ("Can't pull", …) with the reason in one
  sentence and, for tracked changes, the changed files (status + path, at most
  8, then "+N more"). `esc`/`enter`/`n` close it. Errors after git ran stay in
  the status bar. A refusal arriving while another view is open goes to the
  status bar instead (keys meant for that view never land in the dialog).
- Pull with tracked changes: the dialog offers `s` "Stash changes, pull, and
  restore them". It opens a second question ("Stash N changed files, pull M
  commits from origin/x, then restore your changes?") accepted with `y` only;
  then `git pull --ff-only --no-rebase --autostash` runs with the same
  isolation and timeout. Plain `p` keeps `--no-autostash` and still refuses.
  Diverged / in progress / detached / no upstream are refused before. If the
  autostash cannot be re-applied (git keeps it in the stash), the pull stays
  applied and a dialog offers lazygit: "Pulled, but your changes conflicted
  when restored. They are safe in the stash — resolve in lazygit (L)?".
  This supersedes "never an automatic stash" for this path only: gitree
  stashes only after this explicit confirmation.
- git restores the autostash without `--index`: staged changes come back
  unstaged (content kept). The question adds "(staged changes will come back
  unstaged)" when any tracked change is staged.
- A pull killed after stashing but before restoring (timeout) leaves the
  changes only in `MERGE_AUTOSTASH`: gitree runs `git merge --quit`, which
  moves them into the stash, and offers lazygit ("The pull did not finish…").
  `MERGE_AUTOSTASH` counts as an operation in progress, so other actions
  refuse while it exists.
- A stash entry counts as gitree's autostash only when refs/stash moved and
  its reflog message is "autostash" (a stash pushed meanwhile by another tool
  is not misreported).
- Push is allowed with uncommitted changes (it never touches the working tree).

### Round 3b addendum — direct checkout, stash & check out, recommendations
- `c` in the list (and in the detail with list focus) checks out the selected
  row like `x` → `c`: the branch (chooser when the row has several, local /
  remote resolution as in the menu) or, on a commit without branches, the
  commit detached (with the detached warning). On the WIP row it only shows a
  status hint; on the current branch it says "already on <b>". Only `y`
  confirms.
- Every "Can't …" dialog (checkout, merge, cherry-pick, pull, push) is built
  by one function: reason sentence, details (changed tracked files, max 8 +
  "+N more"; "origin/x was deleted on the remote"; …) and "What you can do",
  one `key  description` line per option that applies: `s` stash (pull,
  checkout), `L` lazygit (dirty: commit or stash; in progress: finish or
  abort; diverged: rebase or merge; untracked files in the way: move or
  remove), `P` push to create / recreate the upstream, `f` fetch (nothing to
  push), `b` / `c` find / check out a branch (detached HEAD), advice without
  a key for a running fetch and control-character names. Footer `esc close`.
  Checkout, merge and cherry-pick refuse before asking (operation in
  progress, tracked changes); git's own refusals that change nothing ("would
  be overwritten by checkout/merge") use the same dialog.
- Stash & check out (`s` in "Can't check out", then `y` on "Stash N changed
  files, check out <b>, then restore your changes?"): `git stash push -m
  "gitree: checkout <b>"` (tracked only), the checkout, then `git stash pop
  --index`. A failed checkout pops the stash back on the original branch and
  reports the error; a conflicting pop keeps the stash (git's behaviour) and
  offers lazygit ("Checked out <b>, but your changes conflicted when restored.
  They are safe in the stash — resolve in lazygit (L)?"); a pop whose staged
  state can't be restored is retried without `--index` (changes come back
  unstaged, said in the status bar). Only gitree's own entry, while still
  `stash@{0}`, is ever popped; no failure path drops a stash entry. As with
  pull, gitree stashes only after this explicit confirmation.
- Safety fixes: HEAD (symbolic ref + hash) is recorded before checking out;
  a non-zero checkout that moved HEAD (failing `post-checkout` hook) counts
  as done — changes are restored on the new branch (or a restore conflict
  is reported) and "post-checkout hook failed: <line>" is shown; only when
  HEAD did not move are changes restored on the original branch. Plain
  checkouts report the hook the same way. Restoring uses `git stash apply
  [--index] <hash>` and drops the entry only while it is still `stash@{0}`
  (otherwise it is kept and the user told). A failed `stash push` says
  whether an entry was created. Kept/conflict errors name the entry. Every
  "changes restored" checkout failure opens the "Can't check out" dialog.
- `q` / `ctrl+c` while an action runs asks "An action is running. Quit
  anyway? Your changes may be left in the stash." (`y` only; a second
  `ctrl+c` forces). On startup and every reload, stash entries whose message
  starts with "gitree: checkout " (anywhere in `git stash list`) open a
  notice "Changes from an interrupted checkout are in the stash (<entry>)"
  with `L  Restore them in lazygit`, once per entry per session; listings
  made around an action are ignored.
