## Why

tai's storage layer (`add-storage-schema`) is ready to hold triaged comments, but nothing has been built to *populate* it. Importing review comments is the first user-facing data flow: a user opens `/tai:import <pr>` (or `/tai:import branch <name>`, or `/tai:import stack`), Claude pulls comments from GitHub or staccato, enriches each one with the five mandatory fields, and the resulting structured payload lands in tai's SQLite database where the triage commands (specified in `add-triage-state`) can act on it.

This proposal is the bridge between the unstructured outside world (`gh api`, `st_reviews`, free-form review text) and tai's strict schema. The CLI itself owns the validation-and-persistence half; the bundled `/tai:import` Claude command owns the collection-and-enrichment half. The split keeps the CLI dumb (no GitHub API client, no Claude conversation logic) and the Claude command authoritative for everything subjective (severity, category, dedupe heuristics).

## What Changes

- Introduce `tai import -` — a CLI verb that reads a single JSON payload from stdin, validates it against a strict schema, and upserts the contents into the database inside a single transaction. The verb takes no positional arguments and no flags other than the foundation's `--repo` (which is unused because the JSON carries the repo identity; the flag is rejected with an error if provided).
- Introduce a stable JSON input schema covering one review target per invocation. Fields: `repo` (string `<owner>/<name>`), `target` (one of `pr` or `branch`), zero or more `batches`, and zero or more `comments`. Every comment carries the five mandatory enrichment fields (`severity`, `category`, `why_fix`, `suggested_fix`, `consequences`) plus the standard schema fields, plus an array of `external_refs` for idempotent re-import. A formal JSON schema is included in the spec.
- Stack mode is the caller's job, not the CLI's. The `/tai:import` Claude command invokes `tai import -` once per PR when scope is `stack`. The CLI never sees multi-target payloads.
- Define upsert semantics: every comment is keyed by its `external_refs`. If any ref already maps to an existing comment row, that comment is updated; otherwise a new comment is created. Triage state is preserved across re-imports — a comment whose `status` is not `pending` keeps its enrichment, batch assignment, and resolution fields untouched. Pending comments have their enrichment fields refreshed (so an upstream edit by the reviewer flows through), batch assignment refreshed, and any new `external_refs` attached.
- Define batch upsert: a batch is keyed by `(target, batch_key)`. Existing batches have their `title` updated; missing batches are inserted. A batch's `status` field is not touched by import; it is managed by the triage state machine.
- An empty payload (`comments: []` and `batches: []`) is a successful no-op: the repo and target rows are still upserted (so subsequent imports against the same target are cheap) and the CLI reports `Imported: 0 comments, 0 batches`.
- Introduce the bundled `/tai:import` Claude slash command. It operates in two **collection modes** depending on invocation:
  - **Remote mode** (PR scope, stack scope) — pulls raw comments via `st_reviews` when available, falling back to `gh api`. Enriches each comment in conversation; deduplicates across reviewers; groups into batches.
  - **Manual mode** (branch scope, `/tai:import branch <name> [--from <path>]`) — the source is whatever the user has at hand: a file path, content already in the current Claude conversation, or content the user pastes in response to a prompt. Intended for any review that wasn't pulled from a GitHub PR — an AI run against staged changes, a teammate's pasted review, a linter/audit report on disk. The slash command parses the input, enriches each comment, and pipes the payload to the CLI. No `gh`/`st_reviews` calls are made in this mode.
  Both modes pipe the assembled JSON payload to `tai import -`. The contract — what the command must produce, what it must NOT decide on the user's behalf — is part of the `import` capability spec.
- Reserve new error codes in the `cli-framework` taxonomy: `IMPORT_INVALID_JSON` (exit 1, malformed JSON), `IMPORT_SCHEMA_INVALID` (exit 3, parses but fails schema), `IMPORT_AMBIGUOUS_REFS` (exit 3, `external_refs` resolve to multiple existing comments).
- `tai import` is repo-independent at the CLI level. Repo identity is read from the JSON payload's `repo` field; no `git config` call is made. This is a deliberate departure from the typical `--repo`-aware pattern: the JSON is the authoritative source for "which repo this data belongs to".

## Capabilities

### New Capabilities

- `import`: The `tai import -` CLI verb, the JSON input schema, the upsert state machine, the import-layer error codes, and the bundled `/tai:import` Claude slash command's behavioural contract (what it pulls, how it enriches, what JSON shape it produces).

### Modified Capabilities

- `cli-framework`: Extends the error-code taxonomy with `IMPORT_INVALID_JSON`, `IMPORT_SCHEMA_INVALID`, `IMPORT_AMBIGUOUS_REFS`. Additive per the append-only rule.

## Impact

- No new third-party dependencies in the CLI itself. JSON parsing and schema validation use the standard library plus a small in-package validator. A heavyweight JSON Schema library is not required because the input schema is small and stable.
- The bundled `/tai:import` Claude command is shipped via `add-install-command`'s machinery — this proposal authors the markdown body that gets embedded; install handles the on-disk placement.
- `tai import` is the first verb that writes to SQLite. All earlier verbs (`tai install`, `tai uninstall`, `tai --help`, `tai --version`) either don't touch storage or only touch the filesystem under `~/.claude/`.
- This proposal does NOT specify the `tai list`, `tai show`, `tai accept`, `tai dismiss`, `tai status`, or `tai forget` verbs. They are the subject of `add-triage-state`. Import populates rows; triage state mutates and surfaces them.
- The slash command's collection step relies on the user having `gh` installed and authenticated, or having staccato's MCP server connected. Both are documented assumptions; absent both, the command tells the user what to install and exits without calling `tai import`.
