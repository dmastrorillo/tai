## Context

`/tai:verify` is the post-fix counterpart to `/tai:triage`'s Phase 1. Both look for evidence a comment has been addressed; they differ in scope and trust budget:

| | `/tai:triage` Phase 1 | `/tai:verify` |
|---|---|---|
| Operates on | `pending` comments | `accepted` comments |
| Runs when | Before triage decisions | After fix work |
| Evidence sources | Working tree + git log | Working tree + git log + `gh pr diff` (PR scope) |
| Auto-mark | Yes, when evidence is concrete | No, always asks the user |

The asymmetry on auto-marking matters: in Phase 1, a wrong auto-completion just sends the user through a "no actually this is still open" override moment. In verify, a wrong auto-completion (without user confirmation) silently moves an accepted comment out of the queue forever. The cost-of-mistake is asymmetric, so verify always asks.

This document records the technical choices behind verify's evidence model, the confirmation UX, and the failure modes. The corresponding `specs/verify-command/spec.md` carries the normative contract.

## Goals / Non-Goals

**Goals:**

- A loop that surfaces "we think this is fixed because X" to the user for each accepted comment, with X being concrete and visible.
- A clear distinction between PR-scoped verification (uses `gh pr diff` because the PR diff is the canonical source of truth) and branch-scoped verification (uses the working tree).
- Conservative defaults — never auto-mark; always ask.
- A recap that names the still-accepted work queue, because that's the user's next thing to do.

**Non-Goals:**

- A `tai verify` CLI verb. Verification is AI reasoning over diffs; the CLI is dumb persistence.
- Automated marking. The whole point is the user's confirmation.
- Verifying dismissed comments. Dismissals don't need verification; they're already out of the queue.
- Cross-PR verification. Each invocation operates on a single scope (stack mode iterates one PR at a time).
- Replacing `/tai:triage` Phase 1's investigation. The two are complementary; both ship.

## Decisions

### D1. PR-scope verification uses `gh pr diff`, branch-scope uses the working tree

For a PR target:

1. Pull the PR diff: `gh pr diff <number> --patch`.
2. Parse the diff to extract added/removed line blocks per file.
3. For each accepted comment, look in the diff at the comment's file/lines for either:
   - The original code (the snippet flagged in `description` or `lines`) being removed, OR
   - The suggested-fix pattern (extracted from `suggested_fix`) being added.
4. Cross-reference against the current working tree for the same patterns. (Catches fixes staged locally but not yet pushed.)

For a branch target:

1. Read the file at the comment's lines from the current working tree.
2. Check whether the original code is still present and whether the suggested-fix pattern appears in the file.
3. Walk `git log --oneline -10 -- <file>` for change subjects containing keywords from the comment's title.

**Alternatives considered:**

- Always use the working tree for both targets. Misses the case where the user is verifying a PR that's already merged into main but whose changes aren't reflected in the current branch. The PR diff is the canonical "what changed in this PR" answer.
- Always use the PR diff for both targets. Fails for branch-scoped targets that have no PR.
- Use `gh pr view --json files` for a coarser "did the file change at all" check. Too coarse — passes for any change to the file even if the comment's specific issue is untouched.

### D2. Three-tier evidence summary surfaced to the user

For each comment, the slash command produces an evidence summary at one of three confidence levels:

- **High confidence**: the original code is gone from the relevant location AND the suggested-fix pattern is present. Or: the file is gone entirely.
- **Medium confidence**: one of the two conditions holds (original code absent OR fix pattern present), or recent git history contains a relevant change subject.
- **Low confidence / no evidence**: nothing matches.

The user sees this confidence level explicitly:

```
Comment 5 (critical) — Replace execSync with execFileSync in src/api/auth.ts:15-29

Evidence (HIGH confidence):
  • `execSync` no longer appears at lines 15-29
  • `execFileSync` appears at lines 15-29 with matching argument shape
  • PR diff shows `-execSync(`git config ${k} ${v}`)` removed and `+execFileSync('git', ['config', k, v])` added

Mark as completed? [Y/n]
```

For high-confidence cases, the prompt defaults to `Y` (because the evidence is overwhelming). For medium, it defaults to `n` (because the user should look). For low/no evidence, the slash command MUST NOT prompt for completion — it announces "no evidence found; comment remains accepted" and moves on.

**Alternatives considered:**

