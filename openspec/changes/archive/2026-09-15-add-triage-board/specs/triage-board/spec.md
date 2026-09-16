## ADDED Requirements

### Requirement: `tai triage board -` reads a briefing from stdin and serves it

The system SHALL provide a `tai triage board` subcommand that accepts a single positional argument `-`, signalling that the briefing JSON is read from stdin. Invoking it with no argument, or with any other positional argument, MUST exit `1` with `UNKNOWN_SUBCOMMAND` — a board without a briefing has nothing to show, so silent help is misleading.

The briefing's `repo` and `scope` name the intents artifact the subcommand writes. `tai triage board` accepts no `--pr` / `--branch` flags: the scope-resolution rule the other triage verbs share reaches for the current git branch and the `prs` / `branches` tables, and this subcommand may do neither. The briefing already carries both values and the validator already checks them.

The listener SHALL bind `127.0.0.1` on port `0` (kernel-assigned). The board SHALL be served under a path prefix containing 32 hexadecimal characters drawn from a cryptographically secure random source, generated fresh on every invocation. Any request whose path does not carry the current prefix SHALL receive `404` with no body content derived from the briefing.

Once the listener is bound the command SHALL hand it to a detached server process, write the full board URL to stdout, attempt to open the developer's browser at that URL, and exit `0`. It SHALL NOT wait for a decision. A failed browser launch is NOT an error: the detached server keeps serving and the printed URL remains the developer's entry point.

The command exits while the board is still serving because a developer works a board for ten to twenty minutes, which outlives the per-command timeout of every harness the slash command targets. A blocking command makes the caller responsible for backgrounding it correctly, and a caller that gets that wrong loses the URL — the one thing it needs — with no error to read. Detaching is the CLI's job, so the caller's invocation is an ordinary foreground pipe whose stdout it can read.

The detached server SHALL serve under the same path prefix the announced URL carries, SHALL write the intents artifact for the scope on submit, and SHALL then exit. Its stdout carries nothing: the announcement was made by the command that spawned it, and the artifact is how the decisions come back. If the listener cannot bind, or the server process cannot be spawned, the command SHALL exit with `TRIAGE_BOARD_UNAVAILABLE` having announced nothing and opened no browser.

Neither the path prefix nor the briefing SHALL be passed to the server process as a command-line argument or an environment variable. Process arguments are readable by every other local user on macOS and most Linux configurations, which is the reason the browser is launched at a secret-free landing path; an inherited pipe is readable by neither.

The subcommand SHALL NOT open the database, and SHALL NOT read from the network or shell out to `gh`, `git`, or any other external command. Everything it renders comes from the briefing on stdin.

#### Scenario: Board binds loopback on an ephemeral port

- **WHEN** `tai triage board -` is invoked with a valid briefing on stdin
- **THEN** a listener is bound on `127.0.0.1` with a kernel-assigned port
- **AND** stdout contains the full board URL including the random path prefix

#### Scenario: Request without the path prefix is refused

- **GIVEN** a running board served under `/b/<prefix>/`
- **WHEN** a request arrives for `/` or for `/b/<other-prefix>/`
- **THEN** the response status is `404`
- **AND** the response body contains no comment content

#### Scenario: Browser launch failure is not fatal

- **GIVEN** a machine on which no browser can be launched
- **WHEN** `tai triage board -` is invoked with a valid briefing
- **THEN** the detached server continues serving
- **AND** stdout still carries the board URL
- **AND** the command exits `0`

#### Scenario: Launch returns without waiting for a decision

- **WHEN** `tai triage board -` is invoked with a valid briefing on stdin
- **THEN** the command exits `0` without any submit having arrived
- **AND** stdout carries the board URL
- **AND** the announced URL is served after the command has exited

#### Scenario: Submit writes intents from the detached server

- **GIVEN** a board whose launching command has already exited
- **WHEN** the developer submits the board
- **THEN** the intents artifact for the scope is written
- **AND** the server process exits
- **AND** no comment's `status` in the database has changed

#### Scenario: A server process that cannot be spawned

- **WHEN** `tai triage board -` is invoked with a valid briefing and the server process cannot be spawned
- **THEN** the CLI exits with `TRIAGE_BOARD_UNAVAILABLE`
- **AND** stdout carries no board URL
- **AND** no intents artifact is written

