---
name: apply-review
description: >
  Applies a review already left on a pull request or a stack. Pulls every
  review thread, works out the principle behind each comment, hunts the same
  violation at sites the reviewer missed, fixes on the merge vehicle, replies
  to each thread with the commit that fixed it, resolves it, and distils
  recurring principles into `.review/`. Use when a review needs actioning
  rather than conducting: "apply the review", "action my comments", "apply the
  comments", "fix the review comments", "I've finished reviewing", "resolve
  the threads", "the stack has comments", or whenever a pull request carries
  unresolved review threads waiting on a fix.
---

# Apply Review

This applies a review a person has already written. It does not produce one. An
ask to review code wants a review command instead; this skill starts from
findings that already exist.

The work is interpretive before it is mechanical. A comment is **evidence of a
principle**, and the principle almost always applies in more places than the
comment does. A run that fixes exactly the commented lines and nothing else has
done the typing and skipped the thinking.

Policy this skill assumes, and does not restate: `docs/PR_WORKFLOW.md` for stack
topology and why fixes land on the merge vehicle, `CLAUDE.md` for the
spec-first pipeline, commit scopes, and gates.

Two references, read when the phase needs them rather than up front:

- [references/github-api.md](references/github-api.md) for the queries and
  mutations, in phases 1 and 5.
- [references/formats.md](references/formats.md) for the frozen text of every
  thread reply, the Fix Audit table, and the `.review/` files, in phases 5 to 7.
  These are fixed so the audit reads identically every run, so copy them rather
  than composing something equivalent.

## Hard stops

Check all three before touching anything. Each one means the review is not ready
to apply, and applying it anyway destroys information that cannot be recovered.

1. **Wrong branch.** Fixes land on the merge vehicle, so its head branch has to
   be checked out. If it is not, stop and name the branch (and check
   `git worktree list` in case it is already checked out elsewhere). Do not
   switch branches to fix this without saying so.
2. **A pending review.** Comments reporting `state: PENDING` belong to an
   unsubmitted review. They are half-written and invisible to everyone but their
   author, and they cannot be replied to. Stop and ask for the review to be
   submitted.
3. **Slices nobody reviewed.** Zero threads on a pull request is ambiguous: it
   means clean or it means unread. Zero _submitted reviews_ is not ambiguous.
   List those pull requests and ask whether the review is finished. Skip this and
   an unread slice gets silently treated as clean, which is the one failure the
   resolved-thread audit exists to rule out.

## Three modes

| Mode                    | Recognised by                                 | Comments live       | Fixes land           | Fix Audit |
| ----------------------- | --------------------------------------------- | ------------------- | -------------------- | --------- |
| **Stack, first pass**   | slices carry unresolved threads               | on the slices       | on the merge vehicle | yes       |
| **Vehicle**             | slices are clear, the vehicle carries threads | on the vehicle      | on the vehicle       | no        |
| **Single pull request** | there is no stack                             | on the pull request | on its own branch    | no        |

Vehicle mode and single-pull-request mode are the same procedure. The Fix Audit
exists only to bridge a comment on one pull request to a commit on another, so
when they are the same pull request GitHub links them already and a table would
restate the timeline.

In vehicle mode, do not refetch the slices. Their threads are resolved, and the
commit under discussion carries its own context plus a link to more.

## Phase 0. Resolve the stack

Input is a pull-request number or URL, or nothing, meaning the current branch's
pull request.

`stacked-pr` writes the map into the pull-request bodies, so read it rather than
inferring it:

- A slice body says `Merge vehicle: #N`. That is the vehicle.
- A vehicle body lists its slices with numbers.
- If a body is missing its map, walk `baseRefName` instead: slices chain
  head-to-base, and the vehicle is the one based on `main` whose head no other
  pull request bases on.

**Single pull request** requires all three of: no `Merge vehicle:` line in the
body, no slice list in the body, and no open pull request basing on this head.
Any one alone is fooled by a hand-written body.

Print the map, with each slice's thread count, and get it confirmed. A wrong map
means missed comments, and missed comments are the one outcome the whole
procedure exists to prevent.

## Phase 1. Pull every comment, in slice order, before fixing anything

Pull the whole stack first. Never read slice 1, fix slice 1, then read slice 2.

The reason is not efficiency, it is comprehension. **A principle's fullest
statement is at its first occurrence.** A reviewer writes it out in full the
first time and abbreviates it thereafter, so by the eighth slice the comment is
one word that only parses against the fourth. Reading in ascending slice order
puts the definition before the shorthand.

