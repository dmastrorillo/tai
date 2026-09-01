---
name: "TAI: Verify"
description: "Check whether accepted comments have been addressed; mark completed."
category: "Workflow"
tags: [tai, triage, verify]
version: 1
---
# /tai-triage:verify — check that accepted comments are actually fixed

You are running as Claude inside Anthropic's Claude Code (or an
equivalent agent harness). This slash-command body is your
instructions for one invocation of `/tai-triage:verify`.

Your job: walk every `accepted` review comment in a scope, gather
concrete evidence the fix is in place, surface that evidence to the
user, and — only on explicit confirmation — mark the comment
`completed` via `tai complete`. Verify is the post-fix counterpart to
`/tai-triage:triage`'s investigation phase: there it auto-completed before
asking; here it MUST always ask.

You MUST follow this contract. Anything not specified here is left to
your judgement, but never deviate from the obligations below.

## 1. Invocation forms and scope resolution

Each invocation maps to one of exactly four shapes:

| Invocation                            | Scope                                          |
|---------------------------------------|------------------------------------------------|
| `/tai-triage:verify`                         | auto-detect current scope                      |
| `/tai-triage:verify --pr <number-or-url>`    | a single PR by number or full GitHub URL       |
| `/tai-triage:verify --branch <name>`         | a branch-scoped review                         |
| `/tai-triage:verify stack`                   | every PR from trunk to current, ancestor-first |

Any other argument shape is a usage error. Surface the four forms
above and stop — do not invoke `tai` at all.

For `--pr <number-or-url>`: the CLI's `--pr` flag only accepts a bare
integer. If the user passed a URL of the form
`https://github.com/<owner>/<name>/pull/<N>` (or
`…/pull/<N>/files`, `…/pull/<N>/commits`, etc.), YOU must extract the
integer immediately after `/pull/` — NOT the trailing path segment,
which may be `files`, `commits`, or a comment anchor. Pass only `<N>`
to `tai`.

For `--branch <name>`: pass the branch name verbatim.

For `stack`: see section 9.

**Scope resolution.** Your first action is `tai status [--pr X |
--branch Y]` (with whatever the user supplied). You MUST NOT implement
your own scope detection. Handle the failure modes identically to
`/tai-triage:triage`:

- **`TRIAGE_NO_SCOPE`** (exit `2`): tell the user the current
  directory cannot be matched to any imported scope, offer
  `/tai-triage:import` or re-invocation with `--pr` / `--branch`, then STOP.
- **`TRIAGE_AMBIGUOUS_SCOPE`** (exit `2`): surface the
  disambiguation hint from stderr, ask the user to re-invoke with the
  explicit flag, then STOP.
- **No accepted comments in scope**: when `tai list --status accepted`
  (with the resolved scope flags) returns zero rows, announce
  `No accepted comments to verify in this scope.` and exit without
  entering the loop. The status counts from `tai status` are enough
  context — do NOT emit the section-8 recap.

When the scope resolves AND there is at least one accepted comment,
proceed to evidence gathering.

## 2. Evidence gathering — PR-scope targets

For PR scopes, the canonical "what changed in this PR" answer is the
PR's unified diff. Fetch it once at the start of the loop:

```sh
gh pr diff <N> --patch
```

(Or `gh api repos/<owner>/<name>/pulls/<N>` for the raw patch when
`gh pr diff` is unavailable.) Parse the unified diff into per-file
blocks of added (`+`) and removed (`-`) lines. Keep the parse result
in memory — every accepted comment will be checked against it.

Pull the accepted comments in scope:

```sh
tai show --all --status accepted [--pr <N>]
```

For each accepted comment, look in the PR diff at the comment's
`file:lines` position for:

1. **Removal of the original code** — the snippet flagged in the
   comment's `description` (or implied by `lines`) appearing in a
   removed (`-`) line block at the comment's file.
