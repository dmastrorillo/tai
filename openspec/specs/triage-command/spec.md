# triage-command Specification

## Purpose

The `triage-command` capability defines the bundled `/tai:triage` Claude slash command: its invocation forms, the four-phase loop (scope resolution, investigation, decision loop, recap), the dismissal-debate contract, the batch-override convention, the frontmatter / ledger requirements that make it installable by `tai install`, and the rule that all state changes route through the `tai` CLI verbs rather than direct database writes. The CLI verbs introduced by `triage` (`tai list`, `tai show`, `tai accept`, `tai dismiss`, `tai complete`, `tai status`, `tai forget`) are the keyboard; this slash command is the pianist — everything subjective (severity weighting in presentation order, the sparring debate when a user dismisses, cognitive-bias checks, per-batch override semantics in conversation) lives here, not in the CLI.

## Requirements

### Requirement: Bundled slash command exists and obeys the command-framework

The system SHALL bundle a slash command at `commands/triage.md` whose frontmatter conforms to the `command-framework` schema and whose body is installed at `~/.claude/commands/tai/triage.md` by `tai install`.

The frontmatter MUST include:

| Field | Value |
|---|---|
| `name` | `"TAI: Triage"` |
| `description` | `"Walk through pending PR review comments interactively, batches-first."` |
| `category` | `"Workflow"` |
| `tags` | `[tai, triage, review]` |
| `version` | `1` (initial integer; bumps require a follow-on proposal) |
| `content_hash` | `sha256:<hex>` computed over the body |

A corresponding `commands/triage.ledger.json` MUST exist with at least one entry — the current `content_hash`.

#### Scenario: Slash command bundled and installable

- **WHEN** `tai install` is invoked after this proposal lands
- **THEN** a file is written at `~/.claude/commands/tai/triage.md`
- **AND** that file's frontmatter contains `name: "TAI: Triage"`, `version: 1`, and a valid `content_hash`

#### Scenario: Ledger contains current hash

- **WHEN** the binary is built
- **THEN** `commands/triage.ledger.json` contains the body hash of the shipped `commands/triage.md` as its last entry

### Requirement: Scope resolution via the CLI

The slash command body SHALL resolve operating scope by invoking `tai status` (with any user-supplied `--pr` / `--branch` flag passed through) BEFORE entering the triage loop. The slash command MUST NOT implement its own scope detection.

If `tai status` exits `2` with `TRIAGE_NO_SCOPE`, the slash command MUST surface a message instructing the user to run `/tai:import` first or to re-invoke with `--pr <N>` / `--branch <name>`, then exit the conversation without further action.

If `tai status` exits `2` with `TRIAGE_AMBIGUOUS_SCOPE`, the slash command MUST ask the user to disambiguate by re-invoking with `--pr` or `--branch`, then exit the conversation.

If `tai status` succeeds but reports zero pending comments, the slash command MUST announce that all comments are triaged, surface a brief recap (counts of accepted/completed/dismissed), and exit the conversation without entering the loop.

#### Scenario: TRIAGE_NO_SCOPE handled

- **WHEN** the user invokes `/tai:triage` on a branch with no associated PR row and no branch row
- **THEN** the slash command body instructs the user to run `/tai:import` or pass `--pr`/`--branch`
- **AND** does not invoke any further `tai` verbs

#### Scenario: Empty scope produces a recap without entering the loop

- **WHEN** the user invokes `/tai:triage` in a scope with zero `pending` comments
- **THEN** the slash command body announces completion
- **AND** does not prompt for any decisions

### Requirement: Invocation forms

The slash command SHALL accept exactly these invocation forms:

- `/tai:triage` — auto-detect current scope.
- `/tai:triage --pr <number-or-url>` — single PR by number or full URL.
- `/tai:triage --branch <name>` — branch-scoped review.
- `/tai:triage stack` — every PR from trunk to the current branch, ancestor-first.

Any other argument shape MUST be surfaced as a usage error in the conversation, listing the four supported forms.

#### Scenario: stack mode requires gh or staccato

- **WHEN** the user invokes `/tai:triage stack` and neither `gh` nor staccato MCP is available
- **THEN** the slash command body announces the missing dependency and exits without entering the loop

### Requirement: Phase 1 — investigation before triage

Before entering the per-comment decision loop, the slash command SHALL walk every `pending` comment in scope and look for evidence the comment has already been addressed.

Evidence sources (in order of trust):

1. The file referenced by `comments.file` no longer exists or has been renamed away.
2. The exact code snippet flagged in the comment's `description` is no longer present near `comments.lines`.
3. The pattern described in the comment's `suggested_fix` is now observable in the file.
4. Recent git history (`git log --oneline -10 -- <file>`) shows a change whose subject contains keywords matching the comment's `title`.

When evidence is found, the slash command MUST:

- Call `tai complete <id> --resolution "<one-line description of the evidence found>"`.
- Inform the user what was found and offer an override (`I think this is still an issue, please don't mark it completed`).

The slash command MUST be conservative: when evidence is ambiguous or relies on heuristics alone, it MUST NOT call `tai complete`. The comment proceeds to the decision loop and the user decides.

#### Scenario: Already-fixed comment auto-completed

- **WHEN** the slash command finds the comment's flagged code is no longer present
- **AND** the suggested fix pattern is observable in the file
- **THEN** the slash command runs `tai complete <id> --resolution "…"`
- **AND** tells the user what was found

#### Scenario: User overrides an auto-completion

