## 1. BDD cases — add TC-IDs to test-cases.md before any code

- [ ] 1.1 In `core/test-cases.md`, add a new category if needed and pin TC-IDs for the following scenarios: background git poll silent-fail on missing creds (HTTPS prompt suppressed), `PLUGINS:` block rendering in `tai --help` with and without installed plugins, `tai <plugin> help` forwards to plugin subprocess, `tai help <plugin>` ignores the plugin arg, first-run hint + marker file lifecycle, post-install hint emission/suppression, third-party install prompt (interactive yes/no, `--yes` bypass, non-TTY failure), `tai sync` third-party trust cache (skip/prompt/match/mismatch/`--trust-third-party`), pseudo-version → `dev`, clean tag passthrough, scaffolded README backlink + intro + no `docs.tai.sh`.
- [ ] 1.2 In `pkg/test-cases.md`, add TC-IDs pinning the new wire-contract verb `<plugin> --help-summary` (success, non-zero, empty stdout, >1 KB) and the `PLUGIN_ASSET_MISSING` validation.
- [ ] 1.3 In `plugins/triage/test-cases.md`, retire the TC-IDs for `tai triage install` and `tai triage uninstall` with tombstone comments (`<!-- TC-... retired YYYY-MM-DD: triage no longer ships its own installer; assets routed via host plugin-host SyncAssetsToTargets per bug-fix-round-1 -->`). Add new TC-IDs for: `triage --help-summary` returns the documented description string; `assets/commands/{import,triage,verify}.md` reference themselves as `/tai-triage:...` (content assertion).
- [ ] 1.4 Cross-check every new TC-ID drives at the CLI boundary per CLAUDE.md's "north star" rule. A TC about user-visible behaviour MUST get a test that captures stdout/stderr/exit, not just a unit assertion on a helper.

## 2. pkg/ framework changes (Bug 6, Bug 3 — error codes)

- [ ] 2.1 In `pkg/errcode/`, register new codes (append-only): `PluginAssetMissing`, `PluginHelpSummaryFailed`, `PluginThirdpartyUnconfirmed`. Map each to an appropriate exit code. Update `pkg/test-cases.md` if the error-code taxonomy section needs new entries.
- [ ] 2.2 In `pkg/taiplugin/`, add a `HelpSummary(string)` helper that plugin authors call before `cli.Run` to register the description, plus the `--help-summary` flag handling so plugins built against the SDK get the wire verb for free. Document in the package doc comment as part of the wire contract.
- [ ] 2.3 Add a Go test in `pkg/taiplugin` that exercises the new `--help-summary` flow via a stub command, asserting the documented exit/output behaviour.

## 3. Bug 5 — pseudo-version → `dev`

- [ ] 3.1 In `core/internal/version/version.go`, add a `looksLikePseudoVersion(string) bool` helper. Regex: `^v\d+\.\d+\.\d+(-[A-Za-z0-9]+(\.[A-Za-z0-9]+)*)?[.-]\d{14}-[0-9a-f]{12}$`. Add it to the existing `resolveVersion` cascade so a pseudo `Main.Version` returns `linked` (`dev`) instead of being surfaced.
- [ ] 3.2 Extend the table-driven `TestResolveVersion` in `core/internal/version/version_test.go` with cases for: canonical pre-1.0 pseudo, pseudo with `-0.` prefix, pseudo with prerelease before timestamp, real prerelease (must pass through), build-metadata (`v0.6.0+meta`, must pass through), and a deliberately-malformed pseudo-like string (must pass through verbatim — robustness).
- [ ] 3.3 Mirror the helper into `plugins/triage/internal/version/version.go` (each binary owns its version package per CLAUDE.md). Mirror the table tests too.
- [ ] 3.4 Update `RELEASE.md` and `.claude/skills/tai-release/SKILL.md` with a paragraph spelling out the pseudo-version carve-out: tagged installs surface the tag; pseudo-version installs (non-tagged `go install`, symlinked local) surface `dev`.

