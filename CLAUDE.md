# tai — Go CLI

A Go-based command-line tool. This file documents how we build it. The pipeline is non-negotiable: **OpenSpec → per-component `test-cases.md` (BDD) → tests (TDD) → production code**. Skipping a layer is a process bug, not a shortcut.

The repo is a single Go module organised as a monorepo: `core/` holds the `tai` core binary, `plugins/<name>/` holds first-party plugins (today only `triage/`), `pkg/` exposes the shared framework packages (`errcode`, `cliout`, `exitcode`, `cliexec`, `datadir`) for plugin authors. The BDD spec is split per component: `core/test-cases.md` for core-CLI behaviours, `pkg/test-cases.md` for the framework's stability contract (error template, panic recovery, error-code taxonomy, data-directory resolution), and `plugins/<name>/test-cases.md` for each plugin.

---

## Behavioural Source of Truth (mandatory)

The per-component `test-cases.md` files are the authoritative, human-readable specification of how each piece of this repo behaves: `core/test-cases.md` for the `tai` core CLI, `pkg/test-cases.md` for the shared framework's public stability contract, `plugins/<name>/test-cases.md` for each plugin. They hold BDD-style Given/When/Then scenarios covering happy paths, edge cases, and known historical regressions. **They are the contract; the code is downstream.**

**The flow is: OpenSpec proposal → BDD cases → tests → production code → observed CLI behaviour.** A change is "real" only after it appears as a Given/When/Then in the appropriate `test-cases.md`, is exercised by a test that names its TC-ID, and is implemented behind that test.

### Mandatory workflow for every change

Feature request, bug fix, refactor that alters behaviour, output/format tweak with observable consequences — all follow the same loop:

1. **OpenSpec proposal first.** Open `openspec/` and draft (or update) the change proposal that describes the intent, the new capability, and the user-visible contract. Behaviour decisions land here before any test or code is written. Archive completed proposals under `openspec/changes/archive/`.
2. **Translate the proposal into BDD cases in the right `test-cases.md`.** Core CLI behaviour lands in `core/test-cases.md`; shared-framework behaviour (the public surface under `pkg/`) lands in `pkg/test-cases.md`; plugin behaviour lands in `plugins/<name>/test-cases.md`. Find the section(s) the change affects. New feature or newly discovered edge case → add new Given/When/Then entries with a fresh `TC-<CATEGORY>-<NUMBER>` ID. Bug fix → add a regression case under the regressions section, cross-referenced to the feature section. Behavioural change to existing functionality → update the affected cases in place; do NOT leave stale scenarios.
3. **Invoke the `/tdd` skill and write red/green slices.** Each BDD case becomes a failing test that names the TC-ID in its description (e.g. `t.Run("TC-CMD-001 — prints version string", ...)`). Red → green → refactor. One slice per case (or per case + edge).
4. **Implement until the tests pass and no existing tests regress.**
5. **Before merging**, re-read the cases you touched and confirm the affected `test-cases.md` still describes reality. If a case is obsolete because behaviour deliberately changed, rewrite it; never silently contradict it with code.

If a test case and the code disagree, **the test case is the spec** — investigate the code. The only exception is when the current task's explicit goal is to change that behaviour, in which case the case is rewritten as part of the change.

**Mid-implementation clarifications:** if a case is ambiguous or incomplete while building, update the case in its `test-cases.md` as part of the same change rather than coding around the ambiguity.

### The north star is what the user observes at the CLI

A CLI's behaviour is what shows up in the terminal: stdout bytes, stderr bytes, exit code, files written, files read, prompts displayed, the order of all of the above. It is **NOT** the value of an internal struct field, the shape of a returned `error`, or the absence of mutation on a slice. Those are code-internal facts that may or may not surface.

When a test claims a TC-ID, it MUST drive at the layer where the user observation happens:

- **TC mentions stdout/stderr content** ("the command prints X", "the error message contains Y") → end-to-end command test that captures the actual output bytes.
- **TC mentions exit code** ("the command fails with non-zero exit") → end-to-end test that asserts `os.Exit` / `cmd.Run()` return value.
- **TC mentions a file-system side effect** ("writes a config at ~/.tai/config.yaml") → integration test with a tmp HOME that inspects the file after.
- **TC mentions an interactive prompt** ("asks for confirmation before deleting") → test that drives stdin and asserts the prompt text.
- **TC mentions an internal invariant only** ("the parsed config struct has field X set") → unit test on the parser is fine. But if the case ALSO mentions a visible consequence, that consequence needs its own test at the CLI boundary.