The shape this takes in practice:

| Slice | Comment                                 |
| ----- | --------------------------------------- |
| 4     | "Should be a helper"                    |
| 5, 6  | "Should be a helper", fourteen times    |
| 7     | "Helper", "Helpers"                     |
| 8     | "Helper", "helper"                      |

One principle, twenty-one threads, and only the first is a sentence.

Collect per thread: node id, permalink, pull-request number, path, line, diff
hunk, body, author, and resolved state. Collect top-level and review-body
comments separately, because GitHub gives them no resolved state.

Already-resolved threads are read for context and never re-actioned. Their
principles still govern phases 3 and 7.

Then **anchor every thread against the vehicle head**, not against the slice it
sits on. In a stack, slice 4's diff shows lines that slice 8 rewrites, so a
comment can point at an intermediate state that no longer exists anywhere. Four
outcomes:

- **present**, fix it on the vehicle
- **moved**, the code exists at a different path or shape; fix the vehicle's
  version and say so in the reply, because the reviewer's quoted snippet will not
  match what changed
- **already addressed**, a later slice fixed it; no commit
- **gone**, a later slice deleted it; no commit

`isOutdated` cannot help here. It only fires when a line moves on the pull
request's own head, and fixes never land there, so slice threads stay `false`
forever no matter what happens.

**Work out whether the thread's scope is the line or the file.** A review thread
always anchors to a line, because GitHub has nowhere else to put it, but plenty of
comments are about the shape of the whole file: what order things appear in, what
is duplicated across it, what should be hoisted and reused. A comment like "every
one of these verbs hand-rolls the same validation — declare it once" is anchored
at whatever line the reviewer happened to be looking at, and resolving that line
is not the fix.

The tell is that the comment describes a property of the file rather than of the
code at the anchor. When it does, read the file in full before proposing
anything, and treat the anchor as a pointer to the file rather than to a defect.

## Phase 2. Group, interpret, dispute, classify

**Group by principle before reading anything one at a time.** Twenty-one of
twenty-six threads being one principle is normal, not exceptional. Per-comment
ceremony on a grouped set wastes the run and produces twenty-one interpretations
of one idea.

**Resolve shorthand backwards.** Before asking what a terse comment means, search
the extracted set in ascending order for a fuller statement of the same term. If
"Should be a helper" appears in slice 4, "Helper" in slice 8 needs no
interpretation at all.

**Interpret only when no fuller form exists.** Read the anchored code, state in
one sentence the principle you believe is being invoked, and get a yes before
touching anything. One interpretation covers the whole group. Getting this wrong
means applying the wrong fix at every site in the group, so it is cheaper to ask
than to guess.

**Some comments give a direction rather than name a defect.** "I like how the
help bullets name the exact command to run. Where else can we do that?"
identifies no broken line. It asks how this diff can be better, along a pattern
the diff already established.

That is not a question to answer in prose and it is not new scope to defer.
Answering it means doing the survey: read the file, work out what else falls
under the same pattern, and come back with the concrete site list. Then it goes
through phase 3 like any other generalisation, gets picked, and ends up
classified `fixed`.

The mistake to avoid is treating "extend this pattern" as a new category. It is
not. Phase 3 already does exactly this for every principle the review raises: it
finds the sites the reviewer did not mark. A direction-shaped comment is the same
work with the site list undiscovered rather than partly given.

**Approval is signal too.** A comment saying something was done well establishes
a principle just as firmly as one saying something was done badly, and more
usefully, because it names the pattern rather than one instance of its absence.
Praise still gets a reply and a resolve, or the audit shows an open thread, and it
still feeds phase 7.

**Dispute when the comment is wrong.** Ground it in this repository's own
documents or in the code, never in preference. A worked example of the right
shape: a comment saying "no package-level mutable state" on a `version.String`
declaration is answerable from `CLAUDE.md`'s Conventions section, which
explicitly exempts linker-injectable build-metadata variables and spells out the
constraints they live under (dedicated package, documented at the declaration,
never mutated at runtime). That is a dispute worth having. "I would have written
it differently" is not.

State the case, hear the answer, state it once more if it still looks wrong, then
fold, unless told to keep going. When folding, the reply **carries the argument**:
the position argued, the position held, and the conclusion. The paper trail is
how the reviewer finds out later that they were the one who was wrong, so
dropping it to keep the reply tidy removes its only reason for existing.

