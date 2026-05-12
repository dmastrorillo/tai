# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation`, `add-storage-schema`, and `add-install-command` are implemented (cli scaffolding, error taxonomy, SQLite tables + migration runner, command-framework hash machinery all in place).
>
> Add BDD cases under the `IMP` category (new — add to `test-cases.md` ToC) before each TDD slice.

## 1. JSON payload schema and parser

- [ ] 1.1 Define the Go structs for the payload (`Payload`, `Target`, `PR`, `Branch`, `Batch`, `Comment`, `ExternalRef`) in `internal/import/payload`
- [ ] 1.2 Implement strict decoding (`json.Decoder.DisallowUnknownFields()`) — reject any unknown key
- [ ] 1.3 Implement validation that walks the decoded struct and returns a slice of `ValidationError{Path, Message}`
- [ ] 1.4 Validation rules: required fields (non-empty strings), enum values (severity, category, target.kind), target shape (XOR pr/branch), `comments[].batch_key` references a known batch
- [ ] 1.5 TC-IMP-001..009: well-formed PR/branch payloads accepted; missing why_fix, invalid severity, both pr+branch, neither pr+branch, missing pr, empty external_refs, unknown field, batch_key references unknown batch all rejected
- [ ] 1.6 Validation reports ALL errors at once, not just the first
- [ ] 1.7 TC-IMP-010: multiple violations in one payload appear in one error message

## 2. `tai import -` command wiring

- [ ] 2.1 Wire `tai import` subcommand into urfave/cli
- [ ] 2.2 Require positional `-`; reject any other positional argument with a usage error
- [ ] 2.3 Mark `tai import` as repo-independent; reject `--repo` if combined with `tai import` as a usage error
- [ ] 2.4 Read all of stdin into memory before parsing (payload sizes are small, simplifies error reporting)
- [ ] 2.5 TC-IMP-020..023: stdin read, missing positional, wrong positional, --repo rejection

## 3. Error mapping

- [ ] 3.1 Map JSON decode errors → `IMPORT_INVALID_JSON` (exit 1)
- [ ] 3.2 Map validation errors → `IMPORT_SCHEMA_INVALID` (exit 3) with a multi-line "Error:" message listing every violation
- [ ] 3.3 Map ambiguous-ref errors → `IMPORT_AMBIGUOUS_REFS` (exit 3) including the conflicting comment IDs
- [ ] 3.4 Extend `internal/errcode` with the three new codes; verify the taxonomy table in code matches the spec
- [ ] 3.5 TC-IMP-030..032: each error code produces the standard footer per the foundation contract

## 4. Repo + target upsert

- [ ] 4.1 Implement `repos` upsert: `INSERT OR IGNORE` on `owner_name`
- [ ] 4.2 Implement PR target upsert: `INSERT OR IGNORE` on `(repo_id, number)`; title/url/head_branch written ONLY on insert
- [ ] 4.3 Implement branch target upsert: `INSERT OR IGNORE` on `(repo_id, name)`
- [ ] 4.4 TC-IMP-040..043: first import creates repo and target; re-import doesn't touch existing rows; PR title not overwritten on re-import; branch row created on first branch-scope import

## 5. Batch upsert

- [ ] 5.1 Implement batch upsert keyed by `(target, batch_key)`: update `title` if exists, insert with `status='pending'` otherwise
- [ ] 5.2 Track inserted vs updated batch counts for the summary
- [ ] 5.3 TC-IMP-050..052: new batch inserted, existing batch title updated, batch status preserved on re-import

## 6. Comment upsert by external_refs

- [ ] 6.1 Implement ref resolution: for each `comment.external_refs[i]`, look up `comment_external_refs(source_kind, external_id)` returning a set of distinct `comment_id` values
- [ ] 6.2 Case `len(set) == 0`: insert new comment + all refs
- [ ] 6.3 Case `len(set) == 1` and existing `status == 'pending'`: update enrichment fields + batch_id; insert any new refs
- [ ] 6.4 Case `len(set) == 1` and existing `status != 'pending'`: leave enrichment + batch_id alone; insert any new refs; increment "Frozen" counter
- [ ] 6.5 Case `len(set) > 1`: roll back transaction; exit `IMPORT_AMBIGUOUS_REFS` with conflicting comment IDs in the message
- [ ] 6.6 TC-IMP-060..065: new comment inserted, pending refresh, accepted frozen, dismissed frozen, completed frozen, ambiguous refs rejected
- [ ] 6.7 TC-IMP-066: refs added (new ref attached to existing pending comment increments "Refs added" counter)

