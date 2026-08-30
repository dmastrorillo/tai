# Pull Request Workflow

**A pull request is a unit of review, not a unit of work.** Its size is bounded
by what a person can actually hold in their head and judge, which is a much
smaller number than what one person can write in a day.

Architectural boundaries and the spec-first pipeline → [`CLAUDE.md`](../CLAUDE.md).
Release mechanics → [`RELEASE.md`](../RELEASE.md). This file governs one thing
neither of those does: how a change is cut up so it can be reviewed at all.

**The procedure is a skill, not prose.** This document states the policy and
the reasoning. To actually plan a split or raise the PRs, invoke the
`stacked-pr` skill — it carries the commands, the branch topology, and the
order of operations. To action a review someone has left on a stack — fix,
reply, resolve, audit — invoke the `apply-review` skill.

## The 800-line cap

**No pull request opened for review may exceed 800 countable lines of diff.**

Not a guideline. A change over the cap is split before it is raised.

### Why a number, and why this one

Review quality falls off a cliff long before a diff gets large, and the failure
is silent: an 800-line PR gets read, a 4000-line PR gets approved. Nobody
announces the switch from the first behaviour to the second, so a repository
where large PRs are normal is a repository with no review at all, and it looks
exactly like one that does.

The cap is a forcing function, not a measurement. Its real work happens before
any code is written — it makes the author decide up front where the seams are,
which is the same decision as deciding what the change actually is.

800 is chosen to be roughly one sitting's worth of careful reading. Every
number in this range is arbitrary; the value of picking one is that it stops
being negotiated per PR.

### What counts

Additions plus deletions, with renames detected, excluding generated paths:

```bash
git diff -M --numstat origin/main...HEAD -- . \
  ':(exclude)go.sum' \
  | awk '{a+=$1; d+=$2} END {printf "additions=%d deletions=%d total=%d\n", a, d, a+d}'
```

- **Additions plus deletions**, because a rewrite is as much to read as a
  greenfield file, and additions-only would make "delete 700 lines, add 700
  lines" register as a 700-line change.
- **Renames detected** (`-M`), so hoisting a file between trees — a routine
  consequence of promoting shared code into `pkg/` — does not consume a whole
  slice.
- **`go.sum` excluded**, because it is generated transitive-hash bookkeeping
  with no semantic content; `go.mod` is the source of truth for dependencies
  and IS counted and reviewed.

A path is excluded because it is generated, never because it is inconvenient.
Adding to the list is a change to this document.

## Over the cap: stack it

A change above 800 countable lines is split into a **stack** — a chain of
branches, each based on the previous one, each raised as its own PR.

### Every slice must be reviewable in isolation

This is the constraint that decides where the cuts go, and it is stricter than
it sounds:

> A reviewer who reads **only this slice** must be able to judge whether it is
> correct.

They may know that later slices are coming. They may not need to read one to
form an opinion on this one. A slice that only makes sense as "part 3 of 5" has
been cut mechanically rather than logically, and the split has failed — it has
converted one un-reviewable PR into five.

The test to apply to a proposed seam:

> **Could this slice be merged on its own and leave the repository working?**

Not "will it be" — slices are never merged (see below). But a slice that would
break `main` if it landed alone is a slice that cannot be reasoned about alone
either. "Working" here means the full gate: `go test ./... && go vet ./... &&
gofmt -l .` clean, and no `test-cases.md` entry contradicting the code.

### Seam heuristics for this monorepo

Cut along the dependency direction, bottom up. In descending order of how often
these are the right seams here:

1. **`pkg/` framework packages** — errcode, cliout, exitcode, cliexec, datadir,
   taiplugin, clitest. Self-contained, on the stability contract, and both
   binary trees depend on them.
2. **Core engine packages** — `core/internal/{sync,plugins,config,repoinit,
   standards,workflow,sourcetree,...}`. Each is reviewable with its unit tests
   and no cmd wiring in sight.
3. **Core command wiring** — `core/internal/cmd` plus `core/cmd/tai`, with the
   e2e tests that drive the assembled command.
4. **Plugin trees, bottom-up** — `plugins/<name>/internal/storage` →
   `cmdframework` → domain packages → `internal/cmd` → `cmd/<binary>`.
5. **Spec and docs** — `test-cases.md` updates that retire or reword cases,
   `openspec/` archive moves, `docs/*`. (New TC entries travel with the tests
   that name them — see the forbidden seams.)

Three seams that are **never** valid here:

- **Tests separated from the code they test.** The TDD charter in `CLAUDE.md`
  puts the test and its subject in the same change. A "tests" slice and an
  "implementation" slice is a split that breaks the repository's testing
  contract to satisfy a line count.
- **A `test-cases.md` BDD entry separated from the test that names its TC-ID.**
  The spec entry, the TC-named test, and the behaviour it pins are one unit;
  splitting them leaves a slice where the spec and the suite contradict each
  other.
