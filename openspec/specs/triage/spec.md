# triage Specification

## Purpose

The `triage` capability defines the seven CLI verbs that move imported review comments through their lifecycle: `tai list`, `tai show`, `tai accept`, `tai dismiss`, `tai complete`, `tai status`, and `tai forget`. It owns the scope-resolution rule (`--pr` > `--branch` > auto-detect from current git branch) shared by every verb except `tai forget --repo`; the user-facing per-target sequential ID scheme (computed via `ROW_NUMBER()` at query time); the liberal state machine that lets users change their minds across `accepted`/`dismissed`/`completed`/`pending`; first-class batch-wide operations on accept/dismiss/complete; the stable markdown output contract for `tai show` (the AI-facing read surface); the destructive consent model for `tai forget` (interactive prompt, `--yes` flag, `TAI_ACCEPT_DESTRUCTIVE` env var); and the triage-layer error codes.

## Requirements

### Requirement: Scope resolution

The system SHALL resolve the operating scope for every triage verb (except `tai forget --repo`) using this precedence:

1. If `--pr <N>` is provided, scope is the `prs` row matching `(repo_id, number=N)`. If no such row exists, exit `2` with `TRIAGE_NOT_FOUND`.
2. Else if `--branch <name>` is provided, scope is the `branches` row matching `(repo_id, name)`. If no such row exists, exit `2` with `TRIAGE_NOT_FOUND`.
3. Else (auto-detect): read the current git branch via `git rev-parse --abbrev-ref HEAD`. Look up `prs` where `head_branch == <current>` AND `repo_id == <current>`, and `branches` where `name == <current>` AND `repo_id == <current>`.
   - If exactly one row resolves, that is the scope.
   - If both resolve, exit `2` with `TRIAGE_AMBIGUOUS_SCOPE`.
   - If neither resolves, exit `2` with `TRIAGE_NO_SCOPE`.

`--pr` and `--branch` are mutually exclusive. Providing both exits `1` with `TRIAGE_INVALID_FLAGS`.

#### Scenario: --pr resolves to existing PR

- **WHEN** `tai list --pr 142` is invoked and a `prs` row exists with number 142 in the current repo
- **THEN** the verb operates on that PR's comments

#### Scenario: --pr resolves to non-existent PR

- **WHEN** `tai list --pr 999` is invoked and no such PR row exists
- **THEN** the CLI exits `2` with `TRIAGE_NOT_FOUND`

#### Scenario: Auto-detect from current branch — PR match

- **WHEN** the current git branch is `feat/oauth`
- **AND** a `prs` row exists with `head_branch='feat/oauth'`
- **AND** no `branches` row exists with `name='feat/oauth'`
- **THEN** the scope is that PR

#### Scenario: Auto-detect from current branch — branch match

- **WHEN** the current git branch is `feat/x`
- **AND** no `prs` row matches `head_branch='feat/x'`
- **AND** a `branches` row exists with `name='feat/x'`
- **THEN** the scope is that branch

#### Scenario: Auto-detect — both match

- **WHEN** both a PR with `head_branch='feat/x'` and a `branches` row `name='feat/x'` exist
- **THEN** the CLI exits `2` with `TRIAGE_AMBIGUOUS_SCOPE`

#### Scenario: Auto-detect — neither matches

- **WHEN** the current branch matches no `prs.head_branch` and no `branches.name`
- **THEN** the CLI exits `2` with `TRIAGE_NO_SCOPE`

#### Scenario: --pr and --branch together

- **WHEN** `tai list --pr 142 --branch feat/x` is invoked
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

### Requirement: User-facing comment IDs

The system SHALL surface comment IDs as per-target sequential positions. For the comments of a single target, IDs are computed at query time as `ROW_NUMBER() OVER (PARTITION BY parent ORDER BY comments.id ASC)`, starting at `1`.

Internal database IDs MUST NOT appear in any user-facing output or be accepted by any user-facing flag. All verbs accept and emit per-target positions.

#### Scenario: Sequential IDs start at 1 within a target

- **WHEN** a target has comments with internal IDs 482, 483, 484
- **THEN** `tai list` displays them as IDs 1, 2, 3

