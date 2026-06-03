# /tai/standards

A bundled slash command that delegates to `tai standards load` so the
AI agent can ingest a team-wide standard defined in the configured
source repo.

## How to use

1. If the user names a specific standard (e.g. "load the `sdlc`
   standard"), invoke `tai standards load <name>` and treat the
   printed body as authoritative guidance for the current task.
2. If the user has not named a standard, first invoke
   `tai standards list` to discover the options, present the list,
   and ask which one to load.

## Failure mode

If `tai standards load` exits non-zero with `STANDARD_NOT_FOUND`, the
named standard does not exist in the source repo. Run
`tai standards list` to show what is available, and if the source
repo has changed recently, suggest the user run `tai sync` first.
