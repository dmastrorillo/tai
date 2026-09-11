## ADDED Requirements

### Requirement: `tai triage board` serves a loopback board and blocks until submit

The system SHALL provide a `tai triage board` subcommand that resolves a scope using the `triage` capability's scope-resolution rule (`--pr` > `--branch` > auto-detect), serves an HTML board for that scope over HTTP, and blocks until the developer submits.

The listener SHALL bind `127.0.0.1` on port `0` (kernel-assigned). The board SHALL be served under a path prefix containing 32 hexadecimal characters drawn from a cryptographically secure random source, generated fresh on every invocation. Any request whose path does not carry the current prefix SHALL receive `404` with no body content derived from the database.

On start the command SHALL write the full board URL to stdout, then attempt to open the developer's browser at that URL. A failed browser launch is NOT an error: the command SHALL continue serving and the printed URL remains the developer's entry point.

On submit the command SHALL write the intents artifact for the scope, write a one-line confirmation to stdout, and exit `0`. If the listener cannot bind, the command SHALL exit with `TRIAGE_BOARD_UNAVAILABLE`.

The command SHALL NOT write to the database under any circumstances.

#### Scenario: Board binds loopback on an ephemeral port

- **WHEN** `tai triage board --pr 142` is invoked in a scope with pending comments
- **THEN** a listener is bound on `127.0.0.1` with a kernel-assigned port
- **AND** stdout contains the full board URL including the random path prefix

#### Scenario: Request without the path prefix is refused

- **GIVEN** a running board served under `/b/<prefix>/`
- **WHEN** a request arrives for `/` or for `/b/<other-prefix>/`
- **THEN** the response status is `404`
- **AND** the response body contains no comment content

#### Scenario: Browser launch failure is not fatal

- **GIVEN** a machine on which no browser can be launched
- **WHEN** `tai triage board` is invoked
- **THEN** the command continues serving
- **AND** stdout still carries the board URL
- **AND** the command does not exit

#### Scenario: Submit writes intents and exits zero

- **GIVEN** a running board
- **WHEN** the developer submits the board
- **THEN** the intents artifact for the scope is written
- **AND** the command exits `0`
- **AND** no comment's `status` in the database has changed

#### Scenario: Listener cannot bind

- **WHEN** `tai triage board` is invoked and no loopback listener can be bound
- **THEN** the CLI exits with `TRIAGE_BOARD_UNAVAILABLE`
- **AND** no intents artifact is written

### Requirement: Board renders every pending comment in the scope

The board SHALL render every comment in the resolved scope whose `status` is `pending`, and no comment of any other status.

Each comment SHALL display its position ID, severity, category, file, lines, source, title, description, why-fix, suggested fix, and consequences — the same fields `tai triage show` renders, so that the board and the conversation never describe the same comment differently.

Comments belonging to a batch SHALL be rendered grouped under their batch, labelled with the batch key and title. A batch SHALL carry a batch-level control that sets the same intent on every member in one action, and each member SHALL additionally carry its own control so a member can be given a different intent from the rest of its batch.

Batches SHALL be presented before non-batched comments, batches ordered by their highest-severity member and ties broken by batch key ascending, non-batched comments ordered by severity and ties broken by file then lines ascending — the ordering the `triage-command` capability mandates for the conversational loop.

When the scope has no pending comments, the board SHALL render a message saying so and offer submit of an empty intent set.

#### Scenario: Every field `tai triage show` renders is present

- **GIVEN** a scope with one pending comment whose ten display fields are all populated
- **WHEN** the board is rendered
- **THEN** the served HTML contains each of severity, category, file, lines, source, title, description, why-fix, suggested fix and consequences for that comment

#### Scenario: A note input is present for every comment and every batch

- **GIVEN** a scope with a three-member batch and two non-batched comments
- **WHEN** the board is rendered
- **THEN** the served HTML carries a note input for each of the five comments
- **AND** a note input for the batch

#### Scenario: Empty scope

- **GIVEN** a scope with no pending comments
- **WHEN** the board is rendered
- **THEN** the served HTML says so
- **AND** submitting it writes an intents artifact carrying no entries

#### Scenario: Only pending comments are rendered

- **GIVEN** a scope with three `pending`, two `accepted`, and one `completed` comment
- **WHEN** the board is rendered
- **THEN** exactly the three `pending` comments appear

#### Scenario: Batch members are grouped with a batch-level control

- **GIVEN** a scope with a five-member batch `B1`
- **WHEN** the board is rendered
- **THEN** the five members appear grouped under `B1` with its key and title
- **AND** a batch-level control is present that sets one intent on all five
- **AND** each member carries its own control

#### Scenario: Batches precede non-batched comments

- **GIVEN** a scope with one batch whose highest-severity member is `major`, and a non-batched `critical` comment
- **WHEN** the board is rendered
- **THEN** the batch appears before the non-batched comment

### Requirement: Intents artifact

