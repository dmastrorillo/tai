# workflows/

YAML workflow definitions consumed by `tai workflow run <name>`.
Workflows are NOT synced into targets — they live exclusively in this
repo and are read on demand.

## File format

```yaml
name: review-and-ship
description: Open a PR, run the review skill, post comments, ship.
required-tools:
  - kind: skill
    name: review-code
  - kind: command
    name: ship
steps:
  - Run /review-code on the diff.
  - Apply suggested edits.
  - Run /ship to merge.
failure-mode: |
  If any required tool is unavailable in the current AI session, abort
  immediately and report which tool is missing.
```

Required fields: `name`, `description`, `required-tools`, `steps`,
`failure-mode`. The `kind` value MUST be `skill` or `command`
(`agent` is rejected because agents are not directly invokable).

## Naming

Nested folders give a colon-namespaced address. A file at
`workflows/devops/security.yml` is addressable as `devops:security`.
The reserved name `list` may NOT be used for a workflow.

## Invocation

```sh
tai workflow list
tai workflow run review-and-ship
```

The output of `tai workflow run` is a markdown plan the calling AI
reads — `tai` itself does not execute anything.