- **An OpenSpec archive separated from its implementation.** A completed
  proposal is archived in the same change that ships it, so no slice shows a
  "done" proposal whose code is elsewhere.

### Where a stack does not help

If a change is over the cap and has no seam that survives the isolation test,
the change is too broad, not the PR. Narrow the change. A stack cannot rescue a
single indivisible edit, and pretending otherwise produces five PRs that must
be read as one.

## The shadow PR

**A stack of N slices produces N+1 pull requests.**

The extra one — the **merge vehicle** — is branched from the tip of slice N, so
its tree is identical to slice N's. Its base is `main`, so its diff is the
entire change rather than a slice of it.

| PR      | Base      | Diff                | Role       |
| ------- | --------- | ------------------- | ---------- |
| 1       | `main`    | slice 1             | reviewed   |
| 2       | slice 1   | slice 2             | reviewed   |
| …       | …         | …                   | reviewed   |
| N       | slice N-1 | slice N             | reviewed   |
| **N+1** | `main`    | **the whole thing** | **merged** |

The protocol:

- **Slices 1..N are opened as drafts. The merge vehicle is opened ready for
  review.**
- **Slices 1..N are what gets reviewed.** They exist to be read.
- **Every fix and every change arising from review lands on N+1, and only on
  N+1.** Never amend a slice.
- **N+1 is the only PR that merges.** Slices 1..N are closed unmerged once it
  lands.

### Why fixes land only on the merge vehicle

Because fixing a slice mid-stack rebases every descendant of it, and a rebased
branch discards the review that was already given on it. Correcting slice 2
invalidates the reading of slices 3, 4 and 5 — so the cost of one review
comment is re-reviewing the rest of the stack, and it recurs per comment.

The merge vehicle absorbs fixes without disturbing anything. The slices stay
frozen at the state they were reviewed in, which is also what makes their
review comments still mean something a week later.

### The cap binds slices, not the merge vehicle

PR N+1 is roughly N×800 lines by construction. That is intended and is not a
violation.

The cap exists to bound **what a reviewer reads**, and by the time N+1 is
raised its contents have already been read as slices 1..N. The merge vehicle is
a delivery mechanism, not a review unit. Reviewing it directly would defeat the
entire arrangement — it is the un-reviewable PR the split existed to avoid.

What _is_ reviewed on N+1 is the delta from the slices: the fixes made in
response to review comments. Those are small by nature, and each is traceable
to the comment that asked for it.

### Never merge a slice

Slices are review artefacts. Merging one puts a partial change on `main`, and
`main` is the base every release is tagged from and the tree the spec-first
pipeline promises is coherent — a half-landed stack is a `main` where
`test-cases.md`, the tests, and the code can disagree, and where a `vX.Y.Z`
tag could ship a fraction of a change.

This is also why the slices are drafts: a draft cannot be merged by accident.

The merge vehicle is the inverse case and so is not a draft. It is the one PR
that is meant to merge; GitHub refuses to merge a draft or to enable auto-merge
on one, and a draft that has to be marked ready before it can land is a step
between a finished review and landing that protects nothing.

## CI and the merge gate

Only the merge vehicle's head commit reaches `main`, so it is the only commit
whose gate matters. The full pre-merge gate — `go test ./...`,
`go test -race ./...`, `go vet ./...`, `gofmt -l .` empty, `golangci-lint run`
clean — applies to it in full and is not relaxed because the slices were green.

A green slice is not evidence about the merge vehicle. The merge vehicle
contains review fixes the slices never had, so it is a commit no CI run has
ever seen until it runs on it.

## Cross-linking

A stack is unreadable without a map, so each PR body carries one.

- **Each slice** states its position (`slice 3 of 5`), links the merge vehicle,
  and says in one line what it is responsible for and what it deliberately
  leaves to a later slice.
- **The merge vehicle** lists every slice with its number, and states that
  review happens there and fixes land here.
- **Every slice says, in its body, that it will be closed rather than merged.**
  Otherwise a reader with permissions helpfully merges it.

## When a change fits

Under 800 countable lines: one PR, based on `main`, no stack and no merge
vehicle. The shadow-PR protocol is a consequence of splitting, not a general
rule — with nothing split there is nothing to shadow.

Draft status is then the author's call, on the usual grounds: draft while the
work is unfinished, ready when it is.

## Tooling

- **Stack management**: plain `git`. Branches, `rebase --onto`, and
  `push --force-with-lease`.
- **PR creation**: `gh`.
- **Worktrees**: optional; one per change under `.claude/worktrees/` when
  parallel work needs isolation.

Exact invocations, in order, live in the `stacked-pr` skill. They are not
duplicated here — a command in two places drifts, and this document is the one
that has to stay true.