Engine and helper tests are valuable scaffolding, but they NEVER satisfy a TC about what the user sees — even when the helper is the proximate cause. The wiring between flag parsing, command dispatch, business logic, and output formatting is itself a load-bearing layer with its own failure modes.

**Triage question for every TC-ID-tagged test:** "Could this test pass while the user sees something the spec forbids?" If yes, the existing test isn't wrong — keep it — but the spec is **under-tested**. Add another test at the CLI boundary until the answer becomes "no, a green suite implies the user sees what the spec promises."

### ID scheme

**`TC-<CATEGORY>-<NUMBER>`** (e.g. `TC-CMD-015`, `TC-CFG-003`). Category is a short, stable code for the section, listed in the table of contents at the top of each `test-cases.md`. Numbers increment within each category, starting at `001`, zero-padded to 3 digits, and remain globally unique across components — a TC-ID lives in exactly one `test-cases.md` and is never renumbered when a section moves between core and a plugin.

- When you add a case to an existing category, use the next unused number in that category.
- When you create a new category, add its code to the ToC and start at `001`.
- **Never renumber existing IDs** — tests reference them.
- Each case is self-contained — reads without surrounding context.

### Case retirement and splitting

- If a case is removed because the feature was removed, leave a one-line tombstone (e.g. `<!-- TC-CMD-006 retired YYYY-MM-DD: --legacy flag removed in commit abc123 -->`) so the ID isn't mistaken for one that never existed. Update or delete any tests that reference the retired ID.
- If a case is split into two, keep the original ID for the dominant assertion and assign the new sub-case the next unused number in the category. Update referencing tests accordingly.
- Never silently delete a case whose TC-ID appears in test code.

---

## OpenSpec

`openspec/` holds the change-proposal layer. It exists so behaviour is debated and decided before tests are written, not during.

- `openspec/config.yaml` — schema config.
- `openspec/specs/` — long-lived capability specs (one folder per stable capability).
- `openspec/changes/` — active change proposals.
- `openspec/changes/archive/` — proposals that have shipped.

**Rules:**

1. Every behaviour change begins with an OpenSpec proposal — even a one-paragraph one. The proposal names the user-visible contract it introduces or modifies.
2. A proposal is "done" only when (a) the matching BDD cases exist in the relevant `test-cases.md` (core, plugin, or both), (b) the tests are green, (c) the production code is merged, and (d) the proposal is moved to `openspec/changes/archive/` with the merge date in its frontmatter.
3. If a proposal is abandoned, archive it with a `status: abandoned` note rather than deleting it — future you will want the breadcrumb.

---

## `cli-developer` skill (mandatory for Go CLI work)

The **`cli-developer`** skill at `.claude/skills/cli-developer/` is the authoritative guide for writing Go CLI code in this repo. It MUST be invoked for any Go CLI coding task — implementing a new command, adding flags, refactoring command wiring, writing/fixing tests at the CLI boundary, anything that produces or modifies Go code. Invoke it before writing or editing Go files, not after.

If you find yourself editing `*.go` and haven't loaded the skill, stop and load it first.

---

## TDD (mandatory)

Use the **`/tdd`** skill for every implementation slice. The loop is:

1. **Red.** Write the smallest failing test that names the TC-ID and asserts the user-visible behaviour. Run it; confirm it fails for the right reason.
2. **Green.** Write the minimum production code to make it pass. No speculative abstractions.
3. **Refactor.** Tidy with the test still green.
4. **Repeat** for the next slice / case.

Do not write production code without a red test pointing at it. Do not write a test without a TC-ID. Do not write a TC-ID without an OpenSpec proposal it traces back to.

**Failing tests are regressions, not broken tests.** If a test fails, assume the code under test has regressed. Investigate the behaviour the test describes and fix the production code. Only rewrite a test's expectations when the specific behaviour it covers was deliberately changed, and that change is the actual goal of the current task. Never silence, skip, or loosen a failing test to make the suite green.

---

## Stack

- **Go** (latest stable). Modules; standard `go test`.
- **`urfave/cli`** for command/flag parsing.
- **`log/slog`** (standard library) for structured logging.
- Standard library first for everything else. No dependency added without an OpenSpec proposal naming why std-lib isn't enough.

---

## Commands

- `go build ./...` — compile every package.
- `go test ./...` — run unit + integration tests.
- `go test -race ./...` — same with the race detector. Run before merging.
- `go vet ./...` — static checks. Must be clean.
- `gofmt -l .` — formatting check. Output must be empty.
- `golangci-lint run` — linter (once configured). Zero-baseline: 0 issues.

