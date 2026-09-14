## ADDED Requirements

### Requirement: Board offer between investigation and the triage loop

After phase 1 (investigation) completes and before entering the phase-2 loop, the slash command SHALL count the comments still `pending` in the resolved scope. When that count is greater than five, the slash command SHALL offer the board, naming the count.

The offer is an offer. The slash command SHALL NOT launch `tai triage board` unless the user accepts it, and SHALL NOT offer it at all when five or fewer comments remain pending. A declined offer SHALL be followed by the phase-2 loop exactly as though the board did not exist.

When the user accepts, the slash command SHALL carry out phase 2's step-1 investigation for every pending comment in the scope — the same obligation it already carries when presenting a comment in conversation — assemble the results into the briefing the `triage-board` capability defines, and launch `tai triage board -` with the scope flags resolved in phase 1, as a background process with that briefing on stdin, surfacing the board URL the command prints. It SHALL then wait for the board process to exit, which the board does on submit. Where the agent harness does not notify on background-process exit, the slash command SHALL instead poll `tai triage board intents` for the same scope until it emits intents. A poll MUST recognise the not-yet state by the `TRIAGE_NO_INTENTS` code in the error footer, not by the exit code alone: exit `2` is shared with `TRIAGE_NOT_FOUND`, `TRIAGE_NO_SCOPE` and `TRIAGE_AMBIGUOUS_SCOPE`, so a poller branching on the number would read a scope error as "keep waiting" forever.

The count is evaluated after investigation so that a scope in which most comments were auto-completed does not produce an offer for the handful that survive.

#### Scenario: Offer above the threshold

- **GIVEN** investigation leaves fourteen comments pending
- **THEN** the slash command offers the board, naming the count
- **AND** does not launch it before the user accepts

#### Scenario: No offer at or below the threshold

- **GIVEN** investigation leaves five comments pending
- **THEN** the slash command does not mention the board
- **AND** enters the phase-2 loop

#### Scenario: Declined offer leaves the flow unchanged

- **GIVEN** the board was offered and the user declined
- **THEN** the slash command enters the phase-2 loop with every comment still pending
- **AND** makes no further mention of the board

#### Scenario: Board launched and awaited

- **GIVEN** the user accepted the offer for PR 142
- **THEN** the slash command investigates every pending comment and pipes the briefing to `tai triage board - --pr 142` in the background
- **AND** surfaces the printed board URL
- **AND** waits for the process to exit before reading intents

### Requirement: Intents seed the triage loop and carry no special obligations

When an intents artifact exists for the resolved scope, the slash command SHALL read it via `tai triage board intents` and apply each entry as though the user had typed that answer at that comment's decision prompt in the phase-2 loop.

Every obligation the slash command already carries applies to an intent unchanged. In particular:

- An `accept` intent is persisted via `tai triage accept <id>`, with any note captured as `--resolution`. A run of them MAY be persisted as one batched pass under a single progress line, per the `Phase 2 — triage loop, batches first, severity-ordered` requirement.
- A `dismiss` intent enters the dismissal-debate contract at the severity calibration that contract already specifies. A `critical` dismissal receives the full contract including a concrete scenario-based challenge; a `minor` or `nitpick` dismissal is not debated. A dismissal carrying no note is reasoning-free and SHALL be pushed back on before it is persisted.
- The `--reason` persisted for a debated dismissal SHALL reflect the debate's outcome, not the board note. The note is the user's opening argument; where the debate produces different reasoning, the reasoning is what is recorded.
- An `unanswered` intent means the comment is walked in the phase-2 loop exactly as it would be with no board at all. Where it carries a note, the slash command SHALL surface and address the note as part of presenting that comment.

Comments absent from the artifact SHALL be treated as `unanswered`.

The slash command SHALL NOT introduce any obligation that applies to intents and not to typed answers, or any exemption that applies to intents and not to typed answers.

#### Scenario: Accept intents are persisted without debate

- **GIVEN** intents carrying thirty-seven `accept` entries
- **THEN** the slash command persists all thirty-seven via `tai triage accept`
- **AND** does not open a conversation about any of them

#### Scenario: A bulk-dismissed critical still gets the full debate

- **GIVEN** an intent `{intent: "dismiss", note: "nobody can reach that code path"}` on a `critical` comment
- **THEN** the slash command issues a concrete scenario-based challenge before persisting
- **AND** the persisted `--reason` reflects the outcome of that exchange

#### Scenario: A bulk-dismissed nitpick is not debated

- **GIVEN** an intent `{intent: "dismiss", note: "house style"}` on a `nitpick` comment
- **THEN** the slash command acknowledges in one sentence and persists with the note as the reason

#### Scenario: A dismissal with no note is challenged

- **GIVEN** an intent `{intent: "dismiss"}` with no note on a `major` comment
- **THEN** the slash command asks why before persisting