## 4. Bug 1 — background git poll silent-fail

- [ ] 4.1 In `core/internal/sync/poll.go`, set `cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/echo", "GCM_INTERACTIVE=Never")` on the two background `exec.CommandContext` invocations (`git ls-remote $repoURL HEAD` and `git -C <clone> rev-parse HEAD`). Foreground `clone.go` invocations remain untouched.
- [ ] 4.2 Add a unit test driving a fake git binary (or a `PATH` shim) that asserts the env vars are present on the background path and absent on the foreground path. Reuse the same test scaffolding as the existing poll tests.
- [ ] 4.3 Add an e2e test in `core/internal/cmd/sync_test.go` (or a new `poll_e2e_test.go`) that simulates a non-credential-helper HTTPS source and asserts the background poll fails silently — no prompt bytes written to any stream, no read attempted on stdin.

## 5. Bug 6 — delete triage self-installer, ship `assets/`, enforce in host

- [ ] 5.1 Delete the entire `plugins/triage/internal/installer/` package and its tests.
- [ ] 5.2 Delete `plugins/triage/internal/cmd/install.go` (the `newInstallCommand` / `newUninstallCommand` definitions).
- [ ] 5.3 In `plugins/triage/internal/cmd/root.go`, remove the `newInstallCommand(cfg.bundle)` and `newUninstallCommand(cfg.bundle)` entries from the subcommand list, plus any `bundle` plumbing the deletion makes orphan. Triage's `root.go` becomes shorter and no longer accepts a bundle parameter.
- [ ] 5.4 Implement `triage --help-summary` (using the new `pkg/taiplugin.HelpSummary` helper). Description: `Walk through pending PR review comments interactively.`
- [ ] 5.5 In `.goreleaser.triage.yaml`, extend `archives.files` to include `assets/**/*` mapped to `assets` in the tarball. Run `make release-snapshot` and inspect `dist/triage/tai-plugin-triage-*.tar.gz` to confirm the archive contains the `assets/` tree at the expected path.
- [ ] 5.6 In `core/internal/plugins/install.go`, add the `PLUGIN_ASSET_MISSING` check between fetch and `ValidateAssetNamespace`. Same insertion in `update.go` (which delegates to install for the staging-phase work).
- [ ] 5.7 In `core/internal/plugins/install.go`, after `ValidateAssetNamespace` but before `atomicReplaceDir`, exec the staged binary with `--help-summary`, capture stdout, validate against the limits in the spec, and either fail with `PLUGIN_HELP_SUMMARY_FAILED` or pass the captured description into the `Entry` written to `plugins.json`.
- [ ] 5.8 In `core/internal/plugins/state.go`, extend `Entry` with a `Description string \`json:"description"\`` field. Append-only — existing entries without the field remain readable.
- [ ] 5.9 Update the triage `test-cases.md` retirement entries from task 1.3 with actual dates, and ensure all Go tests that referenced the deleted code paths are removed or rewritten.

## 6. Bug 3 — plugin help discovery (`PLUGINS:` block + `tai <plugin> help`)