Run `go test ./... && go vet ./... && gofmt -l .` before every commit. CI runs the same plus `-race` and the linter.

---

## Project Structure

(Targets — directories are created as the corresponding OpenSpec proposals land.)

Top-level layout:

- `core/` — the `tai` core CLI tree. Self-contained: entry point, core-only internals, and its own `test-cases.md`.
- `plugins/<name>/` — one tree per first-party plugin. Self-contained: entry point, plugin-only internals, bundled assets, and its own `test-cases.md`. Today the only plugin is `triage/`.
- `pkg/` — shared framework packages with a public Go API. Plugin authors (first- and third-party) import from here. Anything published under `pkg/` is on a stability contract: append-only error codes, no renaming or repurposing exported symbols. Behavioural guarantees are pinned by [`pkg/test-cases.md`](pkg/test-cases.md).
- `openspec/` — change proposals (see above).
- `CLAUDE.md` — this file.

Inside `core/`:

- `core/cmd/tai/main.go` — entry point. Wires the root `urfave/cli` command, calls `Run`, fires the background update-check goroutine (`sync.Schedule`) with a brief Wait on exit. Thin — no business logic.
- `core/internal/cmd/` — command-tree assembly (`NewRoot`, plus one file per top-level verb: `config.go`, `repo.go`, `sync.go`, ...). End-to-end tests live alongside as `*_test.go`.
- `core/internal/config/` — YAML config loader, schema, validation, lazy save. Spec: `openspec/changes/pivot-to-ai-as-code/specs/config/spec.md`.
- `core/internal/sync/` — `tai sync` engine: clone manager, eager git fetch with cache fallback, M1 overwrite detection, per-target manifest, prune, batched prompt, background update-check goroutine. Spec: `openspec/changes/pivot-to-ai-as-code/specs/repo-sync/spec.md`.
- `core/internal/repoinit/` — `tai repo init` scaffold with embedded templates, git init + initial commit. Spec: `openspec/changes/pivot-to-ai-as-code/specs/repo-init/spec.md`.
- `core/internal/verbs/` — canonical reserved-verbs registry consumed by the plugin host. `verbs.IsReserved(name)` is the install-time gate that emits `PLUGIN_NAME_RESERVED`.
- `core/internal/plugins/` — plugin host: built-in first-party registry (`registry.go`), on-disk state (`state.go`, written to `<TAI_DATA_DIR>/state/plugins.json`), the HTTP-backed `Fetcher` (`fetch.go`), the asset namespacing rules (`assets.go`), and the install/update/remove/list verb implementations. Spec: `openspec/changes/pivot-to-ai-as-code/specs/plugin-host/spec.md`.
- `core/internal/version/` — build-metadata package exposing the linker-injectable `version.String` for the core binary. Kept separate to isolate one of the project's sole package-level mutable-var exceptions (see Conventions).
- `core/test-cases.md` — BDD spec for core-CLI behaviours.

Inside each `plugins/<name>/`:

- `plugins/<name>/cmd/<binary>/main.go` — entry point for the plugin's executable. Consumes the wire-level plugin contract (env vars `TAI_CLONE_DIR`, `TAI_TARGETS`, `TAI_DATA_DIR`) via `pkg/taiplugin` and dispatches to its own subcommands.
- `plugins/<name>/internal/<domain>/` — plugin-only domain logic. Not importable from `core/` or sibling plugins.
- `plugins/<name>/internal/version/` — plugin's own linker-injectable version string, independent of core.
- `plugins/<name>/assets/` — bundled commands / skills / agents the plugin installs into configured targets at install time.
- `plugins/<name>/test-cases.md` — BDD spec for that plugin's behaviours.

Inside `pkg/`:

- `pkg/errcode/` — append-only error-code taxonomy and the `*errcode.Error` value type with exit-code bindings.
- `pkg/cliout/` — error-template writer that emits the foundation footer `[exit N: ERROR_CODE]`; also owns `IsTTY` for stdout/stderr discipline.
- `pkg/exitcode/` — named exit-code constants.
- `pkg/cliexec/` — `Run(ctx, cmd, args)` wrapper around `*cli.Command.Run` that turns panics into structured `INTERNAL_ERROR` errors. Used by every binary's `main` and by every cmdtest harness so production and tests share the same recovery path.
- `pkg/datadir/` — resolves and (lazy-) creates TAI's per-user data directory (`$TAI_DATA_DIR` > `$XDG_DATA_HOME/tai/` > platform default). Promoted from `plugins/triage/internal/datadir` in Phase 2 of pivot-to-ai-as-code when `core/internal/sync` needed cross-tree access. Surfaces `DATA_DIR_UNWRITABLE` on failure.
- `pkg/taiplugin/` — Go SDK for plugin authors. `taiplugin.Load()` parses the wire-contract env vars (`TAI_DATA_DIR`, `TAI_CLONE_DIR`, `TAI_TARGETS`) into a typed `*Context`. `EnvVars(...)` is the inverse used by `tai` itself when invoking a plugin; the pair guarantees that a plugin authored against the SDK sees exactly what the host promises. The package's three env-var names and the Target struct shape are append-only.
- `pkg/test-cases.md` — BDD spec for the framework's stability contract.