- Binary confidence (evidence found / not). Loses the nuance that lets users move fast on obvious cases.
- Five tiers (very high / high / medium / low / very low). Premature precision.

### D3. Conservative bias on weak evidence

When evidence is "low confidence / no evidence", the slash command does NOT ask the user to mark completed. It simply reports "no evidence found" and moves on. The user can run `tai complete <id>` directly if they know it's fixed.

Why: the verify loop is designed to catch the easy cases at scale, not to put the user through 50 prompts asking "are you sure this wasn't fixed?". Weak-evidence comments stay in the accepted queue; users handle them via the normal "fix and mark complete" path.

**Alternatives considered:**

- Prompt on every comment regardless of evidence. Exhausting for users with many accepted comments still in flight.
- Auto-mark on weak evidence with a `--check` flag instead. The opposite of conservative; rejected.

### D4. Falling back when `gh pr diff` fails

For a PR target, if `gh pr diff <N>` fails (network, auth, no PR found), the slash command MUST:

1. Warn the user that PR-diff evidence is unavailable.
2. Fall back to working-tree-only verification (same heuristics as branch-scope).
3. Lower the confidence ceiling: high-confidence verdicts based on working-tree evidence alone are downgraded to medium. PR-diff evidence is the strongest signal; without it, certainty drops.

**Alternatives considered:**

- Abort the entire loop. Hostile when the user just wants the working-tree check.
- Pretend nothing's wrong and proceed with working-tree-only evidence at full confidence. Misleading.

### D5. Recap names the still-accepted work queue

After the loop, the slash command emits a recap. Beyond the marked-completed and left-accepted counts, it surfaces every still-accepted comment as a one-line bullet sorted by severity. This is the user's actual TODO list. Without it, users either re-run `tai list --status accepted` or paw through the conversation.

The recap ends by suggesting the prune command:

```
Ready to prune the completed comments? Run `tai forget --status completed --yes`.
```

Suggesting the literal command is friendly to AI consumers — Claude can pick it up and offer to run it directly.

### D6. Stack mode mirrors `/tai:triage stack`

For `/tai:verify stack`:

1. Enumerate the stack via `st_reviews` or `gh pr list` (ancestor-first).
2. For each PR: announce `Now verifying PR #<N> (1 of K)`; run the full three-phase loop.
3. Between PRs: per-PR recap, pause for user acknowledgement.
4. End: stack-level aggregate.

The mechanism is identical to triage's stack loop; the body of the per-PR phase is what differs.

### D7. Frontmatter conforms to `command-framework`; version starts at 1

```yaml
---
name: "TAI: Verify"
description: "Check whether accepted comments have been addressed; mark completed."
category: "Workflow"
tags: [tai, triage, verify]
version: 1
content_hash: sha256:<computed at build time>
---
```

## Risks / Trade-offs

- **[`gh pr diff` is slow for large PRs]** A 200-file PR diff might take a few seconds to fetch. → Acceptable; verify is a once-off operation after fix work, not a hot path.

- **[Diff parsing is fragile]** `gh pr diff` output is unified-diff format which is well-specified, but pattern matching against suggested-fix text is heuristic. → Mitigated by showing the user the evidence summary; if matching is wrong, the user says "no, not fixed" and moves on. False negatives are tolerated; false positives are filtered by the always-ask rule.

- **[Working-tree fallback on PR auth failure]** Users without `gh` auth get a degraded experience. → Documented. The fallback works; it's just less confident. Encouraging `gh auth login` is part of the warning message.

- **[Stale evidence after the user's last commit]** If the user is mid-fix (uncommitted changes), the working tree may show in-progress code. → Acceptable; verify is for confirming finished fixes. Mid-flight code is expected to look incomplete.

- **[Severity-order recap might miss subtle fixes]** A medium-severity comment that's been fixed but with weak evidence stays in the recap. → Documented as the conservative bias; users can manually `tai complete <id>` for the cases they know.

- **[Cost of always asking]** Users with 50 accepted comments will see 50 prompts (minus the ones with no evidence, which are silent). → Acceptable trade-off given the asymmetric cost of mistakes. If anyone asks for a `--auto-high-confidence` mode later, easy to add.

## Migration Plan

This is the first bundled `/tai:verify` command; no prior version. Users running tai for the first time after this proposal lands install the new command via `tai install` (which classifies it as `missing` and writes it). No conversation state to migrate.

## Open Questions

(None remaining.)