Classify every thread into exactly one outcome:

| Outcome                      | Meaning                                                                        |
| ---------------------------- | ------------------------------------------------------------------------------ |
| **fixed**                    | code changed                                                                   |
| **fixed after folding**      | code changed, and the reply records the disagreement                           |
| **answered**                 | a question or a pointer with no defect behind it                               |
| **withdrawn**                | disputed, and the reviewer agreed no change is needed                          |
| **deferred, spec-grade**     | behaviour or architecture changes; an OpenSpec change is written and committed |
| **deferred, ticket-grade**   | no spec content, a follow-up tidy; a GitHub issue is raised                    |
| **already addressed / gone** | a later slice handled or removed it                                            |

Resolved means **the comment has been addressed**, not that the problem is gone.
That distinction is the point: every one of these outcomes resolves, so a closed
stack where every thread is resolved proves nothing was forgotten, which is a
different and more useful claim than proving everything was fixed.

A spec-grade deferral follows the pipeline: the OpenSpec change carries the
behaviour decision, and the BDD case and TC-ID land with the implementation
later, per `CLAUDE.md`.

## Phase 3. Hunt the sites the reviewer missed

A reviewer marking twenty-one instances of a principle has almost certainly not
found all of them. This phase is where most of the value is.

Search radius, in order:

1. The vehicle diff.
2. Files the vehicle touches, **including lines outside the diff**. Changing a
   file is the cheapest opportunity to correct what was already wrong in it.
3. Nothing else. No repository-wide sweep.

Category cap: only principles raised in **this** review. Never a category
invented while looking. The cap is what keeps this from becoming an unbounded
refactor riding a pull request nobody reads.

Note what the cap does **not** exclude. A principle the reviewer raises as a
direction ("where else can we do that?") is raised in this review, so extending
it is in scope here, not new scope to be deferred. The cap is about categories
nobody asked for, not about how much of an asked-for category gets applied.

**The routing test: does the answer stay inside the files the vehicle already
changes?**

- **Yes**, it belongs in this phase. Survey, propose, get it picked, fix it.
- **No**, it needs new files, a new package, or trees this change does not
  touch. Propose a change and defer it.

Two things make the answer "yes" more often than instinct suggests. Reshaping a
file this change **introduces** is in-diff by definition, since there is no
previous version to be compatible with. And deferring a structural change to a
new file pays for the migration twice: ship duplicated logic now and restructure
it later, and every caller moves twice instead of once. Duplication and reading
order inside a file the change introduces are therefore in-diff by default.

Present findings as one numbered list, grouped by principle, with a file and line
each. The reviewer picks. Then, per approved finding:

- **Its own commit**, never batched with a comment fix. The vehicle's delta has
  to stay separable, because reviewing that delta is how these get read at all.
- The commit body names the thread whose principle it generalises, so
  traceability survives even though no comment asked for it.

This matters more than it looks. `docs/PR_WORKFLOW.md` says what gets reviewed on
the merge vehicle is the delta from the slices, and that those are small and each
traceable to a comment. An uncommented fix breaks that guarantee, and the vehicle
merges to `main` — the branch every release is tagged from. Its own commit plus a
named origin is what puts the guarantee back.

## Phase 4. Fix and commit

Everything lands on the vehicle branch. Never amend a slice: rebasing it discards
the review already given on every descendant, so one comment costs a re-read of
the rest of the stack, per comment.

Fixes follow the repository's own rules: a behaviour change gets its failing test
first (the TDD charter in `CLAUDE.md` is not suspended because the change was
requested in a review), and a behaviour-facing change updates the affected
`test-cases.md`. Run the gates locally — `go test ./...`, `go vet ./...`,
`gofmt -l .` — fix what goes red, and only then push. The vehicle going red
**after** a push never unresolves anything, but pushing known-red makes the
reviewer's commit-by-commit read harder for no gain.

Commit boundary is **one principle in one scope**:

- One principle, because the reviewer reads the vehicle commit by commit and a
  commit mixing concerns cannot be judged.
- One scope, because commitlint requires a scope from the enum in
  `.commitlintrc` (today: `core`, `pkg`, `triage`, `openspec`, `ci`). A commit
  spanning trees has no honest scope. Split it instead.

A thread should never need two hashes. The single exception is one comment whose
fix genuinely spans trees: that is one commit per scope, and the reply names
both.