#### Scenario: Listener cannot bind

- **WHEN** `tai triage board -` is invoked with a valid briefing and no loopback listener can be bound
- **THEN** the CLI exits with `TRIAGE_BOARD_UNAVAILABLE`
- **AND** no intents artifact is written

### Requirement: Briefing JSON schema

The briefing SHALL be a JSON object of this shape. Unknown fields are rejected.

```json
{
  "repo": "<owner>/<name>",
  "scope": { "kind": "pr", "pr": 142 },
  "batches": [
    { "batch_key": "B1", "title": "Replace execSync with execFileSync" }
  ],
  "comments": [
    {
      "id": 7,
      "batch_key": "B1",
      "severity": "critical",
      "raised_by": "coderabbit",
      "location": "src/api/auth.ts:15-29",
      "description": "…",
      "cause": "…",
      "why_fix": "…",
      "suggested_fix": "…",
      "suggested_fix_origin": "reviewer",
      "concerns_if_skipped": "…"
    }
  ]
}
```

`scope.kind` is `pr` or `branch`, carrying `pr` (integer) or `branch` (string) respectively.

Every field on a comment is required and non-empty except `batch_key`, which is omitted for a comment in no batch. `id` is the comment's per-target position. `severity` is one of `critical`, `major`, `minor`, `nitpick` — it drives presentation order and grouping, not display of a stored column. `suggested_fix_origin` is `reviewer` when the fix came from the stored record and `investigation` when the AI derived it, so the developer knows whose proposal they are reading.

The seven presentation fields — `raised_by`, `location`, `description`, `cause`, `why_fix`, `suggested_fix`, `concerns_if_skipped` — are the investigated presentation the `triage-command` capability's phase-2 step 1 produces. They are NOT the stored columns: `cause` is never stored and is always derived by reading the code, and the rest are the record sharpened or replaced by what the investigation found. The board renders what it is given and derives nothing.

`comments` MAY be empty. Every `batch_key` named by a comment MUST appear in `batches`.

#### Scenario: Minimal valid briefing

- **WHEN** a briefing carrying one comment with all seven presentation fields and no `batch_key` is piped to `tai triage board -`
- **THEN** the board serves that comment
- **AND** the CLI does not open the database

#### Scenario: Unknown field rejected

- **WHEN** a briefing carries a comment field not named above
- **THEN** the CLI exits with `TRIAGE_BOARD_SCHEMA_INVALID`

### Requirement: Briefing validation reports every violation at once

Malformed JSON SHALL exit with `TRIAGE_BOARD_INVALID_JSON`. JSON that parses but violates the schema SHALL exit with `TRIAGE_BOARD_SCHEMA_INVALID`.

The system SHALL collect every schema violation before reporting. The error message MUST list each violation on its own line with a JSON-Pointer-style path indicating where in the briefing it occurred, matching the `import` capability's validation contract — the consumer is an AI regenerating the briefing, and one violation per run costs a round trip each time.

No listener SHALL be bound and no browser launched when validation fails.

#### Scenario: Malformed JSON

- **WHEN** the stdin payload is not valid JSON
- **THEN** the CLI exits with `TRIAGE_BOARD_INVALID_JSON`
- **AND** no listener is bound

#### Scenario: Multiple violations reported together

- **WHEN** a briefing has a comment missing `cause`, a comment missing `raised_by`, and a `batch_key` naming a batch absent from `batches`
- **THEN** stderr lists all three, each with its JSON-Pointer path
- **AND** the CLI exits once with `TRIAGE_BOARD_SCHEMA_INVALID`

#### Scenario: Validation precedes the listener

- **WHEN** a briefing fails validation
- **THEN** no listener is bound, no browser is launched, and no intents artifact is written

### Requirement: Board renders the investigated presentation

The board SHALL render every comment in the briefing, and nothing else.

Each comment SHALL display its `id` and severity, and all seven presentation fields: who raised it, the location, the description, the cause, why to fix it, the suggested fix labelled with its origin, and the concerns if skipped.

Comments carrying a `batch_key` SHALL be rendered grouped under that batch, labelled with its key and title. A batch SHALL carry a batch-level control that sets the same intent on every member in one action, and each member SHALL additionally carry its own control so a member can be given a different intent from the rest of its batch.

