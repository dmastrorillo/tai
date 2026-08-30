# Frozen formats

Identical every run, so the audit reads the same way each time.

## Thread replies

One line where one line will do. The reviewer reads dozens of these.

```
Fixed in [`abc1234`](https://github.com/OWNER/REPO/commit/abc1234). The arg-count check now goes through `requireOneArg` and carries the same usage example.
```

The short description matters when a commit covers many sites: it says what
changed _here_, since the commit message speaks for all of them.

```
Fixed in [`abc1234`](…). The code moved to `internal/triage/forget.go` in a later slice, so the fix is there rather than at the line quoted above.
```

A direction-shaped comment gets the survey result, not just the hashes, because
what was considered and rejected is the part that answers the question:

```
Fixed in [`abc1234`](…) and [`def5678`](…). Surveyed the file: the repo and branch selectors carried the same hand-rolled column ladder, so both now go through scope.ParentColumn. The batch selector is left as it is — it keys on batch_id, which has no parent-column variant.
```

```
No change. The 250ms Wait is deliberate: long enough for a fast state-file write to finish, short enough that a slow remote never delays `tai --version`; the subprocess itself is bounded separately.
```

```
Not changed, agreed above. CLAUDE.md's Conventions section exempts linker-injectable build-metadata vars from the no-package-level-mutable-state rule, and version.String meets its constraints (dedicated package, documented, never mutated at runtime).
```

```
Fixed in [`abc1234`](…).

For the record: I argued this validator's test bypass should stay package-private since the testing.TB parameter already gates it. You held that the exported ForTesting shape is the repo convention and greppability matters more. Applied your call.
```

```
Deferred to OpenSpec change `normalize-target-roots`, committed in [`abc1234`](…).
```

```
Deferred to #61.
```

```
Already fixed on slice 6 (#11) in [`abc1234`](…). No change needed here.
```

```
The code this points at is removed by slice 8 (#13). No change needed.
```

## Fix Audit

```markdown
# Fix Audit

Stack review applied. Every thread is replied to and resolved on its own pull
request; the fixes live here on the merge vehicle.

| Principle                                       | What changed                                            | Commit         | Threads                                                                          |
| ----------------------------------------------- | ------------------------------------------------------- | -------------- | -------------------------------------------------------------------------------- |
| Verb errors carry a runnable example in help    | 6 verbs in `core/internal/cmd`                          | [`abc1234`](…) | 8 threads<br><details><summary>links</summary>[#6](…) · [#8](…) · …</details>    |
| Same principle, sites not commented on          | 3 further verbs, 2 triage verbs                         | [`def5678`](…) | generalises [#8](…)                                                              |
| Query errors propagate, never default to zero   | 2 sites in `internal/triage`                            | [`9abcdef`](…) | 2 threads<br><details>…</details>                                                |
| Review principles recorded                      | `.review/errors.md`, `.review/index.md`                 | [`1234abc`](…) | n/a                                                                              |
| Answered, no change                             | n/a                                                     | n/a            | 1 thread<br><details>…</details>                                                 |
| Withdrawn after discussion                      | n/a                                                     | n/a            | 1 thread<br><details>…</details>                                                 |
| Deferred                                        | OpenSpec `normalize-target-roots`, issue #61            | [`abcd123`](…) | 2 threads<br><details>…</details>                                                |

**14 threads extracted, 14 accounted for.** 2 top-level comments replied to
without resolving, since GitHub gives those no resolved state.
```

The reconciliation line is the part that makes this an audit rather than a
changelog. The counts have to match, and if they do not, say so instead of
adjusting one of them.

Rows for non-code outcomes carry no commit and exist so every thread appears
exactly once. A thread that is in no row is a thread that was lost.

## `.review/index.md`

A hook list, not a content dump. `CLAUDE.md` points at this index, so it loads
on every session and has to stay cheap; the topic files are read when they are
needed.

```markdown
# Review principles

Craft principles taken from review comments on this repository. Nothing here is
enforced by gofmt, go vet, or golangci-lint, and nothing here restates
`CLAUDE.md`, `docs/*`, or a `test-cases.md`. Architecture and rules live there.

- [Error output](errors.md) - help bullets name the exact command to run
- [Test fixtures](fixtures.md) - fixture helpers take the tree subdir, not a copy of the staging loop
```

## `.review/<topic>.md`

Concise enough to read in full, specific enough to apply without asking. Each
entry stands alone: no reference to the discussion that produced it, and no
pull-request numbers in the prose, since those go stale and the evidence line
already carries them.

````markdown
# Error output

## Help bullets name the exact command to run

**Scope:** repo

A `WithHelp` bullet that says what to do without saying how forces the user to
reconstruct the invocation from the message. Name the command, with real values
where the error already knows them.

Do:

```go
WithHelp("example: `tai config target add ~/.claude`")
```

Don't:

```go
WithHelp("add a target to your config")
```

**Evidence:** 8 threads across slices 3 to 6 of `refactor/review-fixes`.
````

The scope line holds either `repo` or the tree the principle holds for
(`core`, `pkg`, `triage`). It is the field promotion reads to pick a
destination, so it is a judgement recorded once rather than one re-derived from
the prose every time the entry is revisited.

The evidence line names where the principle came from and how often it recurred,
which is what distinguishes a principle worth keeping from a one-off preference.
