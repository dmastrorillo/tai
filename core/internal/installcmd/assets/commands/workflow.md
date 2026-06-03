# /tai/workflow

A bundled slash command that delegates to `tai workflow run` so the AI
agent can execute a workflow defined in the configured source repo.

## How to use

1. If the user names a specific workflow (e.g. "run the `propose`
   workflow"), invoke `tai workflow run <name>` and follow the
   markdown plan it emits.
2. If the user has not named a workflow, first invoke
   `tai workflow list` to discover the options, present the list, and
   ask which one to run.
3. The plan emitted by `tai workflow run` is the contract. The
   "Required tools" section names the tools the workflow needs; if
   any are unavailable in your session, report exactly which are
   missing and abort — do not substitute alternatives.

## Failure mode

If `tai workflow run` exits non-zero with `WORKFLOW_NOT_FOUND`, the
named workflow does not exist in the source repo. Run
`tai workflow list` to show what is available, and if the source repo
has changed recently, suggest the user run `tai sync` first.