2. **Addition of the suggested-fix pattern** — the pattern derived
   from the comment's `suggested_fix` appearing in an added (`+`)
   line block at the comment's file.

Then cross-reference the **current working tree** at the same
file/lines. Working-tree evidence catches fixes that are committed
locally but not yet pushed to the PR branch. When the PR diff and the
working tree agree, confidence is maximal.

## 3. Evidence gathering — branch-scope targets

For branch scopes, there is no PR diff to consult — the canonical
source is the working tree. Evidence sources, in order:

1. Read the file at `comments.file` at the comment's `lines` position
   in the current working tree. Check whether the original code is
   still present and whether the suggested-fix pattern appears in the
   file.
2. Run `git log --oneline -10 -- <file>` and check whether any recent
   commit subject contains keywords from the comment's `title`.

You MUST NOT invoke `gh pr diff` for a branch-scope target — there is
no PR to diff against.

## 4. gh-failure fallback

For a PR scope, `gh pr diff` MAY fail (network outage, `gh`
unauthenticated, the PR was deleted, etc.). When it fails:

1. **Warn the user** in plain language:

   > `gh pr diff <N>` failed: `<one-line cause from stderr>`. Continuing
   > with working-tree evidence only — consider `gh auth login` for
   > stronger evidence next time.

2. **Fall back** to the branch-scope flow (section 3) for the rest of
   the run.

3. **Cap confidence at MEDIUM** for the entire run. Even when a
   comment has both working-tree signals (original code absent AND
   suggested-fix pattern present), the absence of PR-diff
   corroboration means you MUST label the evidence MEDIUM rather than
   HIGH. State the cap in the evidence summary block so the user
   understands why a seemingly strong match is downgraded.

You MUST NOT silently proceed at full confidence after a `gh` failure.
The warning AND the cap are both required.

## 5. Three-tier confidence labelling

Every evidence summary carries one of three labels — `HIGH`, `MEDIUM`,
or `NONE`. The criteria are exact:

- **HIGH** — qualifies under EITHER of these branches:
  - **File-gone branch.** The file referenced by `comments.file` no
    longer exists in the working tree (the original code and the
    suggested-fix pattern collapse into a single observation: nothing
    to verify). HIGH for any scope.
  - **Both-signals branch.** At the comment's file/lines, the original
    code is absent AND the suggested-fix pattern is present in the
    working tree. AND:
    - For a **PR-scope target with `gh pr diff` succeeding**, the PR
      diff MUST also corroborate the change — an added (`+`) block
      with the fix at the relevant file, or the file removed/renamed
      entirely. Without diff corroboration, downgrade to MEDIUM.
    - For a **branch-scope target**, the working-tree signals alone
      are sufficient.
    - Under the **gh-failure fallback** (PR scope, `gh` unavailable;
      see section 4), the working-tree signals alone never qualify
      for HIGH — downgrade to MEDIUM. This applies regardless of how
      conclusive the working-tree match looks.
- **MEDIUM** — qualifies under ANY ONE of these signals (and the
  conditions for HIGH are not met):
  - Original code is absent but no clear suggested-fix pattern at
    the comment's lines.
  - Suggested-fix pattern is present but the original code is also
    still nearby (the fix may be incomplete).
  - `git log --oneline -10 -- <file>` shows a recent commit whose
    subject contains keywords from the comment's `title`.
  - The both-signals branch above is satisfied in the working tree
    but the PR diff does not corroborate (PR scope) OR the
    gh-failure fallback is in effect.
- **NONE** — no working-tree signal matches and no relevant git
  history.

Surface the label explicitly in the evidence block. Example:

```
Comment 5 (critical) — Replace execSync with execFileSync (src/api/auth.ts:15-29)

Evidence (HIGH confidence):
  • `execSync` no longer appears at lines 15-29
  • `execFileSync` appears at lines 15-29 with matching argument shape
  • PR diff shows `-execSync(...)` removed and `+execFileSync(...)` added

Mark as completed? [Y/n]
```

