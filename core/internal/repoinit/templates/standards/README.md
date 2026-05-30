# standards/

Markdown standards consumed by `tai standards load <name>`. Like
workflows, these are NOT synced into targets — they live exclusively
in this repo and are read on demand.

## File format

Each standard is a single markdown file. An optional YAML frontmatter
block at the top can carry a `description:` line that `tai standards
list` displays.

```markdown
---
description: How we structure pull-request descriptions.
---

# PR description format

Open with a "Why" paragraph. Then "What changed". Then "Test plan"...
```

When the frontmatter is missing, `tai standards list` shows
`(missing description in frontmatter)` for that standard.

## Naming

Nested folders give a colon-namespaced address (case-insensitive). A
file at `standards/devops/security.md` is addressable as
`devops:security`. Reserved names: `list`, `load` (sub-verbs of `tai
standards`).

## Invocation

```sh
tai standards list
tai standards load devops:security
```

`tai standards load` writes the body of the standard (frontmatter
stripped) to stdout — pipe it into your AI session as context.
