# agents/

Agent markdowns synced into each configured target's
`<target>/<agents>/` directory on `tai sync`. Agents are reached
transitively from skills and commands — they are not directly invoked
by the user.

## Authoring rules

- **User-authored agents** can use any filename.
- **Plugin-installed agents** MUST be named with the prefix
  `tai-<plugin>-` (e.g. `tai-triage-reviewer.md`). The plugin
  installer rejects anything that doesn't match with
  `PLUGIN_ASSET_NAMING`.

## Worked example

```
agents/
  code-explainer.md             # user-authored
  tai-triage-reviewer.md        # plugin-installed
```
