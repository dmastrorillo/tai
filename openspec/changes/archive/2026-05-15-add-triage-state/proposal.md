## Why

Imported review comments sit in the database as `pending` rows. Nothing useful happens until a user can read them, decide on each one, and record those decisions. This proposal introduces the read-and-mutate surface that closes the triage loop: list, show, accept, dismiss, complete, status, and forget.

These verbs are the most-called part of tai. The `/tai:triage` Claude slash command (specified in `add-triage-command`) drives users through the loop by repeatedly invoking them; humans use them directly from the shell. Designing the verbs together — with a consistent scope-resolution rule, consistent ID scheme, and a single error taxonomy — keeps both the AI-facing surface and the human-facing surface coherent.

This proposal is also where the AI-friendly markdown output contract lives. `tai show` is the primary way Claude reads a comment back from the database; getting its shape right matters as much as the database schema does.

## What Changes

- Introduce seven verbs:
  - `tai list [--status <state> …]` — table-format overview of comments in the resolved scope. `--status` (repeatable) filters by status.
  - `tai show <id>` / `tai show --all [--status <state> …]` — markdown block(s) for AI consumption, one per comment. `--status` applies only to `--all`.
  - `tai accept <id> [--resolution <text>]` — transition to `accepted`; record optional resolution.
  - `tai dismiss <id> --reason <text> [--by <name>]` — transition to `dismissed`; `--reason` is REQUIRED; `--by` defaults to `git config user.name` or `$USER`.
  - `tai complete <id> [--resolution <text>]` — transition to `completed` (already fixed in code).
  - `tai status` — summary block: counts by status, batch overview, scope header.
  - `tai forget` — destructive: deletes a comment, batch, PR, branch, or whole repo. The optional `--status <state>` modifier (repeatable) on `--pr`/`--branch`/`--repo`/`--batch` narrows the deletion to comments matching the supplied statuses (parent rows are preserved). Requires explicit consent in all cases.

- Define a single **scope-resolution rule** shared by every verb except `tai forget --repo`:
  1. If `--pr <N>` is provided, scope is `(current repo, PR N)`.
  2. Else if `--branch <name>` is provided, scope is `(current repo, branch name)`.
  3. Else auto-detect from the current git branch: look up `prs.head_branch = <current>` first, then `branches.name = <current>`. Match exactly one or error with `TRIAGE_AMBIGUOUS_SCOPE` / `TRIAGE_NO_SCOPE`.

- Define **user-facing comment IDs** as per-target sequential positions, computed at query time as `ROW_NUMBER() OVER (PARTITION BY target ORDER BY comments.id ASC)`. The internal DB id is never exposed. Positions are stable within a target unless a comment is deleted via `tai forget`; this trade-off is documented for users.

- Define **batch-wide operations**: `tai accept --batch <key>`, `tai dismiss --batch <key> --reason <text>`, `tai complete --batch <key>` apply the same transition to every member of the batch in one transaction. Mixed batches (some members already in a different terminal state) update only the members whose state differs from the requested target state; the summary reports per-member outcomes.

- Define a **liberal state machine**: any current status can transition to any other terminal status (`accepted`, `dismissed`, `completed`) or back to `pending` by re-invoking the matching verb. Self-transitions are idempotent no-ops. The "frozen on import" rule from `add-import-command` is unaffected — that rule is about *imports* not silently mutating user decisions; explicit user verbs always win.

- Define the **markdown output contract** for `tai show`. Each comment renders as a single markdown block with stable section names (`# header`, `**meta**`, `## Title`, `## Description`, `## Why fix it`, `## Suggested fix`, `## What happens if you don't fix it`, plus `## Resolution` or `## Dismissed because` when present). `tai show --all` joins blocks with a `\n\n---\n\n` separator.

- Define the **`tai forget` consent model**:
  - Default: print a summary of what will be deleted, prompt `Continue? [y/N]` (default N).
  - `--yes` flag skips the prompt for the current invocation.
  - Env var `TAI_ACCEPT_DESTRUCTIVE` honoured with truthy semantics (same helper as `TAI_ACCEPT_COMMAND_UPDATES`).
  - Non-interactive stdin without `--yes` and without env var → exit `1` with `TRIAGE_CONFIRMATION_REQUIRED`. Unlike install, destructive operations fail loudly rather than skip silently.

- Define `tai forget` mutex scope flags: exactly one of `--repo <owner/name>`, `--pr <N>`, `--branch <name>`, or `--comment <id>` is required. `--repo` is the only mode that runs without repo context (it carries its own identity); the others use the standard scope resolution.

- Reserve new error codes in the `cli-framework` taxonomy: `TRIAGE_NO_SCOPE` (exit 2), `TRIAGE_NOT_FOUND` (exit 2), `TRIAGE_AMBIGUOUS_SCOPE` (exit 2), `TRIAGE_CONFIRMATION_REQUIRED` (exit 1), `TRIAGE_INVALID_FLAGS` (exit 1).

## Capabilities

### New Capabilities

- `triage`: The list / show / accept / dismiss / complete / status / forget verbs, scope resolution, per-target sequential ID scheme, batch-wide operations, the state machine, the markdown output contract for `tai show`, the `tai forget` consent model, and the triage-layer error codes.

### Modified Capabilities

- `cli-framework`: Extends the error-code taxonomy with `TRIAGE_NO_SCOPE`, `TRIAGE_NOT_FOUND`, `TRIAGE_AMBIGUOUS_SCOPE`, `TRIAGE_CONFIRMATION_REQUIRED`, `TRIAGE_INVALID_FLAGS`. Additive per the append-only rule.

## Impact

- No new third-party dependencies. The verbs use the storage layer introduced by `add-storage-schema` plus the `cli-framework` foundation.
- This proposal does NOT introduce the `/tai:triage` Claude slash command. That is the subject of `add-triage-command`, which drives the verbs through a sparring-style conversation. The verbs work standalone for humans who type them directly.
- The user-facing ID scheme (per-target sequential) is a deliberate divergence from the JSON sidecar today, which used per-PR sequential and exposed it. The semantics are similar; the implementation is now a query-time computation rather than a stored field.
- All seven verbs require either resolved repo context or an explicit `--repo` (in the case of `tai forget --repo`); none are "anywhere" verbs like `tai install`.
- Output from `tai show` is the contract the `/tai:triage` skill (and any future AI integration) parses. Changing the markdown shape after v1 ships is a breaking change to integrations and requires its own proposal.