- **WHEN** the slash command marks a comment completed during investigation
- **AND** the user replies that the fix is incorrect
- **THEN** the slash command runs `tai accept <id>` (reverting to active triage with the user's input)

### Requirement: Phase 2 — triage loop, batches first, severity-ordered

The slash command SHALL present items in this order:

1. Batches, ordered by the highest-severity member's severity (`critical → major → minor → nitpick`). Ties broken by `batch_key` ascending.
2. Non-batched comments, ordered by severity (same hierarchy), ties broken by `file` ascending then `lines` ascending.

Within each item:

1. Surface the relevant `tai show` output verbatim (one per batch member when presenting a batch; one for individual comments).
2. Ask `Accept, dismiss, or complete? Any thoughts on the fix?`.
3. Persist the decision via `tai accept` / `tai dismiss --reason …` / `tai complete --resolution …`.
4. After each decision, run `tai status` and surface a `[X/Y] …` progress line.

The slash command MUST process items strictly in the order above. It MUST NOT skip an item or change order based on conversation drift.

#### Scenario: Batches presented before individuals

- **WHEN** a scope has one batch and three non-batched comments
- **THEN** the slash command presents the batch before any non-batched comment

#### Scenario: Severity order within batches

- **WHEN** a scope has two batches, one with a critical-severity member and one whose highest severity is major
- **THEN** the slash command presents the critical batch first

#### Scenario: Decision triggers CLI call

- **WHEN** the user replies "accept" to an individual comment prompt
- **THEN** the slash command invokes `tai accept <id>` (with `--resolution` if a fix proposal was captured)

### Requirement: Dismissal-debate contract

When the user expresses a dismiss intent, the slash command SHALL engage in a debate calibrated to the comment's severity:

- For `critical` severity: challenge with at least one concrete, scenario-based question before accepting the dismissal. If the user's response contains assumptions treated as facts, scope dismissal, effort bias, or anchoring, continue the debate. Halt when the user's reasoning withstands a concrete scenario OR when the user explicitly insists.
- For `major` severity: challenge once with a concrete scenario; accept the user's response unless it has an evident gap.
- For `minor` / `nitpick` severity: do not debate. Accept the dismissal after a one-sentence acknowledgement.

The slash command MUST record the dismissal via `tai dismiss <id> --reason "<reasoning produced by the conversation, not the user's first sentence>"`. The reason captures the conversation's outcome — including any agreed-upon counter-evidence — not just the user's opening line.

The slash command MUST NOT:

- Use "are you sure?" as a substitute for a concrete challenge.
- Argue past a debate's natural conclusion to demonstrate thoroughness.
- Accept "I don't want to fix that" as a complete reason for a critical-severity dismissal.

#### Scenario: Critical-severity dismissal triggers debate

- **WHEN** the user wants to dismiss a `critical` comment
- **THEN** the slash command poses a concrete scenario challenging the dismissal before persisting it

#### Scenario: Nitpick-severity dismissal does not debate

- **WHEN** the user wants to dismiss a `nitpick` comment
- **THEN** the slash command acknowledges and persists via `tai dismiss --reason …` without further challenge

#### Scenario: Dismissal reason reflects the conversation

- **WHEN** a user dismisses a comment after a multi-round debate
- **THEN** the `--reason` passed to `tai dismiss` describes the conclusion reached, not just the user's opening line

### Requirement: Batch-override convention

When the user names a subset of a batch's members in their decision response, the slash command SHALL split the decision:

1. Apply the batch-wide call first via `tai accept --batch <key>` / `tai dismiss --batch <key> --reason …` / `tai complete --batch <key>`.
2. Then issue per-member overrides for the named exceptions via `tai accept <id>` / `tai dismiss <id> --reason …` / `tai complete <id>`.

Before issuing the calls, the slash command MUST confirm the split with the user in plain language (`Got it — accepting B1 (B1.1, B1.2, B1.3, B1.5) and dismissing B1.4 with reason "<text>". Sound right?`).

The resulting batch status will be `mixed`. This is the intended outcome.

#### Scenario: Batch override applied after batch-wide call

- **WHEN** the user accepts B1 except B1.4 (a dismissal)
- **THEN** the slash command calls `tai accept --batch B1` followed by `tai dismiss 4 --reason …`

#### Scenario: Override confirmation precedes the calls

- **WHEN** the user proposes a batch override
- **THEN** the slash command surfaces the proposed split in plain language and waits for confirmation before invoking `tai`

### Requirement: Recap at end of loop

When `tai status` reports zero `pending` comments in the scope, the slash command SHALL emit a recap containing:

1. A header line naming the scope (e.g. `Triage complete for acme/app PR #142.`).
2. Counts: accepted (with parenthesised batch count when batches accepted), completed, dismissed.
3. The accepted work queue: every accepted comment in severity order, each row showing `[<sev-abbr>] <id-or-batch-key>: <title> (<file>:<lines>)`.
4. A closing line offering to surface the comments again via `tai show`.

The recap MUST be emitted exactly once per loop completion.

For `stack` mode, the recap is per-PR with a final stack-level aggregate after the last PR.

#### Scenario: Recap surfaces accepted work queue

- **WHEN** triage completes for a scope with three accepted comments
- **THEN** the recap lists each accepted comment in severity order with its file and line range

#### Scenario: Stack mode recaps per PR and at the end

- **WHEN** `/tai:triage stack` completes for a stack of three PRs
- **THEN** three per-PR recaps appear in conversation
- **AND** a final stack-level aggregate count appears after the last PR's recap

### Requirement: Slash command persistence is via the CLI only

The slash command body SHALL NOT write to the database directly. It SHALL NOT bypass `tai accept` / `tai dismiss` / `tai complete` by editing files under the data directory.

#### Scenario: All state changes route through tai verbs

- **WHEN** the user makes any decision during the loop
- **THEN** the resulting state change is observable as a `tai accept`/`tai dismiss`/`tai complete` invocation
- **AND** no direct SQLite writes occur from the slash command
