# tai source repo

This repo is a **tai source repo** — a git-versioned bundle of AI assets
(Claude Code skills, slash commands, agents, plus workflows and
standards) that one or more developers' machines pull from via `tai sync`.

## Layout

| Folder | What lives here | Synced into target? |
|--------|-----------------|---------------------|
| [`skills/`](skills/README.md) | Skill markdowns and folders. | Yes — `<target>/<skills>/` |
| [`commands/`](commands/README.md) | Slash-command markdowns. | Yes — `<target>/<commands>/` |
| [`agents/`](agents/README.md) | Agent markdowns. | Yes — `<target>/<agents>/` |
| [`workflows/`](workflows/README.md) | YAML workflows for `tai workflow run`. | No — accessed via `tai workflow`. |
| [`standards/`](standards/README.md) | Markdown standards for `tai standards load`. | No — accessed via `tai standards`. |
| `plugins.yml` | Additive list of plugins `tai sync` auto-installs. | No — read by tai itself. |

## Next steps

```sh
git remote add origin <your-remote-url>
git push -u origin main

# on each developer's machine:
tai config set repo-url <your-remote-url>
tai config target add ~/.claude
tai sync
```

See [`docs.tai.sh`](https://docs.tai.sh) for the full reference (or the
README in each subfolder for naming conventions and worked examples).