## 6. Confirmation prompts

For each accepted comment, the prompt and its default depend on the
confidence label:

- **HIGH** → emit `Mark as completed? [Y/n]`. The default (on bare
  Enter) is **YES**.
- **MEDIUM** → emit `Mark as completed? [y/N]`. The default is **NO**.
- **NONE** → do NOT prompt. Announce
  `No evidence found; comment remains accepted.` and move to the next
  comment.

Parse the user's reply:

- `y` / `yes` / `mark it` / Enter at HIGH → call
  `tai complete <id> --resolution "<one-line evidence summary>"`
  (pass the same `--pr <N>` / `--branch <name>` scope flags that
  section 1 resolved, so the comment is looked up under the right
  scope). The resolution should describe what evidence was found, not
  just "verified".
- `n` / `no` / `not yet` / Enter at MEDIUM → leave as `accepted`.
  Move on.
- `show me again` / `re-show` / `show it again` → see section 7.

You MUST NOT call `tai complete` on a NONE-confidence comment without
the user explicitly typing the completion. The conservative bias is:
weak evidence stays in the queue.

## 7. Show-again loopback

When the user replies "show me again" (or equivalent) to a verify
prompt:

1. Run `tai show <id>` (passing the same scope flags that section 1
   resolved) and surface the full comment markdown verbatim.
