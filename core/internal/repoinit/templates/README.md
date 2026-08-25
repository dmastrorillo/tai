# tai source repo

This is a **tai source repo** — a git-versioned bundle of AI assets
(skills, slash commands, agents, workflows, standards) that
[tai](https://github.com/dmastrorillo/tai) distributes onto each
developer's machine via `tai sync`.

## What is tai?

`tai` is a CLI for sharing AI tooling across a team like code. Each
developer points tai at this repo, runs `tai sync`, and tai copies the
configured assets into whichever target their AI tool reads from
(`~/.claude/`, `~/.opencode/`, or any custom path). Skills, slash
commands, and agents become available wherever they configured the
target; workflows and standards stay in the clone and are loaded on
demand. See the [tai repository on GitHub](https://github.com/dmastrorillo/tai)
for the full project README.

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

For naming conventions and worked examples, see the README in each
subfolder. For full tai usage, see
<https://github.com/dmastrorillo/tai>.
