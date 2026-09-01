# Triage plugin assets

Files in this tree ship inside the `triage` plugin's release artifact
(`tai-plugin-triage-<os>-<arch>.tar.gz`). At install time
(`tai plugins install triage`) the tai host extracts the tarball and
copies these assets into every configured target per the plugin
asset-namespacing rules (post-Phase-6 plugin-host install path):

- `commands/*.md` → `<target>/commands/tai-triage/<file>.md` — the
  directory provides the `tai-triage` namespace; filenames don't need
  the prefix.
- `skills/tai-triage-*` and `agents/tai-triage-*` would land at
  `<target>/skills/<file>` and `<target>/agents/<file>` respectively
  (no per-name re-routing — the `tai-triage-` prefix is the namespace).
  Triage ships no skills or agents today.

This tree is the single source for the plugin's assets. The host is
the only sanctioned writer for target directories: a plugin MUST NOT
place files there from its own subcommands, and `triage` ships no
`install` / `uninstall` verbs. The tarball therefore always carries an
`assets/` directory — the host rejects one that does not with
`PLUGIN_ASSET_MISSING`, because that directory is its guaranteed input.