2. Re-emit the SAME evidence summary block from before — same label,
   same bullets. You MUST NOT re-gather evidence on a show-again loop
   — do NOT re-fetch the PR diff, do NOT re-read working-tree files,
   do NOT re-run `git log`. The original gather (from before the
   user's first prompt for this comment) is the canonical input for
   the entire iteration. Re-gathering would shift the goalposts and
   make the loop non-deterministic.
3. Re-prompt with the same default as the original prompt
   (HIGH → `[Y/n]`, MEDIUM → `[y/N]`).

Each accepted comment may be re-shown at most **three times**. On the
fourth show-again request for the same comment, do NOT re-show — ask
the user whether they want to defer (leave as `accepted`) or
explicitly mark completed, then move to the next comment regardless
of their answer.

## 8. Recap

When every accepted comment has been processed, emit the recap
exactly once. Template:

```
Verification complete for <repo> <scope-label>.

  Marked completed: <N>
  Left accepted:    <M>

Still-accepted work queue (severity order):
  [crit] 7: <title> (<file>:<lines>)
  [crit] 5: <title> (<file>:<lines>)
  [maj]  3: <title> (<file>:<lines>)
  …

Ready to prune the completed comments? Run `tai forget <scope-flag> --status completed --yes`.
```

- `<scope-label>` is `PR #142` for PR scope, `branch feat/x` for
  branch scope.
- `<M>` is the residual queue size — the number of still-accepted
  comments AFTER this run.
- Source the still-accepted rows from `tai list --status accepted
  <scope-flag>` (the same surface used at the loop start, re-run
  after the loop finishes). Each row's `<id>` is the integer
  position number `tai list` prints in its `ID` column — do NOT use
  `B<key>.<member>` notation; that is not a `tai list` output
  format. (The `BATCH` column shows the batch key on its own line
  for members of a batch; the comment's primary identifier remains
  the integer position.)
- The severity-abbreviation prefix is one of `crit | maj | min | nit`.
- `<scope-flag>` MUST be `--pr <N>` for a PR-scope run or
  `--branch <name>` for a branch-scope run. `tai forget` rejects
  invocations without a scope selector — `--status` alone is a
  filter, not a selector.
- The final line MUST contain the scope-qualified
  `tai forget <scope-flag> --status completed --yes` so AI consumers
  can pick it up directly. Emit this line even when zero comments
  were marked completed in this run (the user may have completed
  some in a prior session); the suggestion is always safe.

The recap is emitted EXACTLY ONCE per loop completion. Do not repeat
it if the user asks a follow-up question.

## 9. Stack mode

`/tai-triage:verify stack` runs the three-phase loop once per PR in the
current staccato stack (or `gh`-derived stack), ancestor-first.

1. **Enumerate the stack.** Preferred backend:
   `st_reviews(scope='to-current')` — returns the ordered list of PRs
   from trunk to the current branch. Fallback:
   `gh pr list --state open --json number,headRefName,baseRefName,title`
   filtered to the local stack's PR numbers.
2. **If neither backend is available**, announce the missing
   dependency (`stack mode needs either the staccato MCP or the gh
   CLI on PATH`) and STOP without entering the loop.
3. **For each PR in ancestor-first order:**
   - Announce: `Now verifying PR #<N> (<index> of <total>): <title>.`
   - Run the full three-phase loop for that PR (sections 2–7 and 8's
     recap).
   - After the per-PR recap, pause: `Continue to the next PR? [y/n]`.
     A `no` exits stack mode with a partial stack-level summary
     covering only the PRs already verified.
4. **At the end** (after the last PR's per-PR recap), emit a
   stack-level aggregate:

   ```
   Stack verification complete (<total> PRs).

     Marked completed: <sum-N>
     Left accepted:    <sum-M>

   Per-PR breakdown:
     PR #<N1>: <C1> completed, <A1> still accepted
     PR #<N2>: <C2> completed, <A2> still accepted
     …

   Ready to prune across the stack? Run, per PR:
     tai forget --pr <N1> --status completed --yes
     tai forget --pr <N2> --status completed --yes
     …
   ```

   `tai forget` requires a primary scope selector, so the
   stack-level aggregate lists one `--pr <N>` invocation per PR
   rather than a single multi-PR command.

Per-PR failures (`TRIAGE_NO_SCOPE`, `TRIAGE_AMBIGUOUS_SCOPE`,
`gh pr diff` failure for that PR) do NOT bail the whole stack —
surface the failure for the affected PR, ask the user how to handle
it, then continue to the next PR.

## 10. Guardrails — things you MUST NOT do

- Do NOT bypass the CLI by writing to the SQLite database directly.
  Every state change goes through `tai complete`. There is no other
  state-mutating call in this command.
- Do NOT auto-complete a comment without user confirmation, even on
  HIGH-confidence evidence. The cost of a wrong auto-completion
  (silently removing an accepted comment that wasn't actually fixed)
  is asymmetrically worse than the cost of one extra `[Y/n]`
  keystroke.
- Do NOT prompt for completion on NONE-confidence comments. The
  conservative bias is: weak evidence stays in the queue, the user
  handles it via the normal fix-and-mark path.
- Do NOT proceed at HIGH confidence after `gh pr diff` failed for a
  PR-scope target. Cap at MEDIUM and tell the user why.
- Do NOT re-gather evidence on a "show me again" loop. The original
  gather is the canonical input for that iteration.
- Do NOT change the loop order based on user drift. Process the
  accepted comments in severity order, then file order — same
  ordering rules `/tai-triage:triage` Phase 2 uses.
- Do NOT emit the recap (section 8) more than once per loop, and do
  NOT emit it when the loop was bypassed due to an empty accepted
  queue (use the one-line announcement from section 1 instead).
- Do NOT suppress the
  `tai forget <scope-flag> --status completed --yes` line from the
  recap even when zero comments were marked completed in this run.
  The suggestion is always safe (the user may have completed some in
  a prior session) and AI consumers need a stable anchor to pick up.
- Do NOT emit an unscoped `tai forget --status completed --yes`. The
  CLI rejects invocations without a primary selector
  (`--pr` / `--branch` / `--repo` / `--comment` / `--batch`); always
  interpolate the resolved scope's flag.
- Do NOT verify dismissed comments. They're already out of the
  queue.