**A restructure is never one commit.** "Restructure the verb wiring" cannot be
judged as a unit, so it is not one. Hoisting duplicated logic to a single
declaration, reordering into the sequence a reader needs, and renaming a shared
term are three separate changes with three separate ways of being wrong. Split
along those lines, and the reply on the thread that asked for it names each
commit. This is the one case where a thread legitimately carries several hashes,
and it is not an exception to the rule above so much as the rule applied: each
commit still carries one principle.

## Phase 5. Reply, then resolve

In that order, always. Read `isResolved` back from the mutation and never report
a thread resolved on an unverified response. A thread resolved without its reply
landing is a thread that claims to be handled and says nothing about how.

Top-level and review-body comments get a reply and no resolve, because GitHub
gives them no resolved state. Say so rather than leaving it looking skipped.

## Phase 6. Fix Audit

First pass of a stack only. One comment on the merge vehicle, titled exactly
`# Fix Audit` so it stays findable, edited in place if it already exists.

It exists because the comment is on one pull request and the commit is on
another, and nothing in GitHub connects those. Later passes skip it entirely.

## Phase 7. Distil into `.review/`

`.review/` records **review principles**: matters of craft that come up when
reading code. Whether an error deserves help bullets, whether a helper belongs
in the cmd layer or the engine package, how a thing should be named, when a
table-driven test earns its keep.

It is not for architecture or rules. Which tree owns a helper, what the plugin
wire contract promises, what the spec-first pipeline requires: those are
documented decisions and they belong in `CLAUDE.md`, `docs/*`, or an OpenSpec
change. The line is whether the comment is about _how this code is written_ or
about _how the system is arranged_.

**Entry test, all four:**

1. Evidenced by a real review comment, **approving or critical**. A comment
   praising a pattern establishes it as firmly as a comment objecting to its
   absence, and names it more clearly, since it points at the pattern working
   rather than at one place it is missing.
2. Not already stated in `CLAUDE.md`, `docs/PR_WORKFLOW.md`, or the relevant
   `test-cases.md`. If it is, the finding is that a documented rule was broken.
   Say that in the thread and write nothing.
3. Not caught by gofmt, go vet, or golangci-lint. Formatting is gofmt's, and
   whatever the linter rejects is the linter's; a prose entry duplicating either
   is a rule nothing enforces. If the principle is mechanically checkable,
   propose enabling or configuring a golangci-lint linter in `.golangci.yml`
   instead — a better outcome than a paragraph.

   This excludes what those tools catch, not the subjects they cover. A
   preference about how exported symbols are documented is an entry, because
   nothing rejects the form it argues against. A comment reporting something
   `go vet` already flags is not.

4. Craft, not architecture, per above.

**Two asks, never one and never none.** Both are the reviewer's call, and either
can come back a no.

1. **Before writing anything, ask where it goes.** Name the destination that
   looks right — a `.review/` file, `CLAUDE.md`, a `docs/*` file, or a
   golangci-lint linter — and say why. Ask this even where `.review/` looks
   obvious, and ask it on the first principle of a run as readily as the fifth:
   the answer is often that it belongs directly in the repository's
   documentation and never passes through `.review/` at all.
2. **Before promoting, ask where to.** When adding to or amending an existing
   entry looks like grounds for promotion, propose the destination and the
   reason, and wait for an answer. Meeting the three criteria below is what makes
   promotion arguable, not what authorises it.

**Promotion out of `.review/` needs all three:**

1. Statable in one sentence that names no file and no symbol.
2. At least two distinct comments behind it. One instance is a fix, not a
   principle.
3. True everywhere its destination governs. This picks the destination rather
   than gating the way to one: a principle that holds across the repo promotes
   to `CLAUDE.md` or a `docs/*` file, and a principle that holds for one tree
   promotes into that tree's documentation. Holding narrowly is a reason to
   promote somewhere narrower, never a reason to stay.

On promotion the entry is **deleted** from `.review/`, and no pointer is left
behind. Two homes for one rule is how they drift.

A principle can also arrive from a comment in a later stack. Appending evidence
to an existing entry is normal, and repetition is what marks the heavy ones.

`.review/` is committed on its own commit, scoped to the tree the evidence came
from (`core`, `pkg`, or `triage`), and gets its own row in the Fix Audit.

**Creating it the first time** also adds one line to `CLAUDE.md` pointing at
`.review/index.md`. Without that the principles are written somewhere nothing
reads before code is written, so the same comments get written again next
review. One line, and the index stays a hook list precisely so that line is
cheap.
