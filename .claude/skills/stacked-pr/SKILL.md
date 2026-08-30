---
name: stacked-pr
description: >
  Plan and raise pull requests under this repository's 800-line cap, including
  splitting an oversized change into a stack of reviewable slices plus a merge
  vehicle. Use when about to open a pull request, when asked to raise, split,
  stack, or restack PRs, when a diff may exceed 800 lines, when starting work
  that looks large enough to need splitting, and when the user says "stacked
  PR", "split this PR", "PR too big", "merge vehicle", or "shadow PR".
---

# Stacked PRs

Policy and reasoning live in [`docs/PR_WORKFLOW.md`](../../../docs/PR_WORKFLOW.md).
This skill is the procedure. When the two disagree, the document wins and this
file is wrong.

The rule in one line: **no PR over 800 countable lines; over that, split into
slices and add one merge vehicle that carries the whole change and is the only
PR that merges.**

## Two entry points

Pick by where the work is.

- **Plan time** — before or early in writing code. This is the one that
  matters; a split decided here is nearly free.
- **Raise time** — the code exists and PRs need opening.

If work is already finished and oversized, run plan time anyway on the existing
commits before touching `gh`.

---

## Plan time

### 1. Estimate the size

If a comparable change exists, measure it. Otherwise count the files the change
will touch and assume 60–120 lines each for new code, less for edits.

Under 800: one PR. Stop — no stack, no merge vehicle. Say so and move on.

### 2. Name the seams out loud

List the slices before writing code, each as one sentence: what it is
responsible for, and what it deliberately leaves to a later slice.

Cut bottom-up along the dependency direction — `pkg/*` framework packages,
then `core/internal/*` engine packages, then `core/internal/cmd` +
`core/cmd/tai` wiring with its e2e tests, then `plugins/<name>/*` bottom-up
(storage → cmdframework → domain → cmd), then spec/docs. Full heuristics and
the three forbidden seams (tests split from their subject; a `test-cases.md`
TC entry split from the test naming it; an OpenSpec archive split from its
implementation) are in the document.

Apply the isolation test to each proposed seam:

> Could this slice be merged on its own and leave the repository working?
> (`go test ./... && go vet ./... && gofmt -l .` clean, and no
> `test-cases.md` entry contradicting the code.)

If a seam fails it, move the boundary. If no set of boundaries passes, the
**change** is too broad — say so and propose narrowing the change instead of
splitting the PR.

### 3. Commit along the seams

This is what makes raise time trivial. **One commit, or one contiguous group of
commits, per slice, in slice order.** Then every slice branch is just a pointer
at a commit already in history — no cherry-picking, no rebasing, and slice N's
tip is byte-identical to the merge vehicle's tip by construction.

Conventional commits with scope, as always.

Get this wrong and raise time needs the fallback in step 6, which is worse.

---

## Raise time

### 4. Set up the branch

One branch per change, off a current `main`:

```bash
git fetch origin
git switch -c <type>/<scope>-<slug> origin/main
git rev-list --count HEAD..origin/main   # must print 0
```

(For parallel work, put the branch in its own worktree under
`.claude/worktrees/` — optional, not required.)

**This branch is the merge vehicle.** Work happens on it, review fixes land on
it, and it is the branch that merges. Slices are carved out of its history.

### 5. Measure

```bash
# No $-positional field refs here: the skill loader substitutes them
# with invocation arguments, so the count reads fields via `read`.
git diff -M --numstat origin/main...HEAD -- . ':(exclude)go.sum' | {
  a=0; d=0
  while read -r add del _; do
    case "$add" in *[!0-9]*) continue ;; esac   # skip binary "-" rows
    a=$((a+add)); d=$((d+del))
  done
  echo "additions=$a deletions=$d total=$((a+d))"
}
```

Under 800: raise one PR against `main` and stop.

Over 800: continue. Slice count is `ceil(total / 800)` as a floor — the seams
decide the real count, and seams that produce a 400-line slice and a 750-line
slice are better than seams that produce two 575-line slices.

### 6. Create the slice branches

With commits already grouped by seam (step 3), each slice is a branch pointing
at the last commit of its group. No history is rewritten.

