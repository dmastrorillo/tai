// Package triage owns tai's read-and-mutate state machine for
// triaged review comments: per-target position translation,
// transitions to accepted/dismissed/completed, batch member
// iteration, and batch status recomputation.
//
// The behavioural contract is `openspec/changes/add-triage-state/
// specs/triage/spec.md`.
//
// Subordinate package:
//
//   - internal/triage/scope — resolves the operating scope for every
//     triage verb (PR / branch / auto-detect) per the spec's
//     precedence rule.
//
// The CLI verbs that consume this package live under internal/cmd/:
//
//   - list.go       — `tai list`
//   - show.go       — `tai show` (single + --all)
//   - accept.go     — wires the `accept`/`dismiss`/`complete` subcommands
//   - mutate.go     — shared `runTransition` used by all three
//   - status.go     — `tai status`
//   - forget.go     — destructive `tai forget`
//   - triage.go     — shared flag names and `openDBAndScope` helper
package triage