#### Scenario: IDs are scoped per target

- **WHEN** two PRs each have comments
- **THEN** each PR's first comment is displayed as `1`
- **AND** `tai accept 1 --pr 142` mutates PR 142's first comment without affecting PR 200

#### Scenario: ID shift after delete

- **WHEN** a target has 5 comments displayed as 1..5
- **AND** the user runs `tai forget --comment 3`
- **THEN** subsequent `tai list` calls display the remaining comments as 1, 2, 3, 4

### Requirement: `tai list` output

The system SHALL provide a `tai list` subcommand that displays comments in the resolved scope as a table:

```
Repo: <owner>/<name>   Scope: <target-label>

  ID  SEV    STATUS     BATCH  FILE                      TITLE
  …
```

`<target-label>` is `PR #<number> — <title>` for PR targets and `branch <name>` for branch targets.

Severity is abbreviated: `critical → crit`, `major → maj`, `minor → min`, `nitpick → nit`. Status is the full word (`pending`, `accepted`, `dismissed`, `completed`). Batch column shows the batch key (e.g. `B1`) or `-` when the comment is not in a batch. Title is truncated to fit the terminal width with `…` (falling back to 80 chars when the width can't be determined).

`tai list` SHALL accept zero or more `--status <state>` flags. When one or more are provided, only comments whose `status` matches one of the supplied values appear in the output. When no `--status` flag is given, all statuses are shown.

`--status` values are the four valid comment statuses (`pending`, `accepted`, `dismissed`, `completed`). An unknown value exits `1` with `TRIAGE_INVALID_FLAGS`.

When the filtered result has zero rows, `tai list` prints the header line followed by `(no comments)` and exits `0`. (The filtered-out case is indistinguishable from "scope has no comments at all" — both produce the same output. This is intentional.)

#### Scenario: List with comments

- **WHEN** `tai list` is invoked in a scope with three comments
- **THEN** stdout contains the header and three rows

#### Scenario: List with no comments

- **WHEN** `tai list` is invoked in a scope with no comments
- **THEN** stdout contains the header line and the literal line `(no comments)`
- **AND** the CLI exits `0`

#### Scenario: --status filter narrows the result

- **WHEN** a scope has three pending comments and two accepted comments
- **AND** `tai list --status accepted` is invoked
- **THEN** the output shows two rows, both with status `accepted`

#### Scenario: Multiple --status values combine via OR

- **WHEN** a scope has comments in every status
- **AND** `tai list --status accepted --status completed` is invoked
- **THEN** the output shows only `accepted` and `completed` rows

#### Scenario: Unknown --status value

- **WHEN** `tai list --status urgent` is invoked
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

### Requirement: `tai show` markdown output contract

The system SHALL provide `tai show <id>` and `tai show --all` subcommands that emit one markdown block per comment with the following structure:

1. **H1 heading line** — `# <repo> <target-label> — comment <pos> of <total>`.
2. **Blank line.**
3. **Meta lines** in this order, each on its own line:
   - `**Severity:** <severity>  **Category:** <category>  **Status:** <status>` (one line, double-space between key/value pairs)
   - `**File:** ` followed by the file path and lines in backticks: `` `<file>:<lines>` ``
   - `**Source:** <source>`
   - `**Batch:** <batch_key> — <batch_title>` — present ONLY when the comment has a `batch_id`.
4. **Blank line.**
5. **Five mandatory H2 sections** in this exact order, each preceded by a blank line:
   - H2 heading `Title`, followed by the comment's `title` text.
   - H2 heading `Description`, followed by the comment's `description` text.
   - H2 heading `Why fix it`, followed by the comment's `why_fix` text.
   - H2 heading `Suggested fix`, followed by the comment's `suggested_fix` text.
   - H2 heading `What happens if you don't fix it`, followed by the comment's `consequences` text.
6. **Conditional H2 `Resolution` section** — present ONLY when `status` is `accepted` or `completed` AND `resolution` is non-null. Body is the resolution text.
7. **Conditional H2 `Dismissed because` section** — present ONLY when `status` is `dismissed`. Body is `<dismiss_reason> (by <dismissed_by>)`.

The H2 heading text (`Title`, `Description`, `Why fix it`, `Suggested fix`, `What happens if you don't fix it`, `Resolution`, `Dismissed because`) MUST match exactly. No section may be renamed or reordered.

`tai show --all` emits one block per comment in the scope, ordered by target position ascending. Blocks are joined by a separator consisting of a blank line, a line containing exactly `---`, and another blank line.

`tai show --all` SHALL accept zero or more `--status <state>` flags using the same semantics as `tai list --status`: when supplied, only matching comments are surfaced. `tai show <id>` (single-comment form) does NOT accept `--status` — it always shows the requested comment regardless of status.

#### Scenario: Pending comment without batch

- **WHEN** `tai show 1` is invoked for a pending comment with no `batch_id`
- **THEN** the output omits the `**Batch:**` meta line
- **AND** omits the `Resolution` and `Dismissed because` H2 sections
- **AND** includes the five mandatory H2 sections in order

#### Scenario: Accepted comment with resolution

- **WHEN** `tai show 1` is invoked for an accepted comment with `resolution = 'use execFileSync'`
- **THEN** the output contains an H2 `Resolution` section whose body is `use execFileSync`

#### Scenario: Dismissed comment

- **WHEN** `tai show 1` is invoked for a dismissed comment
- **THEN** the output contains an H2 `Dismissed because` section
- **AND** the body of that section ends with `(by <dismissed_by>)`
- **AND** the output omits the `Resolution` H2 section

#### Scenario: Comment with batch shows batch meta

- **WHEN** `tai show 1` is invoked for a comment whose `batch_id` references batch `B1` with title `Replace execSync`
- **THEN** the output's meta lines include `**Batch:** B1 — Replace execSync`

#### Scenario: --all joins with horizontal rule

- **WHEN** `tai show --all` is invoked for a scope with two comments
- **THEN** stdout contains both blocks separated by a blank line, a line containing exactly `---`, and a blank line

#### Scenario: --all with no comments

- **WHEN** `tai show --all` is invoked for a scope with no comments
- **THEN** stdout is empty (zero bytes)
- **AND** the CLI exits `0`

#### Scenario: Section names and order are stable

- **WHEN** `tai show 1` is invoked
- **THEN** the H2 headings appear in this order: `Title`, `Description`, `Why fix it`, `Suggested fix`, `What happens if you don't fix it`, optionally `Resolution` or `Dismissed because`

#### Scenario: Pending comment without batch

- **WHEN** `tai show 1` is invoked for a pending comment with no batch_id
- **THEN** the output omits the `**Batch:**` meta line
- **AND** omits the `## Resolution` and `## Dismissed because` sections

#### Scenario: Accepted comment with resolution

- **WHEN** `tai show 1` is invoked for an accepted comment with `resolution = 'use execFileSync'`
- **THEN** the output contains a `## Resolution` section with `use execFileSync`

#### Scenario: Dismissed comment

- **WHEN** `tai show 1` is invoked for a dismissed comment
- **THEN** the output contains a `## Dismissed because` section
- **AND** the line includes the reason followed by `(by <name>)`
- **AND** omits the `## Resolution` section

#### Scenario: --all joins with horizontal rule

- **WHEN** `tai show --all` is invoked for a scope with two comments
- **THEN** stdout contains both blocks separated by `\n\n---\n\n`

#### Scenario: --all with no comments

- **WHEN** `tai show --all` is invoked for a scope with no comments
- **THEN** stdout is empty (zero bytes)
- **AND** the CLI exits `0`

### Requirement: `tai accept` transitions a comment to `accepted`

The system SHALL provide a `tai accept <id>` subcommand that sets the comment's `status` to `accepted`. When `--resolution <text>` is provided, `resolution` is updated; otherwise `resolution` is left as-is (or set to NULL if no prior value).

`dismissed_by` and `dismiss_reason` are set to NULL on this transition.

`tai accept` is idempotent: invoking it on an already-`accepted` comment is a success. If `--resolution` is provided, the resolution updates; otherwise the row is untouched.

`tai accept --batch <key>` accepts every member of the named batch within the resolved scope. The batch's `status` is recomputed after the transition.

`--batch` and the positional `<id>` are mutually exclusive. Providing both exits `1` with `TRIAGE_INVALID_FLAGS`.

#### Scenario: Accept a pending comment

- **WHEN** `tai accept 1 --resolution "use execFileSync"` is invoked on a pending comment
- **THEN** the comment's status becomes `accepted`
- **AND** its `resolution` is `"use execFileSync"`
- **AND** `dismissed_by` and `dismiss_reason` are NULL

#### Scenario: Accept a dismissed comment (state reversal)

- **WHEN** `tai accept 1` is invoked on a dismissed comment
- **THEN** the comment's status becomes `accepted`
- **AND** `dismissed_by` and `dismiss_reason` are NULL

#### Scenario: Accept an already-accepted comment is idempotent

- **WHEN** `tai accept 1` is invoked twice in succession (with no flag changes)
- **THEN** both invocations exit `0`
- **AND** the second invocation does not modify the row

#### Scenario: Accept by batch

- **WHEN** a batch `B1` has three pending members
- **AND** `tai accept --batch B1` is invoked
- **THEN** all three members become `accepted`
- **AND** the batch's `status` becomes `accepted`

#### Scenario: Accept --id and --batch together

- **WHEN** `tai accept 1 --batch B1` is invoked
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

#### Scenario: Accept non-existent ID

- **WHEN** `tai accept 99` is invoked when the scope has only 3 comments
- **THEN** the CLI exits `2` with `TRIAGE_NOT_FOUND`

### Requirement: `tai dismiss` transitions a comment to `dismissed`

The system SHALL provide a `tai dismiss <id> --reason <text>` subcommand that sets the comment's `status` to `dismissed`. `--reason` is REQUIRED — invocations missing it exit `1` with `TRIAGE_INVALID_FLAGS`.

`--by <name>` overrides the default attribution; if omitted, the value of `git config --get user.name` is used, falling back to `$USER`, falling back to the literal string `"unknown"`.

`resolution` is set to NULL on this transition. `dismiss_reason` and `dismissed_by` are set to the provided/derived values.

`tai dismiss` is idempotent on an already-dismissed comment: the `dismiss_reason` and `dismissed_by` fields are updated if `--reason` differs.

`tai dismiss --batch <key> --reason <text>` dismisses every member of the named batch.

#### Scenario: Dismiss a pending comment

- **WHEN** `tai dismiss 1 --reason "false positive — that path is read-only"` is invoked
- **THEN** the comment's status becomes `dismissed`
- **AND** `dismiss_reason` is set to the provided text
- **AND** `dismissed_by` is set to the user's git config name (or $USER fallback)
- **AND** `resolution` is NULL

#### Scenario: Dismiss without --reason

- **WHEN** `tai dismiss 1` is invoked without `--reason`
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`
- **AND** stderr explains that `--reason` is required

#### Scenario: Dismiss by batch

- **WHEN** `tai dismiss --batch B1 --reason "out of scope"` is invoked
- **THEN** every batch member becomes `dismissed` with the provided reason

#### Scenario: Dismiss state reversal

- **WHEN** `tai dismiss 1 --reason "..."` is invoked on a previously-accepted comment
- **THEN** the comment's status becomes `dismissed`
- **AND** `resolution` is cleared (set to NULL)
- **AND** `dismiss_reason` is set to the new reason

### Requirement: `tai complete` transitions a comment to `completed`

The system SHALL provide a `tai complete <id> [--resolution <text>]` subcommand that sets the comment's `status` to `completed`. When `--resolution` is provided, the resolution is recorded (typically describing what was found during investigation).

`dismissed_by` and `dismiss_reason` are set to NULL on this transition.

Idempotency, batch support (`--batch`), and the `--batch`/`<id>` mutex rule are the same as for `tai accept`.

#### Scenario: Complete a pending comment

- **WHEN** `tai complete 1 --resolution "already fixed in e7eeec0"` is invoked on a pending comment
- **THEN** the comment's status becomes `completed`
- **AND** `resolution` is set to the provided text

#### Scenario: Complete by batch

- **WHEN** `tai complete --batch B1` is invoked
- **THEN** every batch member becomes `completed`
- **AND** the batch's status becomes `completed`

### Requirement: Batch status recomputation

The system SHALL recompute a batch's `status` after every transition affecting one or more of its members:

- All members in `pending` → batch is `pending`.
- All members in the same terminal state (`accepted` / `dismissed` / `completed`) → batch matches that state.
- Members split across two or more distinct states (terminal or pending) → batch is `mixed`.

The recompute runs inside the same transaction as the member mutations.

#### Scenario: All members accepted → batch accepted

- **WHEN** every member of `B1` transitions to `accepted`
- **THEN** `B1.status` becomes `accepted`

#### Scenario: Members split → batch mixed

- **WHEN** `B1` has two members: one `accepted` and one `dismissed`
- **THEN** `B1.status` is `mixed`

#### Scenario: Recompute on single-member change

- **WHEN** a single member of `B1` (where all others are `accepted`) is changed to `dismissed`
- **THEN** `B1.status` recomputes to `mixed`

### Requirement: `tai status` summary

The system SHALL provide a `tai status` subcommand that prints a compact summary of the resolved scope:

```
Repo: <owner>/<name>
Scope: <target-label>

Counts:
  Total:      <N>
  Pending:    <P>
  Accepted:   <A>
  Dismissed:  <D>
  Completed:  <C>

Batches: <batch-count>
  <key> (<n> comments — <status>) — <title>
  …

[exit 0]
```

For PR targets, `<target-label>` is `PR #<number> — <title> (branch: <head_branch>)`.
For branch targets, `<target-label>` is `branch <name>`.

When the scope has zero batches, the `Batches:` block is omitted (the line itself and its bullet list).

When the scope has zero comments, the counts block reads `Total: 0` and other rows are omitted.

#### Scenario: PR status with batches

- **WHEN** `tai status` runs in a PR scope with 12 comments and one batch
- **THEN** stdout contains the `Repo:` line, the `Scope:` line with PR number, the counts block, and a `Batches: 1` block with the batch entry

#### Scenario: Branch status without batches

- **WHEN** `tai status` runs in a branch scope with 3 comments and no batches
- **THEN** stdout contains the `Repo:` line, the `Scope: branch <name>` line, and the counts block; no `Batches:` line is printed

### Requirement: `tai forget` consent model

The system SHALL provide a `tai forget` subcommand that deletes one of: a single comment, a single batch, a single PR/branch, an entire repo, or every comment in a scope filtered by status. Exactly one of `--comment <id>`, `--batch <key>`, `--pr <N>`, `--branch <name>`, or `--repo <owner/name>` MUST be provided as the selector. Zero or more-than-one selectors exits `1` with `TRIAGE_INVALID_FLAGS`.

The optional `--status <state>` modifier (repeatable) narrows the deletion to comments whose `status` matches one of the supplied values. When `--status` is combined with:

- `--pr <N>` / `--branch <name>`: only matching comments under that target are deleted; the PR/branch row itself is preserved.
- `--repo <owner/name>`: only matching comments across the entire repo are deleted; PRs, branches, and the repo row are preserved.
- `--comment <id>`: invalid — comments are already addressed by ID. Exits `1` with `TRIAGE_INVALID_FLAGS`.
- `--batch <key>`: only matching members of that batch are deleted; the batch row is preserved but its status recomputes per `add-triage-state`'s batch-status rule.

Without `--status`, `--pr`/`--branch`/`--repo` delete the row and everything cascading from it, and `--batch` deletes the batch (member comments survive with `batch_id` set to NULL per the storage schema's cascade rules).

Before deleting, the CLI MUST print a summary of what will be deleted:

```
You're about to delete:
  • <human-readable description>
  • <N> comments
  • <M> batches
  • <R> external references
This cannot be undone. Continue? [y/N]
```

Counts MUST include the cascade results.

Consent is granted by any of:

- An interactive answer of `y`/`Y` at the prompt (default on empty input is `N`).
- The `--yes` flag (skips the prompt for this invocation).
- The `TAI_ACCEPT_DESTRUCTIVE` environment variable set to a truthy value (same `isTruthyEnv` semantics as `TAI_ACCEPT_COMMAND_UPDATES`).

If consent is denied or stdin is non-interactive and neither `--yes` nor the env var is set, the CLI exits `1` with `TRIAGE_CONFIRMATION_REQUIRED`. No data is deleted.

`tai forget --repo` is the only mode that does NOT require resolved repo context. It MAY run anywhere on the filesystem. The other selectors use the standard scope-resolution rule (auto-detect from current branch, or explicit `--pr`/`--branch` to identify the parent for `--comment`/`--batch`).

The actual delete MUST run inside a single transaction.

#### Scenario: --repo prompts and deletes on consent

- **WHEN** `tai forget --repo acme/app` is invoked from a TTY
- **AND** the user answers `y`
- **THEN** the `repos` row and all cascaded rows are deleted
- **AND** the CLI exits `0`

#### Scenario: --repo declined leaves data intact

- **WHEN** `tai forget --repo acme/app` is invoked
- **AND** the user answers `n`
- **THEN** no rows are deleted
- **AND** the CLI exits `1` with `TRIAGE_CONFIRMATION_REQUIRED`

#### Scenario: --yes skips prompt

- **WHEN** `tai forget --pr 142 --yes` is invoked
- **THEN** the prompt is suppressed
- **AND** the PR and its cascaded rows are deleted

#### Scenario: TAI_ACCEPT_DESTRUCTIVE skips prompt

- **WHEN** `tai forget --branch feat/x` is invoked with `TAI_ACCEPT_DESTRUCTIVE=true` set
- **THEN** the prompt is suppressed
- **AND** the branch row and its cascaded rows are deleted

#### Scenario: Non-interactive stdin without consent

- **WHEN** `tai forget --pr 142` is invoked with stdin not a TTY and neither `--yes` nor a truthy env var
- **THEN** the CLI exits `1` with `TRIAGE_CONFIRMATION_REQUIRED`
- **AND** no data is deleted

#### Scenario: Zero selectors

- **WHEN** `tai forget` is invoked with no `--repo`/`--pr`/`--branch`/`--comment`/`--batch`
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

#### Scenario: Two selectors

- **WHEN** `tai forget --pr 142 --branch feat/x` is invoked
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

#### Scenario: --repo outside any git repo

- **WHEN** `tai forget --repo acme/app --yes` is invoked from `/tmp`
- **THEN** the CLI succeeds (`--repo` mode does not require git context)

#### Scenario: Prune completed comments under a PR

- **WHEN** `tai forget --pr 142 --status completed --yes` is invoked
- **THEN** every comment under PR 142 with status `completed` is deleted
- **AND** comments with other statuses are preserved
- **AND** the `prs` row for PR 142 is preserved

#### Scenario: Prune completed comments repo-wide

- **WHEN** `tai forget --repo acme/app --status completed --yes` is invoked
- **THEN** every comment across every PR and branch under `acme/app` with status `completed` is deleted
- **AND** the `repos`, `prs`, and `branches` rows are preserved

#### Scenario: --status combined with --comment is rejected

- **WHEN** `tai forget --comment 5 --status completed` is invoked
- **THEN** the CLI exits `1` with `TRIAGE_INVALID_FLAGS`

#### Scenario: Multiple --status values combine via OR

- **WHEN** `tai forget --pr 142 --status completed --status dismissed --yes` is invoked
- **THEN** every comment under PR 142 with status `completed` or `dismissed` is deleted

#### Scenario: --status with --batch deletes matching members and recomputes batch status

- **WHEN** a batch `B1` has 3 completed and 2 accepted members
- **AND** `tai forget --batch B1 --status completed --yes` is invoked
- **THEN** the 3 completed members are deleted
- **AND** the 2 accepted members survive (still members of `B1`)
- **AND** the `B1` row is preserved with its status recomputed (`accepted`, since all surviving members are accepted)
