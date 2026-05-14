# `tai-ledger`

Developer-only helper that keeps each bundled Claude slash-command's
hash ledger in sync with its markdown body.

The user-facing `tai install` verb reconciles target files against a
cumulative hash history (`commands/<verb>.ledger.json`) so that a body
that was once shipped — but has since been replaced — can be silently
overwritten, while a body the user has hand-edited prompts before being
clobbered. This helper appends new entries to those ledgers when a
command body changes.

## Run it

From the repo root:

```bash
make ledger-update
```

Or directly:

```bash
go run ./cmd/tai-ledger              # auto-resolves the bundled dir
go run ./cmd/tai-ledger -dir <path>  # explicit override
```

## When to run it

After editing any `internal/cmdframework/commands/<verb>.md` body,
before committing. The build-time test
`TestBundle_TCINST003_current_hash_is_last_entry` fails if the
in-binary ledger's last entry doesn't equal the current body hash —
running this helper is the fix.

## Behaviour

For every `<verb>.md` it finds (excluding `README.md`):

1. Compute the sha256 over the body bytes.
2. If `<verb>.ledger.json` does not exist, create it as a JSON array
   containing the single hash.
3. If `<verb>.ledger.json` exists and its last entry already equals
   the computed hash, do nothing.
4. Otherwise, append the new hash to the array and write the file.

The helper is idempotent. Running it twice with no body changes is a
no-op.

## Why a separate binary?

The user-facing `tai` binary embeds the ledgers as read-only data;
mutating them belongs in a developer tool, not the shipped CLI. Keeping
the mutation out of the main binary also lets the embed pattern in
`internal/cmdframework/ledger.go` stay strictly read-only, and lets
this helper depend on the `os` package's mutation primitives without
polluting the shipped binary.