#### Scenario: An unanswered note is addressed in the loop

- **GIVEN** an intent `{intent: "unanswered", note: "explain this one to me"}`
- **THEN** the comment is presented in the phase-2 loop in its normal ordered position
- **AND** the slash command addresses the note as part of presenting it

#### Scenario: Comments absent from the artifact are unanswered

- **GIVEN** a scope with twelve pending comments and an artifact carrying entries for nine of them
- **THEN** the remaining three are walked in the phase-2 loop

#### Scenario: No artifact leaves the loop unchanged

- **GIVEN** `tai triage board intents` exits with `TRIAGE_NO_INTENTS`
- **THEN** the slash command enters the phase-2 loop with every comment pending
- **AND** surfaces no error to the user

## MODIFIED Requirements

### Requirement: Bundled slash command exists and is placed by the plugin host

The system SHALL bundle a slash command at `plugins/triage/assets/commands/triage.md`. The plugin host copies it verbatim into `<target.commands>/tai-triage/triage.md` on install and update; the plugin does not place it itself and MUST NOT write into a target directory from its own subcommands.

The frontmatter MUST include:

| Field | Value |
|---|---|
| `name` | `"TAI: Triage"` |
| `description` | `"Walk through pending PR review comments interactively, batches-first."` |
| `category` | `"Workflow"` |
| `tags` | `[tai, triage, review]` |
| `version` | integer, bumped when the body's contract changes |

The body SHALL address itself by its installed slash-command name, `/tai-triage:triage`.

No `content_hash` field and no ledger file accompany the command. The host owns placement and replaces a plugin's namespace wholesale on update, so there is nothing for a per-command hash to arbitrate.

#### Scenario: Command is placed under the plugin's namespace

- **WHEN** the triage plugin is installed
- **THEN** the command is written to `<target.commands>/tai-triage/triage.md`
- **AND** its frontmatter carries `name: "TAI: Triage"`

#### Scenario: Body addresses its installed name

- **WHEN** the bundled body refers to itself or its sibling commands
- **THEN** it uses the `/tai-triage:` prefix

### Requirement: Scope resolution via the CLI

The slash command body SHALL resolve operating scope by invoking `tai triage status` (with any user-supplied `--pr` / `--branch` flag passed through) BEFORE entering the triage loop. The slash command MUST NOT implement its own scope detection.

If `tai triage status` exits `2` with `TRIAGE_NO_SCOPE`, the slash command MUST surface a message instructing the user to run `/tai-triage:import` first or to re-invoke with `--pr <N>` / `--branch <name>`, then exit the conversation without further action.

If `tai triage status` exits `2` with `TRIAGE_AMBIGUOUS_SCOPE`, the slash command MUST ask the user to disambiguate by re-invoking with `--pr` or `--branch`, then exit the conversation.

If `tai triage status` succeeds but reports zero pending comments, the slash command MUST announce that all comments are triaged, surface a brief recap (counts of accepted/completed/dismissed), and exit the conversation without entering the loop.

#### Scenario: TRIAGE_NO_SCOPE handled

- **WHEN** the user invokes `/tai-triage:triage` on a branch with no associated PR row and no branch row
- **THEN** the slash command body instructs the user to run `/tai-triage:import` or pass `--pr`/`--branch`
- **AND** does not invoke any further `tai triage` verbs

#### Scenario: Empty scope produces a recap without entering the loop

- **WHEN** the user invokes `/tai-triage:triage` in a scope with zero `pending` comments
- **THEN** the slash command body announces completion
- **AND** does not prompt for any decisions

### Requirement: Invocation forms

The slash command SHALL accept exactly these invocation forms:

- `/tai-triage:triage` — auto-detect current scope.
- `/tai-triage:triage --pr <number-or-url>` — single PR by number or full URL.
- `/tai-triage:triage --branch <name>` — branch-scoped review.
- `/tai-triage:triage stack` — every PR from trunk to the current branch, ancestor-first.

Any other argument shape MUST be surfaced as a usage error in the conversation, listing the four supported forms.

#### Scenario: stack mode requires gh or staccato

- **WHEN** the user invokes `/tai-triage:triage stack` and neither `gh` nor staccato MCP is available
- **THEN** the slash command body announces the missing dependency and exits without entering the loop

### Requirement: Phase 1 — investigation before triage

Before entering the per-comment decision loop, the slash command SHALL walk every `pending` comment in scope and look for evidence the comment has already been addressed.

Evidence sources (in order of trust):

1. The file referenced by `comments.file` no longer exists or has been renamed away.
2. The exact code snippet flagged in the comment's `description` is no longer present near `comments.lines`.
3. The pattern described in the comment's `suggested_fix` is now observable in the file.
4. Recent git history (`git log --oneline -10 -- <file>`) shows a change whose subject contains keywords matching the comment's `title`.