Production code lives under `core/internal/`, `plugins/<name>/internal/`, or `pkg/`. The two `internal/` trees enforce isolation between core and plugins; `pkg/` exposes the small, stable surface plugin authors are allowed to import. Anything under either `internal/` tree cannot be imported by other modules — that's the contract.

`go.mod` records direct dependencies; `go.sum` tracks transitive hashes (including dependencies-of-dependencies pulled in by their own test suites). `go.sum` is not the source of truth for what tai depends on — `go.mod` is.

---

## Testing layout

- **Unit tests** live next to the code they test (`foo.go` + `foo_test.go`), same package.
- **End-to-end command tests** live in `core/internal/cmd/*_test.go` (for core verbs) and `plugins/<name>/internal/cmd/*_test.go` (for plugin verbs). They exercise the assembled `*cli.Command` via `cliexec.Run(ctx, cmd, args)` with `cmd.Writer` / `cmd.ErrWriter` pointed at captured buffers. These are where most TC-IDs land — they are at the layer the user observes.
- **Integration tests** (file-system, real config loading, etc.) live in `<tree>/internal/<pkg>/*_integration_test.go` with a build tag if they're slow or environment-sensitive: `//go:build integration`.

Run the integration tier with `go test -tags=integration ./...`.

### Test bypasses for `pkg/`-level validators

Some `pkg/` validators (e.g. `core/internal/config.validateRepoURL`) reject inputs that the e2e test harness needs (`file://` URLs for hermetic bare-repo fixtures, etc.). The pattern is: each validator dispatches through a package-level `*Func` variable that defaults to the strict production implementation. The package exports a `<Name>ForTesting(t testing.TB)` helper that swaps in a permissive variant and registers a `t.Cleanup` to restore the strict default. The `testing.TB` parameter makes accidental production use a glaring code-review red flag; the t.Cleanup keeps each test self-contained. Example: `config.AllowFileURLsForTesting(t)` in `core/internal/cmd/sync_test.go`.

Test naming convention: `TestCommandName_TCID_short_description`, e.g. `TestVersion_TCCMD001_prints_version_string`. The `TCID` segment is the test-case ID with hyphens stripped, preserving every character — `TC-CMD-001 → TCCMD001`, not `TCMD001`. The TC-ID in the name is the breadcrumb back to the right `test-cases.md` (`core/`, `pkg/`, or `plugins/<name>/`).

---

## Conventions

- No `panic` in normal control flow. Return errors. Each binary's `main` (e.g. `core/cmd/tai/main.go`, `plugins/<name>/cmd/<binary>/main.go`) is the only place in its tree that translates an error into an exit code.
- Wrap errors with `fmt.Errorf("context: %w", err)`. Never lose the cause.
- Use `context.Context` for anything that might be cancellable or time out (network, long file walks, prompts).
- Logging: `log/slog` from the standard library.
- Don't write package-level mutable state. The sole exception is **linker-injectable build-metadata variables** — variables declared `var` specifically so `go build -ldflags="-X …"` can overwrite them at link time (e.g. `core/internal/version.String`, `plugins/<name>/internal/version.String`). These MUST be documented at their declaration site, MUST NOT be mutated from Go code (including tests), and MUST live in a dedicated package so the exception's surface stays narrow. Each binary owns its own version package — core and each plugin ship on independent release lifecycles.
- One exported symbol per file is a guideline, not a rule — but if a file has many, look for a missing package boundary.

---

## Plugin host

The plugin layer is a subprocess-exec contract, not an in-process Go-plugin abstraction. Anything a plugin needs from `tai` flows over a small, stable surface: three environment variables and the foundation error template.

### Wire contract (do not change without a major version bump)

When `tai` invokes a plugin subprocess (`tai <plugin> <args>`), it sets these environment variables in addition to the inherited environment:

| Variable | Meaning |
|----------|---------|
| `TAI_DATA_DIR` | Absolute path of `tai`'s per-user data directory. Plugins place their own state under `$TAI_DATA_DIR/plugins/<name>/state/`. |
| `TAI_CLONE_DIR` | Absolute path of the source-repo clone, or empty when no `repo-url` is configured. |
| `TAI_TARGETS` | JSON array of `{root, skills, commands, agents}` for every configured target, with effective (defaulted) sub-paths. Empty array when no targets are configured. |

Stdin, stdout, stderr, and the exit code pass through unchanged. The host translates a non-zero child exit into its own exit code; the plugin owns its own template-conforming error output via `pkg/cliout`.

Go plugin authors should not parse the env vars themselves: import `pkg/taiplugin` and call `taiplugin.Load()` for a typed `*Context`. The same package re-exports the error code taxonomy (`pkg/errcode`) and the CLI output writer (`pkg/cliout`) so a plugin's footer-format is identical to `tai`'s.

### Asset namespacing

Every asset a plugin distributes lives inside the plugin's namespace. The rules are enforced at install time:

- **Skills**: every entry in the plugin's `assets/skills/` MUST start with `tai-<plugin>-`. Install fails with `PLUGIN_ASSET_NAMING` otherwise.
- **Agents**: same prefix rule for `assets/agents/`.
- **Commands**: filenames in `assets/commands/` are unconstrained — `tai` routes them into `<target.commands>/tai-<plugin>/` regardless of authored name.

The namespace IS the manifest. `tai plugins <name> update` wipes the plugin's namespace in every target and re-copies, with no overwrite prompts.

### Local state: `plugins.json`

The host's record of installed plugins lives at
`<TAI_DATA_DIR>/state/plugins.json`. The file's shape:

```json
{
  "plugins": [
    {
      "name": "triage",
      "source": {
        "host": "github.com",
        "repo": "dmastrorillo/tai",
        "subpath": "",
        "version": "v0.5.0"
      },
      "version": "v0.5.0",
      "installed-at": "2026-05-30T12:00:00Z"
    }
  ]
}
```

The schema is append-only — new fields MAY be added (every existing
plugin install must remain readable by future tai versions); fields
MUST NOT be renamed or removed without a major version bump.

### Auto-install from `plugins.yml`

At the start of every `tai sync`, the host reads `<clone>/plugins.yml`
(if present) and installs any listed plugin not already in
`plugins.json`. The list is additive — removing a YAML entry does NOT
uninstall the plugin from a developer's machine (removal is
exclusively a user gesture via `tai plugins <name> remove`).

A plugin-install failure during this hook (bad source, network down,
401) aborts the entire `tai sync` with that error code. This is
intentional: the host cannot reason about which assets in the
source-repo sync depend on which plugin, so partial-failure modes
risk producing a broken target state. Users can recover by removing
the offending entry from `plugins.yml` (or installing the plugin
manually with `tai plugins <name> install --source ...`) before
re-running sync.

### Adding a first-party plugin

A first-party plugin is one whose canonical source lives in this repo's GitHub Releases and whose name is hard-coded into the registry. The two-step workflow:

1. **Register the entry in `core/internal/plugins/registry.go`.** Add an entry to the `builtin` map pointing at `Source{Host: "github.com", Repo: "<this-repo>"}`. The map is the only authoritative list — there is no `plugins.yml` for first-party plugins.
2. **Cut a release whose assets follow `tai-plugin-<name>-<os>-<arch>.tar.gz`.** The tarball ships the binary and the `assets/` directory at the top level. The CI matrix in `.github/workflows/` builds these assets across the platform matrix; the release job attaches them.

Both steps land in the same OpenSpec change. A registry entry without a release leaves users staring at a `PLUGIN_FETCH_FAILED` until the release ships, so don't merge half of the pair.

### Reserved verb collisions

A plugin's directory name is also its top-level CLI verb. The reserved list (`config`, `sync`, `repo`, `install-commands`, `workflow`, `standards`, `plugins`, `help`, `version`) is owned by `core/internal/verbs.Reserved()`. Adding a new top-level verb to TAI MUST also append to that list in the same OpenSpec change, since the install path checks `verbs.IsReserved` to surface `PLUGIN_NAME_RESERVED`.

---

## When you forget the pipeline

If you find yourself about to write production code and you haven't:

1. Opened an OpenSpec proposal,
2. Added/updated a Given/When/Then in the appropriate `test-cases.md` (core, `pkg/`, or plugin) with a TC-ID, and
3. Written a failing test that names that TC-ID,

**stop and back up.** The pipeline is the product's memory. Skipping it produces code that works today and is unexplained tomorrow.
