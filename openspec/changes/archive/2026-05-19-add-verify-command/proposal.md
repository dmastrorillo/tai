## Why

After triage, the user fixes the accepted comments. Then comes the question every triage tool eventually answers wrong: "which of the accepted comments did I actually address?". `/tai:triage` Phase 1 handles this for *pending* comments (catching cases the user fixed before triage even started), but there's no equivalent surface for *accepted* comments after the fix work is done.

`/tai:verify` closes that loop. It walks every accepted comment in a scope, looks for concrete evidence the fix is in place (in the working tree for branch-scoped reviews, in the PR diff for PR-scoped reviews), and asks the user to confirm before marking each one `completed`. The user can then run `tai forget --status completed` to prune the queue.

This is a slash command only. The CLI verbs it uses (`tai list --status accepted`, `tai show`, `tai complete`) already exist after `add-triage-state` lands. No new CLI verbs are required.

## What Changes

- Introduce the bundled `/tai:verify` Claude slash command, installed by `tai install` at `~/.claude/commands/tai/verify.md`. The command's body specifies a three-phase loop:
  1. **Scope resolution** — accept `/tai:verify`, `/tai:verify --pr <N>`, `/tai:verify --branch <name>`, `/tai:verify stack`. Calls `tai status` to verify scope is resolvable, same failure handling as `/tai:triage`.
  2. **Evidence gathering** — pull every accepted comment in scope via `tai show --all --status accepted`. For each comment, the slash command searches for evidence the fix has landed:
     - **PR-scoped target**: query the PR's diff via `gh pr diff <N>` and look for the suggested-fix pattern within the diff. Cross-reference against the working tree as a secondary check (for staged-but-unpushed fixes).
     - **Branch-scoped target**: walk the working tree at the comment's file/lines and the recent git history (`git log --oneline -10 -- <file>`). Look for the suggested-fix pattern in the current file and for relevant change subjects in history.
     - In either case, evidence sources are the same four heuristics as `/tai:triage` Phase 1 (file moved/missing, original code absent at the lines, suggested-fix pattern present, recent git activity referencing the comment's title).
  3. **Confirmation loop** — present each comment with the evidence summary to the user. Three response paths:
     - "Yes, it's fixed" → `tai complete <id> --resolution "<one-line summary of the evidence>"`. Move on.
     - "No, not fixed yet" → leave the comment as `accepted`. Move on.
     - "Show me again" → re-run `tai show <id>` and re-prompt.
     If the evidence is unambiguous (e.g. the file is gone, or the exact suggested code is in the diff), the slash command MAY surface a "high-confidence" indicator and ask for a simple confirmation; if ambiguous, it MUST present the evidence neutrally and let the user decide.
- Define the **conservative bias**: when evidence is weak or ambiguous, the slash command leaves the comment as `accepted` rather than asking the user to mark it completed. False positives (marking a comment fixed when it isn't) are worse than false negatives (failing to detect a real fix and asking the user to mark it manually). The user can always run `tai complete <id>` themselves.
- Define the **recap** at end of loop:
  ```
  Verification complete for acme/app PR #142.
    Marked completed: <N>
    Left accepted:    <M>

  Still-accepted work queue (severity order):
    [crit] 5: Replace execSync with execFileSync (src/api/auth.ts:15-29)
    ...

  Ready to prune the completed comments? Run `tai forget --status completed --yes`.
  ```
- Define **stack mode loop**: same per-PR cycle as `/tai:triage stack`. Recap is per-PR plus a stack-level aggregate.
- Author the version-1 markdown body for `commands/verify.md` and seed its `commands/verify.ledger.json` ledger.
- Define the slash command's **failure modes**:
  - `TRIAGE_NO_SCOPE` — same handling as `/tai:triage`: tell the user, exit.
  - No accepted comments in scope → announce "no accepted comments to verify" and exit without entering the loop.
  - `gh pr diff` fails (no GitHub auth, no PR, etc.) for a PR-scoped target → fall back to working-tree-only verification and warn the user that PR-diff evidence is unavailable.

## Capabilities

### New Capabilities

- `verify-command`: The bundled `/tai:verify` Claude slash command — its invocation forms, three-phase loop, evidence-gathering heuristics (working tree for branch scope, `gh pr diff` for PR scope), confirmation UX, conservative-bias rule, recap output, and stack-mode handling. The contract is owned by this spec; future revisions to the slash command's body require updating the spec in lockstep and bumping the frontmatter `version`.

### Modified Capabilities

None. The CLI verbs the slash command invokes (`tai status`, `tai list`, `tai show`, `tai complete`) already exist after `add-triage-state` lands. No new error codes.

## Impact

- No new external dependencies. The slash command's body is markdown plus shell invocations of `tai`, `gh`, and `git`. The same `gh` dependency `/tai:import` has for remote mode; `git` is universally available where tai runs.
- This proposal adds a THIRD bundled slash command, joining `/tai:import` and `/tai:triage`. The bundled set after this proposal: `tai-import`, `tai-triage`, `tai-verify`. The `tai install` machinery handles all three without modification.
- Depends on `add-triage-state` shipping the `--status` filter on `tai list` and `tai show --all` (added by the same proposal that introduced `tai forget --status` for bulk prune). Without those filters the slash command would need to read every comment and filter in Claude — workable but ugly.
- The "still-accepted work queue" recap exists to make the post-verification next-step obvious. Users land at: pruned completed comments, list of work that's still pending fixes.
- This proposal does NOT introduce a CLI verb `tai verify`. Verification requires AI reasoning (parsing diffs, matching fix patterns against suggested code) that the CLI is not in the business of doing. The "verify" identity lives entirely in the slash command.
