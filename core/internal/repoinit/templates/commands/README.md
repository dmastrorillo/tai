# commands/

Slash-command markdowns synced into each configured target's
`<target>/<commands>/` directory on `tai sync`. The user invokes them
in Claude Code as `/<command-name>`.

## Authoring rules

- **User-authored commands** can use any filename. They appear at the
  top level of `<commands>/` in the target.
- **Plugin-installed commands** are routed into the namespaced
  subdirectory `<commands>/tai-<plugin>/<verb>.md`. The plugin host
  manages that subdirectory; user authoring should stay outside it.

Filenames need no special prefix (unlike skills/agents) — the
namespaced subdir is the isolation mechanism.

## Worked example

```
commands/
  review-pr.md           # user-authored, invoked as `/review-pr`
  ship.md                # user-authored, invoked as `/ship`
  tai-triage/
    import.md            # plugin-installed, invoked as `/tai-triage:import`
    accept.md
```