Batches SHALL be presented before non-batched comments, batches ordered by their highest-severity member and ties broken by batch key ascending, non-batched comments ordered by severity and ties broken by `id` ascending — the ordering the `triage-command` capability mandates for the conversational loop, so the board and the loop present the same queue in the same sequence.

When `comments` is empty, the board SHALL render a message saying so and offer submit of an empty intent set.

#### Scenario: All seven presentation fields are rendered

- **GIVEN** a briefing with one comment
- **WHEN** the board is rendered
- **THEN** the served HTML contains that comment's `raised_by`, `location`, `description`, `cause`, `why_fix`, `suggested_fix` and `concerns_if_skipped`

#### Scenario: Suggested-fix origin is visible

- **GIVEN** a comment whose `suggested_fix_origin` is `investigation`
- **WHEN** the board is rendered
- **THEN** the served HTML marks that fix as the investigation's rather than the reviewer's

#### Scenario: A note input is present for every comment and every batch

- **GIVEN** a briefing with a three-member batch and two non-batched comments
- **WHEN** the board is rendered
- **THEN** the served HTML carries a note input for each of the five comments
- **AND** a note input for the batch

#### Scenario: Empty briefing

- **GIVEN** a briefing whose `comments` is empty
- **WHEN** the board is rendered
- **THEN** the served HTML says so
- **AND** submitting it writes an intents artifact carrying no entries

#### Scenario: Batch members are grouped with a batch-level control

- **GIVEN** a briefing with a five-member batch `B1`
- **WHEN** the board is rendered
- **THEN** the five members appear grouped under `B1` with its key and title
- **AND** a batch-level control is present that sets one intent on all five
- **AND** each member carries its own control

#### Scenario: Batches precede non-batched comments

- **GIVEN** a briefing with one batch whose highest-severity member is `major`, and a non-batched `critical` comment
- **WHEN** the board is rendered
- **THEN** the batch appears before the non-batched comment

### Requirement: Intents artifact

The board SHALL record the developer's calls as an intents artifact at
`<TAI_DATA_DIR>/plugins/triage/state/intents/<owner>--<name>--<scope>.json`,
where `<scope>` is `pr-<number>` for a PR scope and `branch-<slug>` for a branch scope. The artifact SHALL be overwritten in full on each submit for the same scope.

The artifact SHALL record the repo, the scope, the submit timestamp in RFC 3339, and one entry per comment carrying the comment's `id` from the briefing — its per-target position, which is what the triage verbs accept — its intent, and its note.

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

The system SHALL register four error codes in the append-only `pkg/errcode` registry:

- `TRIAGE_BOARD_INVALID_JSON` — the stdin briefing is not valid JSON. Exit code `1`, matching `IMPORT_INVALID_JSON`.
- `TRIAGE_BOARD_SCHEMA_INVALID` — the briefing parses but violates the schema. Exit code `3`, matching `IMPORT_SCHEMA_INVALID`.
- `TRIAGE_BOARD_UNAVAILABLE` — the board could not bind a loopback listener. Exit code `3`, joining the other codes for an environment that blocked the operation (`DATA_DIR_UNWRITABLE`, `CONFIG_UNWRITABLE`, `REPO_FETCH_FAILED`, `PLUGIN_FETCH_FAILED`, `INSTALL_TARGET_UNWRITABLE`). Exit code `1` is reserved for an unknown subcommand, a malformed flag, or conflicting options, none of which describes a bind failure.
- `TRIAGE_NO_INTENTS` — an intents artifact was requested for a scope that has none. Exit code `2`, joining the other "the thing you named does not exist" triage codes.

Both SHALL render through the foundation error template with the `[exit N: ERROR_CODE]` footer.

#### Scenario: Codes render through the foundation template

- **WHEN** either code is surfaced
- **THEN** stderr carries the foundation error template
- **AND** the footer reads `[exit 1: TRIAGE_BOARD_INVALID_JSON]`, `[exit 3: TRIAGE_BOARD_SCHEMA_INVALID]`, `[exit 3: TRIAGE_BOARD_UNAVAILABLE]` or `[exit 2: TRIAGE_NO_INTENTS]` respectively