```bash
git log --oneline origin/main..HEAD          # identify each group's last commit
git branch <type>/<scope>-<slug>-1 <sha-end-of-group-1>
git branch <type>/<scope>-<slug>-2 <sha-end-of-group-2>
# … through N, where <sha-end-of-group-N> is HEAD
```

Slice N therefore points at the same commit as the merge vehicle. That is
correct, and it is what makes the last slice and the merge vehicle identical in
content while differing in base.

Verify before pushing — every slice must be an ancestor of the merge vehicle,
and slice N must equal it:

```bash
for b in $(git branch --list "<type>/<scope>-<slug>-*" --format='%(refname:short)'); do
  git merge-base --is-ancestor "$b" HEAD && echo "ok  $b" || echo "ERR $b not in history"
done
git rev-parse HEAD "<type>/<scope>-<slug>-N"   # two identical SHAs
```

**Fallback for history that is not grouped by seam:** create each slice branch
off its predecessor and cherry-pick the commits belonging to it, then rebuild
the merge vehicle on top of slice N. Avoid needing this — it rewrites the
branch that has already been published.

### 7. Push

```bash
git push -u origin <type>/<scope>-<slug>-1 … <type>/<scope>-<slug>-N
git push -u origin HEAD:<type>/<scope>-<slug>
```

### 8. Open the PRs — slices as drafts, merge vehicle ready

Slices chain their bases; the merge vehicle targets `main`.

The slices open as drafts because a draft cannot be merged by accident, and a
slice on `main` is a partial change on the branch every release is tagged
from. The merge vehicle opens ready for review because it is the PR that
merges: GitHub refuses `gh pr merge` on a draft and will not enable auto-merge
on one, so a draft merge vehicle only adds a step that has to be undone before
it can land.

```bash
# slice 1
gh pr create --draft --base main \
  --head <type>/<scope>-<slug>-1 \
  --title "<type>(<scope>): <slice 1 subject> (1/N)" --body-file -

# slice k, for k = 2..N
gh pr create --draft --base <type>/<scope>-<slug>-$((k-1)) \
  --head <type>/<scope>-<slug>-$k \
  --title "<type>(<scope>): <slice k subject> (k/N)" --body-file -

# merge vehicle — not a draft
gh pr create --base main \
  --head <type>/<scope>-<slug> \
  --title "<type>(<scope>): <whole change subject>" --body-file -
```

Bodies must carry the map, or the stack is unnavigable.

Each slice body:

```markdown
Slice k of N. Merge vehicle: #<merge-vehicle-number>

**This slice** — <one line: what it is responsible for>
**Deliberately not here** — <one line: what a later slice covers>

Review this PR. Do not merge it — fixes land on #<merge-vehicle-number>, and
this slice is closed unmerged once that PR lands.
```

Merge vehicle body:

```markdown
The whole change. Review happens on the slices; fixes land here.

- #<n1> — slice 1: <subject>
- #<n2> — slice 2: <subject>
- … through N

Over the 800-line cap by construction — the cap binds review units, and these
contents are reviewed as the slices above. See docs/PR_WORKFLOW.md.

Slices are closed unmerged when this merges.
```

Number the slices only after the merge vehicle exists, so its number can be
cited. Either create it first and edit slice bodies with `gh pr edit`, or
create slices with placeholder bodies and fill them in after.

### 9. During review

- Review comments are answered on the **merge vehicle only**.
- Never amend, rebase, or force-push a slice. A rebased slice discards the
  review already given on it, and on everything stacked above it.
- Commit fixes to the merge vehicle branch and push. Slices stay frozen at the
  state they were read in.

### 10. Land it

Confirm with the user before this step — it merges to `main`, the branch every
release is tagged from.

```bash
gh pr merge <merge-vehicle-number> --squash
gh pr close <slice-1> <slice-2> … <slice-N>
```

Then delete the local slice branches (and the worktree, if one was used).

## Guard rails

- **Never merge a slice.** A slice on `main` is a partial change on the
  release base, and a tree where the spec, the tests, and the code can
  disagree.
- **Never force-push a slice** once its PR has a review comment on it.
- **Force-push the merge vehicle with `--force-with-lease` only**, and only
  after confirming the remote holds nothing you do not have.
- **The merge vehicle's gate is not optional.** `go test ./...`,
  `go test -race ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run` —
  all clean on its head commit. It contains review fixes no slice ever had, so
  green slices say nothing about it.
