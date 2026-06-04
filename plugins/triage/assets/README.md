# Triage plugin assets

Files in this tree ship inside the `triage` plugin's release artifact
(`tai-plugin-triage-<os>-<arch>.tar.gz`). At install time
(`tai plugins triage install`) the tai host extracts the tarball and
copies these assets into every configured target per the plugin
asset-namespacing rules (post-Phase-6 plugin-host install path):

- `commands/*.md` → `<target>/commands/tai-triage/<file>.md` — the
  directory provides the `tai-triage` namespace; filenames don't need
  the prefix.
- `skills/tai-triage-*` and `agents/tai-triage-*` would land at
  `<target>/skills/<file>` and `<target>/agents/<file>` respectively
  (no per-name re-routing — the `tai-triage-` prefix is the namespace).
  Triage ships no skills or agents today.

## Transitional duplication with `cmdframework/commands/`

Pre-Phase-6 the same markdown files lived under
`plugins/triage/internal/cmdframework/commands/` and were installed
in-process by the (then-named) `tai install` verb. That older path
remains available via `tai triage install` and is **NOT** the same
as the new plugin-host install path:

| Install path                       | Reads from                    | Writes to                                 |
|------------------------------------|-------------------------------|-------------------------------------------|
| `tai plugins triage install`       | `assets/commands/`            | `<target>/commands/tai-triage/<file>.md`  |
| `tai triage install` (in-process)  | `cmdframework/commands/`      | `<target>/commands/<file>.md` (flat)      |

This `assets/` tree is the canonical source for the new plugin-host
flow. Both trees ship identical bytes today, enforced by
`plugins/triage/assets/assets_test.go` (`TestAssetsMirrorCmdframework`)
which byte-compares the two and fails CI on divergence. A
post-Phase-6 cleanup will retire the in-process flow and remove the
duplication once the plugin-host install is the only path tested in
CI.