When evidence is found, the slash command MUST:

- Call `tai triage complete <id> --resolution "<one-line description of the evidence found>"`.
- Inform the user what was found and offer an override (`I think this is still an issue, please don't mark it completed`).

The slash command MUST be conservative: when evidence is ambiguous or relies on heuristics alone, it MUST NOT call `tai triage complete`. The comment proceeds to the decision loop and the user decides.

This phase asks only whether a comment is already addressed. The per-comment investigation that produces the seven presentation fields belongs to phase 2 step 1 and runs for the comments that survive this one.

#### Scenario: Already-fixed comment auto-completed

- **WHEN** the slash command finds the comment's flagged code is no longer present
- **AND** the suggested fix pattern is observable in the file
- **THEN** the slash command runs `tai triage complete <id> --resolution "…"`
- **AND** tells the user what was found

#### Scenario: User overrides an auto-completion

- **WHEN** the slash command marks a comment completed during investigation
- **AND** the user replies that the fix is incorrect
- **THEN** the slash command runs `tai triage accept <id>` (reverting to active triage with the user's input)

### Requirement: Phase 2 — triage loop, batches first, severity-ordered

The slash command SHALL present items in this order:

1. Batches, ordered by the highest-severity member's severity (`critical → major → minor → nitpick`). Ties broken by `batch_key` ascending.
2. Non-batched comments, ordered by severity (same hierarchy), ties broken by `file` ascending then `lines` ascending.

Within each item:

1. Read the stored record with `tai triage show` (one per batch member when presenting a batch; one for individual comments), then open the file at the flagged lines and read the surrounding code. Present what the investigation found under seven fields: who raised it, file:line, description, cause, why fix it, suggested fix, and concerns if skipped. `cause` is always derived from the code read in this step and never taken from the record. The stored markdown MUST NOT be surfaced verbatim.
2. Ask `Accept or dismiss? Any thoughts on the fix?`.
3. Persist the decision via `tai triage accept` / `tai triage dismiss --reason …` / `tai triage complete --resolution …`.
4. After each decision, run `tai triage status` and surface a `[X/Y] …` progress line.

The slash command MUST process items strictly in the order above. It MUST NOT skip an item or change order based on conversation drift.

A run of `accept` intents read from an intents artifact is one decision for this purpose, not one per comment: the developer made those calls on the board, and the loop is persisting them rather than asking for them. The slash command MAY persist them as a single batched pass and surface one progress line covering the run. Every other decision, including any dismissal, keeps its own progress line.

#### Scenario: Batches presented before individuals

- **WHEN** a scope has one batch and three non-batched comments
- **THEN** the slash command presents the batch before any non-batched comment

#### Scenario: Severity order within batches

- **WHEN** a scope has two batches, one with a critical-severity member and one whose highest severity is major
- **THEN** the slash command presents the critical batch first

#### Scenario: Decision triggers CLI call

- **WHEN** the user replies "accept" to an individual comment prompt
- **THEN** the slash command invokes `tai triage accept <id>` (with `--resolution` if a fix proposal was captured)

#### Scenario: An item is presented as an investigation, not a stored record

- **WHEN** the slash command presents a comment in the loop
- **THEN** it surfaces the seven presentation fields
- **AND** it does not surface `tai triage show`'s markdown verbatim

#### Scenario: A run of accept intents surfaces one progress line

- **GIVEN** an intents artifact carrying thirty-seven `accept` entries
- **THEN** the slash command surfaces one progress line for the run, not thirty-seven

### Requirement: Dismissal-debate contract

When the user expresses a dismiss intent, the slash command SHALL engage in a debate calibrated to the comment's severity:

- For `critical` severity: challenge with at least one concrete, scenario-based question before accepting the dismissal. If the user's response contains assumptions treated as facts, scope dismissal, effort bias, or anchoring, continue the debate. Halt when the user's reasoning withstands a concrete scenario OR when the user explicitly insists.
- For `major` severity: challenge once with a concrete scenario; accept the user's response unless it has an evident gap.
- For `minor` / `nitpick` severity: do not debate. Accept the dismissal after a one-sentence acknowledgement.

The slash command MUST record the dismissal via `tai triage dismiss <id> --reason "<reasoning produced by the conversation, not the user's first sentence>"`. The reason captures the conversation's outcome — including any agreed-upon counter-evidence — not just the user's opening line.

The slash command MUST NOT:

- Use "are you sure?" as a substitute for a concrete challenge.
- Argue past a debate's natural conclusion to demonstrate thoroughness.
- Accept "I don't want to fix that" as a complete reason for a critical-severity dismissal.

#### Scenario: Critical-severity dismissal triggers debate

- **WHEN** the user wants to dismiss a `critical` comment
- **THEN** the slash command poses a concrete scenario challenging the dismissal before persisting it

#### Scenario: Nitpick-severity dismissal does not debate

- **WHEN** the user wants to dismiss a `nitpick` comment
- **THEN** the slash command acknowledges and persists via `tai triage dismiss --reason …` without further challenge

#### Scenario: Dismissal reason reflects the conversation

- **WHEN** a user dismisses a comment after a multi-round debate
- **THEN** the `--reason` passed to `tai triage dismiss` describes the conclusion reached, not just the user's opening line

### Requirement: Batch-override convention

When the user names a subset of a batch's members in their decision response, the slash command SHALL split the decision:

1. Apply the batch-wide call first via `tai triage accept --batch <key>` / `tai triage dismiss --batch <key> --reason …` / `tai triage complete --batch <key>`.
2. Then issue per-member overrides for the named exceptions via `tai triage accept <id>` / `tai triage dismiss <id> --reason …` / `tai triage complete <id>`.

Before issuing the calls, the slash command MUST confirm the split with the user in plain language (`Got it — accepting B1 (B1.1, B1.2, B1.3, B1.5) and dismissing B1.4 with reason "<text>". Sound right?`).

The confirmation defends against the slash command misreading a split expressed in prose, where a misread is silent and persistent. It therefore applies to splits the slash command derives from a conversational response, and does NOT apply to a split that arrives already stated as per-member intents in an intents artifact — those carry no reading for the slash command to get wrong. A split sourced from intents SHALL be applied without a confirmation prompt.

The two-step sequence above is likewise a rule about a conversational split. An intents artifact records a batch-level call only as the per-member intents it produced, so which member call was "the batch-wide one" is not recoverable from it and nothing turns on recovering it: the `Batch status recomputation` requirement derives a batch's status from its members after every transition, so issuing the batch-wide call first and issuing each member call directly reach the same persisted state. For a split sourced from intents the slash command SHALL therefore be judged on the resulting member states and batch status, and MAY reach them by whichever calls it prefers. Using `--batch` for the majority intent remains the cheaper path and stays available.

The resulting batch status will be `mixed` in both cases. This is the intended outcome.

#### Scenario: Batch override applied after batch-wide call

- **WHEN** the user accepts B1 except B1.4 (a dismissal)
- **THEN** the slash command calls `tai triage accept --batch B1` followed by `tai triage dismiss 4 --reason …`

#### Scenario: Override confirmation precedes the calls

- **WHEN** the user proposes a batch override in conversation
- **THEN** the slash command surfaces the proposed split in plain language and waits for confirmation before invoking `tai`

#### Scenario: Intent-sourced split needs no confirmation

- **GIVEN** an intents artifact carrying `accept` on four members of B1 and `dismiss` on B1.4
- **THEN** those four members end `accepted` and B1.4 ends `dismissed` carrying its note as the reason
- **AND** B1's status is `mixed`
- **AND** the slash command does not prompt for confirmation of the split

### Requirement: Recap at end of loop

When `tai triage status` reports zero `pending` comments in the scope, the slash command SHALL emit a recap containing:

1. A header line naming the scope (e.g. `Triage complete for acme/app PR #142.`).
2. Counts: accepted (with parenthesised batch count when batches accepted), completed, dismissed.
3. The accepted work queue: every accepted comment in severity order, each row showing `[<sev-abbr>] <id>: <title> (<file>:<lines>)`, where `<id>` is the integer position `tai triage list` prints.
4. A closing line pointing at the next command: `/tai-triage:fix` to work through the accepted queue, then `/tai-triage:verify` to confirm it.

The recap MUST be emitted exactly once per loop completion.

For `stack` mode, the recap is per-PR with a final stack-level aggregate after the last PR.

#### Scenario: Recap surfaces accepted work queue

- **WHEN** triage completes for a scope with three accepted comments
- **THEN** the recap lists each accepted comment in severity order with its file and line range

#### Scenario: Recap points at the next command

- **WHEN** the recap is emitted
- **THEN** its closing line names `/tai-triage:fix` and `/tai-triage:verify`

#### Scenario: Stack mode recaps per PR and at the end

- **WHEN** `/tai-triage:triage stack` completes for a stack of three PRs
- **THEN** three per-PR recaps appear in conversation
- **AND** a final stack-level aggregate count appears after the last PR's recap

### Requirement: Slash command persistence is via the CLI only

The slash command body SHALL NOT write to the database directly. It SHALL NOT bypass `tai triage accept` / `tai triage dismiss` / `tai triage complete` by editing files under the data directory.

#### Scenario: All state changes route through tai triage verbs

- **WHEN** the user makes any decision during the loop
- **THEN** the resulting state change is observable as a `tai triage accept`/`tai triage dismiss`/`tai triage complete` invocation
- **AND** no direct SQLite writes occur from the slash command
