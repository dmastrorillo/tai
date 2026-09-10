## ADDED Requirements

### Requirement: Board offer between investigation and the triage loop

After phase 1 (investigation) completes and before entering the phase-2 loop, the slash command SHALL count the comments still `pending` in the resolved scope. When that count is greater than five, the slash command SHALL offer the board, naming the count.

The offer is an offer. The slash command SHALL NOT launch `tai triage board` unless the user accepts it, and SHALL NOT offer it at all when five or fewer comments remain pending. A declined offer SHALL be followed by the phase-2 loop exactly as though the board did not exist.

When the user accepts, the slash command SHALL launch `tai triage board` with the scope flags resolved in phase 1, as a background process, and surface the board URL the command prints. It SHALL then wait for the board process to exit, which the board does on submit. Where the agent harness does not notify on background-process exit, the slash command SHALL instead poll `tai triage board status` for the same scope until it reports that intents exist.

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
- **THEN** the slash command launches `tai triage board --pr 142` in the background
- **AND** surfaces the printed board URL
- **AND** waits for the process to exit before reading intents

### Requirement: Intents seed the triage loop and carry no special obligations

When an intents artifact exists for the resolved scope, the slash command SHALL read it via `tai triage board intents` and apply each entry as though the user had typed that answer at that comment's decision prompt in the phase-2 loop.

Every obligation the slash command already carries applies to an intent unchanged. In particular:

- An `accept` intent is persisted via `tai accept <id>`, with any note captured as `--resolution`. Accepts MAY be issued as one batched pass rather than one round trip per comment, followed by a single progress line.
- A `dismiss` intent enters the dismissal-debate contract at the severity calibration that contract already specifies. A `critical` dismissal receives the full contract including a concrete scenario-based challenge; a `minor` or `nitpick` dismissal is not debated. A dismissal carrying no note is reasoning-free and SHALL be pushed back on before it is persisted.
- The `--reason` persisted for a debated dismissal SHALL reflect the debate's outcome, not the board note. The note is the user's opening argument; where the debate produces different reasoning, the reasoning is what is recorded.
- An `unanswered` intent means the comment is walked in the phase-2 loop exactly as it would be with no board at all. Where it carries a note, the slash command SHALL surface and address the note as part of presenting that comment.

Comments absent from the artifact SHALL be treated as `unanswered`.

The slash command SHALL NOT introduce any obligation that applies to intents and not to typed answers, or any exemption that applies to intents and not to typed answers.

#### Scenario: Accept intents are persisted without debate

- **GIVEN** intents carrying thirty-seven `accept` entries
- **THEN** the slash command persists all thirty-seven via `tai accept`
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

### Requirement: Batch-override convention

When the user names a subset of a batch's members in their decision response, the slash command SHALL split the decision:

1. Apply the batch-wide call first via `tai accept --batch <key>` / `tai dismiss --batch <key> --reason …` / `tai complete --batch <key>`.
2. Then issue per-member overrides for the named exceptions via `tai accept <id>` / `tai dismiss <id> --reason …` / `tai complete <id>`.

Before issuing the calls, the slash command MUST confirm the split with the user in plain language (`Got it — accepting B1 (B1.1, B1.2, B1.3, B1.5) and dismissing B1.4 with reason "<text>". Sound right?`).

The confirmation defends against the slash command misreading a split expressed in prose, where a misread is silent and persistent. It therefore applies to splits the slash command derives from a conversational response, and does NOT apply to a split that arrives already stated as per-member intents in an intents artifact — those carry no reading for the slash command to get wrong. A split sourced from intents SHALL be applied without a confirmation prompt.

The resulting batch status will be `mixed` in both cases. This is the intended outcome.

#### Scenario: Batch override applied after batch-wide call

- **WHEN** the user accepts B1 except B1.4 (a dismissal)
- **THEN** the slash command calls `tai accept --batch B1` followed by `tai dismiss 4 --reason …`

#### Scenario: Override confirmation precedes the calls

- **WHEN** the user proposes a batch override in conversation
- **THEN** the slash command surfaces the proposed split in plain language and waits for confirmation before invoking `tai`

#### Scenario: Intent-sourced split needs no confirmation

- **GIVEN** an intents artifact carrying `accept` on four members of B1 and `dismiss` on B1.4
- **THEN** the slash command applies the batch-wide call followed by the per-member override
- **AND** does not prompt for confirmation of the split
- **AND** the batch lands `mixed`