The board SHALL record the developer's calls as an intents artifact at
`<TAI_DATA_DIR>/plugins/triage/state/intents/<owner>--<name>--<scope>.json`,
where `<scope>` is `pr-<number>` for a PR scope and `branch-<slug>` for a branch scope. The artifact SHALL be overwritten in full on each submit for the same scope.

The artifact SHALL record the repo, the scope, the submit timestamp in RFC 3339, and one entry per comment carrying the comment's position ID, its intent, and its note.

An intent SHALL be exactly one of `accept`, `dismiss`, or `unanswered`. `unanswered` covers every comment the developer did not decide, whether they deliberately skipped it or never reached it. Any intent MAY carry a free-text note; a note is never required and its absence is not an error.

A batch-level call SHALL be recorded as the resulting per-member intents, not as a batch-level entry, so that a batch decided with exceptions is represented exactly as the per-member calls that produced it.

The artifact is written only by `tai triage board` and read only by `tai triage board intents`. No other component reads it.

#### Scenario: Artifact records all three intents with notes

- **WHEN** the developer accepts comment 1, dismisses comment 2 with the note `covered by the sandbox`, and leaves comment 3 alone
- **AND** submits
- **THEN** the artifact carries `{id: 1, intent: "accept"}`, `{id: 2, intent: "dismiss", note: "covered by the sandbox"}`, and `{id: 3, intent: "unanswered"}`

#### Scenario: A note on an unanswered comment is preserved

- **WHEN** the developer leaves comment 4 undecided but writes the note `explain this one to me`
- **THEN** the artifact carries `{id: 4, intent: "unanswered", note: "explain this one to me"}`

#### Scenario: Batch call expands to per-member intents

- **GIVEN** a five-member batch `B1`
- **WHEN** the developer sets the batch-level control to `accept` and then sets member `B1.4` to `dismiss` with a note
- **THEN** the artifact carries four `accept` entries and one `dismiss` entry carrying the note
- **AND** carries no batch-level entry

#### Scenario: Resubmitting the same scope replaces the artifact

- **GIVEN** an intents artifact already exists for PR 142
- **WHEN** a second board for PR 142 is submitted
- **THEN** the artifact contains only the second submission's entries

#### Scenario: Artifacts are scope-keyed

- **GIVEN** intents artifacts exist for PR 142 and PR 200 in the same repo
- **THEN** each is stored at its own path
- **AND** reading intents for PR 142 never returns PR 200's entries

### Requirement: `tai triage board intents` emits the artifact for the AI

The system SHALL provide a `tai triage board intents` subcommand that resolves a scope using the same rule as every other triage verb, reads that scope's intents artifact, and writes it to stdout as markdown.

The output SHALL carry the submit timestamp, and one line per entry giving the comment's position ID, its intent, and its note. Entries SHALL be emitted in the board's presentation order.

When no artifact exists for the resolved scope, the CLI SHALL exit with `TRIAGE_NO_INTENTS`.

The subcommand SHALL NOT write to the database and SHALL NOT modify or delete the artifact.

#### Scenario: Intents are emitted as markdown

- **GIVEN** an intents artifact for PR 142 with three entries
- **WHEN** `tai triage board intents --pr 142` is invoked
- **THEN** stdout carries the submit timestamp and one line per entry with ID, intent, and note
- **AND** the CLI exits `0`

#### Scenario: No artifact for the scope

- **WHEN** `tai triage board intents --pr 999` is invoked and no artifact exists for that scope
- **THEN** the CLI exits with `TRIAGE_NO_INTENTS`

#### Scenario: Reading intents is not destructive

- **WHEN** `tai triage board intents --pr 142` is invoked twice
- **THEN** both invocations produce identical output
- **AND** the artifact still exists after both

### Requirement: Board-layer error codes

The system SHALL register two error codes in the append-only `pkg/errcode` registry:

- `TRIAGE_BOARD_UNAVAILABLE` — the board could not bind a loopback listener. Exit code `3`, joining the other codes for an environment that blocked the operation (`DATA_DIR_UNWRITABLE`, `CONFIG_UNWRITABLE`, `REPO_FETCH_FAILED`, `PLUGIN_FETCH_FAILED`, `INSTALL_TARGET_UNWRITABLE`). Exit code `1` is reserved for an unknown subcommand, a malformed flag, or conflicting options, none of which describes a bind failure.
- `TRIAGE_NO_INTENTS` — an intents artifact was requested for a scope that has none. Exit code `2`, joining the other "the thing you named does not exist" triage codes.

Both SHALL render through the foundation error template with the `[exit N: ERROR_CODE]` footer.

#### Scenario: Codes render through the foundation template

- **WHEN** either code is surfaced
- **THEN** stderr carries the foundation error template
- **AND** the footer reads `[exit 3: TRIAGE_BOARD_UNAVAILABLE]` or `[exit 2: TRIAGE_NO_INTENTS]` respectively
