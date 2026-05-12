## Why

The CLI verbs introduced by `add-triage-state` are minimal and impartial: they read, transition, and report state. They have no opinions about *which* comment to present next, *how* to investigate whether a comment has already been fixed, *whether* a user's dismissal reasoning holds water, or *how* to debate it. Those are the responsibility of the `/tai:triage` Claude slash command — the AI-driven loop that walks a user through every pending comment in a scope.

This proposal authors that loop. The slash command is the user-visible counterpart to the `pr-review-triage` skill that motivated tai's existence, ported to use tai's CLI verbs for persistence and output. Importantly, **everything subjective lives in the slash command, not the CLI**: severity weighting in batch presentation order, the sparring debate when a user dismisses, the cognitive-bias checklist, the per-batch override semantics in conversation. The CLI is the keyboard; the slash command is the pianist.

## What Changes

- Introduce the bundled `/tai:triage` Claude slash command, installed by `tai install` at `~/.claude/commands/tai/triage.md`. The command's body specifies a four-phase loop:
  1. **Scope resolution** — accept invocations:
     - `/tai:triage` — auto-detect current scope (calls `tai status` to verify a scope resolves).
     - `/tai:triage --pr <number-or-url>` — single PR.
     - `/tai:triage --branch <name>` — branch-scoped review.
     - `/tai:triage stack` — every PR from trunk to the current branch (loops per PR, ancestor-first).
  2. **Investigation** — for every `pending` comment in scope, the slash command searches the working tree and recent git history for evidence the comment has already been addressed (the suggested change appears in the diff; the file no longer exists; the suggested function call replaces the flagged one). When evidence is found, the slash command runs `tai complete <id> --resolution "<what was found>"` and moves on. The user MAY override an automatic completion if the evidence is misread.
  3. **Triage loop** — present batches first (ordered by highest severity member), then remaining individual comments (severity-sorted). For each item:
     - Run `tai show <id>` (or `tai show --batch <key>` once that surface lands, otherwise iterate `tai show` per member) and surface the markdown verbatim.
     - Ask the user `Accept, dismiss, or complete? Any thoughts on the fix?`.
     - On accept: optionally capture a resolution; call `tai accept <id> [--resolution <text>]`.
     - On dismiss: engage in sparring per the dismissal-debate contract below.
     - On complete: capture the resolution; call `tai complete <id> --resolution <text>`.
     - After each decision, run `tai status` and surface the updated counts.
     - Allow per-item overrides within a batch decision (the user may say "accept B1 except item 4"; the slash command splits the batch decision accordingly).
  4. **Recap** — once `tai status` shows zero pending comments, summarise: accepted / dismissed / completed counts, per-batch breakdown, and the accepted comments ordered by severity (the implied work queue).
- Specify the **dismissal-debate contract** in the slash command's body — the rules that distinguish a real debate from theatre:
  - Understand the user's reasoning before pushing back.
  - Challenge with concrete scenarios, not "are you sure?".
  - Watch for cognitive biases: assumptions treated as facts; scope dismissal ("nobody would do that"); effort bias; anchoring on the reviewer's framing.
  - Calibrate depth to severity: critical security issues warrant multiple rounds; style nitpicks do not.
  - When the debate concludes, record the outcome via `tai dismiss <id> --reason <text>` with the agreed reasoning.
- Specify the **batch-override convention**: when a batch decision is made, the slash command's body treats the entire batch as the default, but the user MAY name a subset (`only fix B1.2 and B1.4`, `dismiss all of B1 except B1.5`). The slash command splits the decision: members that match the batch decision get `tai accept --batch <key>` / `tai dismiss --batch <key>`; overridden members get individual `tai accept <id>` / `tai dismiss <id>`. The result is a `mixed` batch status, which is intentional.
- Author the version-1 markdown body for `commands/triage.md` and seed its `commands/triage.ledger.json` ledger with that body's content hash. Frontmatter values: `name: "TAI: Triage"`, `description: "Walk through pending PR review comments interactively, batches-first."`, `category: "Workflow"`, `tags: [tai, triage, review]`, `version: 1`, `content_hash: sha256:…`.
- Define the slash command's **failure modes**:
  - `TRIAGE_NO_SCOPE` from `tai status` → tell the user to either `/tai:import` first or specify `--pr`/`--branch`. Exit the conversation.
  - `TRIAGE_AMBIGUOUS_SCOPE` → ask the user to choose; re-invoke with the explicit flag.
  - Empty scope (target rows exist but no pending comments) → announce "All comments in scope are triaged" with a brief recap; do NOT enter the loop.

## Capabilities

### New Capabilities

- `triage-command`: The bundled `/tai:triage` Claude slash command — its invocation forms, four-phase loop, dismissal-debate contract, batch-override convention, frontmatter requirements, and recap output. The contract is owned by this spec; future revisions to the slash command's body require updating the spec in lockstep with `commands/triage.md` and bumping its frontmatter `version`.

### Modified Capabilities

None. The CLI verbs invoked by the slash command (`tai status`, `tai show`, `tai accept`, `tai dismiss`, `tai complete`) are unchanged — `add-triage-state` already specifies them. No new error codes are introduced.

## Impact

- No new external dependencies. The slash command's body is markdown plus shell invocations of `tai`. No `gh`, no `st_reviews`, no other tools — investigation walks the working tree, not the network.
- This proposal adds a SECOND bundled slash command, joining `/tai:import` from `add-import-command`. The bundled set is now: `tai-import`, `tai-triage`. The `tai install` machinery from `add-install-command` already handles both without modification.
- The slash command's behavioural contract is part of the spec, not just the markdown body. A reviewer can audit `commands/triage.md` against `specs/triage-command/spec.md` the same way they audit Go code against a spec.
- The CLI verb surface introduced by `add-triage-state` is sufficient to drive every step of the loop. No new verbs are required. (`tai show --batch <key>` was mentioned as a future convenience — it is NOT introduced here; the slash command iterates over batch members today.)
- The slash command's investigation phase is conservative by design: it only marks `completed` when evidence is concrete. False positives (marking completed when not actually fixed) would mask real issues; false negatives (failing to mark completed when fixed) just send the user through an extra "looks already fixed" conversation. The slash command body errs toward false negatives.