- [ ] 6.1 In `core/internal/cmd/root.go`, add a `--help` post-processor (urfave/cli's `CustomHelpTemplate` or a wrapping print step) that reads `plugins.json` and renders a `PLUGINS:` section after the auto-generated `COMMANDS:` block. Suppress the section header when the plugin list is empty.
- [ ] 6.2 In `core/internal/cmd/plugin_invoke.go`, confirm `dispatchPluginOrUnknown` already forwards `help` as a verbatim arg (it does — the rest slice is `args[1:]`). Add an e2e test driving `tai triage help` that asserts the exec'd binary receives `help` as `argv[1]`. Add a second test driving `tai help triage` that asserts the GLOBAL help is rendered and the plugin binary is NOT exec'd.
- [ ] 6.3 e2e test: `tai --help` output with one installed plugin contains the `PLUGINS:` block and the captured description. With no plugins installed, the output does NOT contain `PLUGINS:`.

## 7. Bug 7 — first-run hint + post-install hint

- [ ] 7.1 Add `core/internal/firstrun/` (a small package) exposing `MaybeEmit(io.Writer, dataDir string) (printed bool)` that reads/writes `<dataDir>/state/first-run.json`. Marker shape: `{"first-run": "<ISO-8601 UTC>"}`. Best-effort write — does not error the caller on failure.
- [ ] 7.2 In `core/cmd/tai/main.go`, after the foreground command's Run returns (regardless of error), and BEFORE the existing brief `Waiter.Wait()` for the background poll, call `firstrun.MaybeEmit(os.Stderr, dataDir)`. Suppress the call when `len(os.Args) == 1` AND stderr is not a TTY (per the spec's CI-friendliness rule).
- [ ] 7.3 In `core/internal/sync/banner.go`, when the first-run hint has fired this invocation, set the banner's `last-banner-date` to today so the daily banner is eligible to fire NEXT day instead of stacking.
- [ ] 7.4 In `core/internal/plugins/install.go` and `update.go`, after a successful install/update, write the post-install hint `→ Run \`tai <name> help\` to learn how to use <name>.` to the stderr writer threaded through from the verb's `*cli.Command`. When called from the `tai sync` auto-install loop, the writer is suppressed and the loop accumulates names for the aggregate hint instead.
- [ ] 7.5 In `core/internal/sync/sync.go` (or wherever the auto-install loop lives), accumulate the installed plugin names and emit the aggregate hint `→ <N> plugin(s) installed — run \`tai <name> help\` for any of: <comma-list>.` after the loop completes.
- [ ] 7.6 e2e tests: first-run hint emission on a clean data dir; suppression on a populated marker; pending-update banner skipped on first run with `last-banner-date` advanced; post-install hint after `tai plugins install triage`; suppression after a failing install; aggregate hint after a multi-plugin auto-install.

## 8. Bug 8 — third-party install prompt + trust cache

- [ ] 8.1 In `core/internal/plugins/`, add a helper `IsBuiltin(src Source) bool` that returns true when the source matches an entry in `registry.go::builtin` (host/repo/subpath equal). Used by the install/update path.
- [ ] 8.2 In `core/internal/plugins/install.go` (entry: `Install`) and `update.go`, before the fetch step, branch on `IsBuiltin(src)`. When false: print the third-party prompt to `stderr`, read confirmation from `stdin`. Honour `opts.AssumeYes` (new field, threaded from a new `--yes`/`-y` flag on `tai plugins install` and `tai plugins update`). Non-TTY without `AssumeYes` fails with `PluginThirdpartyUnconfirmed`. TTY-detection helper lives in `pkg/cliout` (`IsTTY`).
- [ ] 8.3 In `core/internal/cmd/plugins.go`, wire the `--yes`/`-y` boolean flag on `tai plugins install` and `tai plugins update`. Plumb the value into `InstallOptions.AssumeYes`.
- [ ] 8.4 Add a `core/internal/plugins/trust.go` (or similar) with `TrustStore` reading/writing `<dataDir>/state/trust.json`. Shape: `{"trust":[{"repo-url":"...","plugins-yml-sha256":"..."}]}`. Provide `Load`, `Save`, `Get(repoURL) (hash, ok)`, `Put(repoURL, hash)`.
- [ ] 8.5 In `core/internal/sync/sync.go` (start of `tai sync`, immediately after the `plugins.yml` parse), call a new helper `ConfirmThirdPartyPlugins(stderr, stdin, parsed, cfg, dataDir, assumeYesFlag, isTTY)`. The helper: short-circuits if no third-party entries; computes sha256 of the verbatim file bytes; loads the trust store; compares against stored hash; either proceeds silently, prompts, or fails per the spec.
- [ ] 8.6 Wire the new `--trust-third-party` flag on `tai sync`. Plumb into the `ConfirmThirdPartyPlugins` helper as the non-TTY/non-interactive bypass.
- [ ] 8.7 e2e tests: confirm three paths — TTY yes path persists hash; TTY no path aborts with `PLUGIN_THIRDPARTY_UNCONFIRMED`; non-TTY without `--trust-third-party` aborts; non-TTY with the flag proceeds and persists. Plus: internal-only plugins.yml never prompts and never writes trust.json.

