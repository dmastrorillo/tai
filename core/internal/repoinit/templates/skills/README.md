# skills/

Files and folders in this directory are synced into each configured
target's `<target>/<skills>/` directory on `tai sync`.

## Authoring rules

- **User-authored skills** can use any filename. They live alongside
  plugin-installed skills in the target.
- **Plugin-installed skills** MUST be named with the prefix
  `tai-<plugin>-` (e.g. `tai-triage-import.md` for a skill installed by
  the `triage` plugin). The plugin installer enforces this — anything
  that doesn't match is rejected with `PLUGIN_ASSET_NAMING`.

Folders are allowed: `<skills>/<my-skill>/SKILL.md` plus supporting
files works the same as a single `<skills>/<my-skill>.md` file.

## Worked example

```
skills/
  reading-code/
    SKILL.md
    references/
      checklist.md
  triage-comments.md
```

Both `reading-code/` (a folder with `SKILL.md`) and `triage-comments.md`
(a single file) are valid skills. `tai sync` copies them as-is.
