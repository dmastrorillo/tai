# Bundled Claude slash-commands

This directory is the on-disk home of every Claude slash-command tai
ships. Each bundled command consists of two paired files:

- `<verb>.md` — the slash-command markdown the user sees as
  `/tai:<verb>` inside Claude. Carries strict YAML frontmatter (six
  fixed keys; see `internal/cmdframework`) and a body. The body is
  what `tai install` writes to `~/.claude/commands/tai/<verb>.md`.

- `<verb>.ledger.json` — an ordered JSON array of every
  `sha256:<hex>` body hash this verb has ever shipped, oldest first.
  The last element is the current build's body hash. `tai install`
  uses the ledger to distinguish "stale-but-untouched" target files
  (overwrite silently) from "user-modified" ones (prompt or skip).

Both files are embedded into the binary via `//go:embed`. The
build-time helper `cmd/tai-ledger` keeps the ledger in sync — run
`make ledger-update` after editing any `<verb>.md` body, before
committing.

## Currently bundled

For human reference only; `cmdframework.Verbs()` is the authoritative
runtime enumeration (it discovers verbs by walking this directory's
embedded view).

- `import` — `/tai:import`, capture review comments into tai's DB.
  Authored by `add-import-command`.
- `triage` — `/tai:triage`, walk pending comments interactively.
  Authored by `add-triage-command`.
- `verify` — `/tai:verify`, confirm accepted comments are fixed and
  mark them completed. Authored by `add-verify-command`.

## Format invariants

- The ledger MUST be a JSON array.
- Every entry MUST match `^sha256:[0-9a-f]{64}$`.
- The array MUST be ordered oldest-first. The current build's body
  hash MUST equal the last element (build-time test enforces this).

## Why this lives under `internal/cmdframework/`

The conceptual path used throughout the docs and OpenSpec proposals
is `commands/<verb>.md`. The actual on-disk path is rooted under
`internal/cmdframework/` because `//go:embed` paths are resolved
relative to the Go source file containing the directive — embeds
cannot reach up the tree. The `tai-ledger` helper resolves the
physical path via `go.mod` lookup, so day-to-day usage matches the
conceptual model.