## 7. Transactional execution

- [ ] 7.1 Begin transaction at start of `tai import` after validation passes
- [ ] 7.2 All upserts (repo, target, batches, comments, refs) run inside the transaction
- [ ] 7.3 Any DB error rolls back; emit appropriate code (`DB_CONSTRAINT_VIOLATION` from storage layer, `IMPORT_AMBIGUOUS_REFS` from our logic)
- [ ] 7.4 TC-IMP-070: a multi-comment payload where the third comment violates a constraint leaves zero rows persisted

## 8. Output summary

- [ ] 8.1 Implement the success summary writer per the spec (header line + counts block + `[exit 0]` tag)
- [ ] 8.2 Suppress zero-count lines (except header)
- [ ] 8.3 TC-IMP-080..082: PR header format, branch header format, empty-payload summary

## 9. Bundled `/tai:import` slash command

- [ ] 9.1 Author `commands/import.md` with the full contract laid out in `specs/import/spec.md`:
  - Scope-and-mode resolution table (PR/stack → remote; branch → manual)
  - Remote mode collection (st_reviews preferred, gh api fallback, exit-early when both missing)
  - Manual mode collection — three sub-paths in preference order: `--from <path>` (Markdown / JSON / plain text), current conversation context, user prompt
  - JSON-payload-shaped source files MAY skip per-field confirmation and use a single summary
  - Enrichment obligations (five mandatory fields, confirm with user, never silently invent)
  - External refs by mode (GitHub-derived in remote, deterministic `manual` hash in manual mode; pass-through when JSON source supplies them)
  - Dedup rules (auto-merge in remote, surface-for-confirm in manual)
  - Batching, invocation, stack-mode loop, ambiguous-ref handling
- [ ] 9.2 Update `commands/import.ledger.json` with the body hash (use `tai-ledger` helper from `add-install-command`)
- [ ] 9.3 Ensure frontmatter conforms to the `command-framework` schema (name, description, category, tags, version, content_hash)
- [ ] 9.4 Smoke test: `tai install` writes `import.md` into the target dir and classifier reports it as `up-to-date` immediately after
- [ ] 9.5 Manual exercise — sub-path 1 (file): write a markdown review to `./review.md`; invoke `/tai:import branch <name> --from ./review.md`; verify parsing, enrichment, and payload assembly with `kind: "manual"` external_refs
- [ ] 9.6 Manual exercise — sub-path 2 (conversation): simulate an in-conversation AI review against a local branch; invoke `/tai:import branch <name>` with no `--from`; verify the slash command pulls from context
- [ ] 9.7 Manual exercise — sub-path 3 (prompt): invoke `/tai:import branch <name>` with no `--from` and no recognisable conversation content; verify the slash command prompts the user for input and proceeds after a paste
- [ ] 9.8 Manual exercise — JSON pass-through: write a JSON file already matching tai's payload schema; invoke `/tai:import branch <name> --from ./review.json`; verify per-field confirmation is skipped in favour of a single summary; verify external_refs from the JSON are passed through verbatim
- [ ] 9.9 Idempotency check: re-run `/tai:import branch <name> --from ./review.md` with the same file content; verify the second run produces the same external_ref IDs and the CLI reports zero "Inserted" / N "Updated" or "Frozen"

## 10. Documentation and validation

- [ ] 10.1 Add `IMP` to `test-cases.md` ToC
- [ ] 10.2 Author package doc in `internal/import` pointing at `specs/import/spec.md`
- [ ] 10.3 Update `internal/errcode` taxonomy table to include the three new codes
- [ ] 10.4 `go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean
- [ ] 10.5 `openspec validate add-import-command` reports no errors