## 9. Bug 4 — triage asset content rewrite

- [ ] 9.1 In `plugins/triage/assets/commands/triage.md`, rewrite every `/tai:triage`, `/tai:import`, `/tai:verify` reference to `/tai-triage:triage`, `/tai-triage:import`, `/tai-triage:verify` respectively. Same for `import.md` and `verify.md`. Total 31 replacements across the three files.
- [ ] 9.2 Update the front-matter `content_hash` field on each rewritten file (existing scheme uses sha256 of the body — regenerate it after the text changes).
- [ ] 9.3 Add a regression test in `plugins/triage/internal/cmd/` (or wherever the asset-content test lives — create one if absent) that scans the three markdowns and asserts no stale `/tai:` references remain.

## 10. Bug 2 — repoinit README template rewrite

- [ ] 10.1 Rewrite `core/internal/repoinit/templates/README.md`. Required elements: heading `# <repo-name> — a tai source repo` (the scaffold step substitutes `<repo-name>` with the directory base name — confirm via the embed-FS loader whether substitution is supported; if not, hard-code a neutral title and add a TODO for the operator to rename), one-paragraph intro describing tai, explicit backlink to `https://github.com/dmastrorillo/tai`, remove `docs.tai.sh` reference, preserve the existing folder-layout table and Next-steps block.
- [ ] 10.2 Update `core/internal/cmd/repo_init_test.go` (or wherever the scaffold test lives) with assertions matching the new spec scenarios: README contains the backlink substring, contains an intro sentence, does NOT contain `docs.tai.sh`.

## 11. CLAUDE.md and CONTEXT.md updates

- [ ] 11.1 In `CLAUDE.md`, under the Plugin host section, add a paragraph stating that plugins MUST place all target-bound assets via the tarball's `assets/` directory and MUST NOT write directly to target dirs from their own subcommands. Note the mandatory empty-assets requirement.
- [ ] 11.2 In `CLAUDE.md`, under Conventions, add a line referencing the pseudo-version-detection rule, pointing readers at `core/internal/version` for the regex.
- [ ] 11.3 Confirm `CONTEXT.md` already has the `First-party plugin` and `Third-party plugin` glossary entries (added in a prior commit on this branch); no additional CONTEXT.md edits required.

## 12. Validation, archive

- [ ] 12.1 Run `go test ./...`, `go vet ./...`, `gofmt -l .`. All clean.
- [ ] 12.2 Run `go test -race ./...`. Clean.
- [ ] 12.3 Run `golangci-lint run`. Zero issues against the configured baseline.
- [ ] 12.4 Run `make release-snapshot` and verify both archives extract cleanly. Confirm `dist/triage/tai-plugin-triage-*.tar.gz` contains the `assets/` tree.
- [ ] 12.5 Manual smoke: clean data dir, `go install ./core/cmd/tai` (symlinked local), confirm `tai --version` prints `dev`. Run any verb, confirm first-run hint fires and marker is written. Run again, confirm hint is suppressed. `tai plugins install triage` (after triage is also re-installed at the same dev cycle), confirm post-install hint, confirm files land at `<target>/commands/tai-triage/` and not at `<target>/commands/tai/`.
- [ ] 12.6 Update the affected `test-cases.md` files (core, pkg, triage) so the retired/added TC-IDs reflect the final landed state. Tombstone any IDs that no longer correspond to running tests.
- [ ] 12.7 Move the proposal to `openspec/changes/archive/<YYYY-MM-DD>-bug-fix-round-1/` and merge the spec deltas into the live `openspec/specs/<capability>/spec.md` files (per the existing archive flow in this repo). Bundle the archive into the same commit as the implementation per [[feedback-openspec-commit-flow]].
