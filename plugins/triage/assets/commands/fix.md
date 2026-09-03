---
name: "TAI: Fix"
description: "Work through accepted review comments and implement the fixes."
category: "Workflow"
tags: [tai, triage, fix]
version: 1
---
# /tai-triage:fix — implement the fixes for accepted comments

You are running as Claude inside Anthropic's Claude Code (or an
equivalent agent harness). This slash-command body is your
instructions for one invocation of `/tai-triage:fix`.

Your job: walk every `accepted` review comment in a scope and make the
change each one asks for. Fix is the step between `/tai-triage:triage`,
which decides what is worth doing, and `/tai-triage:verify`, which
confirms it was done.

**You MUST NOT change any comment's status.** Not to `completed`, not
to anything else. A comment stays `accepted` until `/tai-triage:verify`
finds evidence in the tree and the user confirms it. Marking your own
work complete is marking your own homework: it would record the fix as
verified on the strength of having attempted it, which is the one
thing the accepted → completed transition exists to prevent. The only
`tai` verbs you may call are the read-only ones — `tai triage status`,
`tai triage list`, `tai triage show`.

You MUST follow this contract. Anything not specified here is left to
your judgement, but never deviate from the obligations below.

## 1. Invocation forms and scope resolution

Each invocation maps to one of exactly three shapes:

| Invocation                                | Scope                                    |
|-------------------------------------------|------------------------------------------|
| `/tai-triage:fix`                         | auto-detect current scope                |
| `/tai-triage:fix --pr <number-or-url>`    | a single PR by number or full GitHub URL |
| `/tai-triage:fix --branch <name>`         | a branch-scoped review                   |

Any other argument shape is a usage error. Surface the three forms
above and stop — do not invoke `tai` at all.

There is deliberately no `stack` form, unlike `/tai-triage:triage` and
`/tai-triage:verify`. Those read; this one edits. Fixing every PR in a
stack in one invocation produces an uncommitted change set spanning
work that belongs in separate commits, and leaves the user to
untangle it. Run `/tai-triage:fix` once per PR instead.

For `--pr <number-or-url>`: the CLI's `--pr` flag only accepts a bare
integer. If the user passed a URL of the form
`https://github.com/<owner>/<name>/pull/<N>` (or `…/pull/<N>/files`,
`…/pull/<N>/commits`, etc.), YOU must extract the integer immediately
after `/pull/` — NOT the trailing path segment. Pass only `<N>` to
`tai`.

For `--branch <name>`: pass the branch name verbatim.

**Scope resolution.** Your first action is `tai triage status [--pr X |
--branch Y]` (with whatever the user supplied). You MUST NOT implement
your own scope detection. Handle the failure modes identically to
`/tai-triage:triage`:

- **`TRIAGE_NO_SCOPE`** (exit `2`): tell the user the current
  directory cannot be matched to any imported scope, offer
  `/tai-triage:import` or re-invocation with `--pr` / `--branch`, then
  STOP.
- **`TRIAGE_AMBIGUOUS_SCOPE`** (exit `2`): surface the disambiguation
  hint from stderr, ask the user to re-invoke with the explicit flag,
  then STOP.
- **No accepted comments in scope**: when
  `tai triage list --status accepted` (with the resolved scope flags)
  returns zero rows, announce `No accepted comments to fix in this
  scope.` and stop. If the counts from `tai triage status` show
  pending comments, add one line pointing at `/tai-triage:triage` —
  there is work, it just has not been triaged yet.

## 2. Read the queue

Pull every accepted comment in one call:

```sh
tai triage show --all --status accepted [--pr <N> | --branch <name>]
```

Work them in severity order (`critical`, `major`, `minor`, `nitpick`),
then file order — the same ordering `/tai-triage:triage` Phase 2 and
`/tai-triage:verify` use, so the three commands present the same work
in the same sequence.

Each comment's markdown carries `Suggested fix` and, when the user
refined it during triage, a `Resolution` section. **The `Resolution`
wins.** It is what the user actually agreed to, recorded at the moment
they agreed; `Suggested fix` is the reviewer's opening proposal.

## 3. Fix each comment

For each comment in order:

1. **Read the code first.** Open the file at the flagged lines. The
   line numbers came from import and may have moved; locate the
   construct the comment describes rather than trusting the offset.
2. **Check it is not already fixed.** If the tree already satisfies
   the comment, do not edit. Say so, name the evidence, and move on —
   leave the status alone, `/tai-triage:verify` will find the same
   evidence and ask the user to confirm it.
3. **Make the change.** Follow the repository's own conventions over
   the reviewer's phrasing when the two differ in style; the comment
   is about the defect, not the house style.
4. **Keep it to the comment's scope.** Fix what the comment describes.
   If you spot an adjacent problem, mention it and leave it — an
   unrequested change buried in a fix commit is the hardest kind to
   review.
5. **Say what you changed**, one line per comment, naming the id and
   the files touched.

When a comment cannot be fixed — the code has moved on, the request
conflicts with something else in the tree, the fix needs a decision
only the user can make — do NOT guess. Leave the code alone, keep the
comment accepted, and add it to the blocked list for the recap.

**Run whatever check the repository makes obvious** (its test command,
type-check, or linter) once you have finished editing, and report the
result. A fix that breaks the build is worse than no fix. If no check
is evident, say so rather than inventing one.

## 4. Recap

End with:

```
Fixed <N> of <M> accepted comments for <repo> <scope-label>.

  Fixed:        <N>
  Already done: <A>
  Blocked:      <B>

Blocked:
  [crit] 7: <title> — <one-line reason>

Run `/tai-triage:verify` to confirm these and mark them completed.
```

Omit the `Blocked:` list entirely when nothing is blocked. Omit the
`Already done:` count when it is zero. Always emit the closing
`/tai-triage:verify` line — it is the next step, and AI consumers need
a stable anchor to pick up.

## 5. Obligations

- Do NOT call `tai triage complete`, `tai triage accept`,
  `tai triage dismiss`, or `tai triage forget`. This command does not
  change state. `/tai-triage:verify` owns the accepted → completed
  transition.
- Do NOT commit, stage, push, or create a PR. The user reviews the
  working tree and decides how it lands.
- Do NOT fix comments that are `pending`, `dismissed`, or `completed`.
  Pending has not been decided yet — say so and point at
  `/tai-triage:triage`.
- Do NOT skip a comment silently. Every accepted comment ends the run
  in exactly one of fixed, already done, or blocked.
- Do NOT reorder the queue based on which fixes look easiest. Severity
  order, then file order.
- Do NOT expand a fix beyond what its comment describes.
