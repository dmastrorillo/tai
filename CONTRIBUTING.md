# Contributing to TAI

Thanks for being interested. TAI is small, opinionated about its own pipeline, and unopinionated about what teams ship through it. The rules below exist because they have already paid for themselves in clarity.

## Before you open a PR

Read these first. If your change is not consistent with them, the PR will be hard for both of us.

- [`docs/NORTHSTAR.md`](docs/NORTHSTAR.md) — what TAI is and is not for. A feature that drifts from the north star is the most common reason a PR gets pushed back.
- [`CONTEXT.md`](CONTEXT.md) — vocabulary. Use the canonical terms (source repo, target, asset, workflow, standard, plugin) the way the glossary uses them.
- [`CLAUDE.md`](CLAUDE.md) — the project's pipeline contract: OpenSpec → BDD → TDD. This applies to every behaviour change, including small ones.

If you are unsure whether your idea aligns with the north star, open an issue with the **Feature request** template before you write code. It is much cheaper to align on direction first.

## The pipeline (OpenSpec → BDD → TDD)

For any change that alters observable behaviour (anything a user sees in stdout, stderr, an exit code, or on disk), the order is:

1. **OpenSpec proposal.** Open or update a folder under `openspec/changes/<name>/` with the proposal, design, specs, and tasks. For small changes this can be brief; for cross-cutting changes it is the design document. Archived proposals under `openspec/changes/archive/` are good references.
2. **BDD cases.** Translate the proposal into Given/When/Then cases in the right `test-cases.md` — `core/test-cases.md` for core CLI behaviour, `pkg/test-cases.md` for shared-framework (`pkg/`) behaviour, `plugins/<name>/test-cases.md` for plugin code. Each case gets a `TC-<CATEGORY>-<NUMBER>` ID, globally unique across components.
3. **Failing test.** Write a test that references the TC-ID in its name. Run it. Confirm it fails for the right reason.
4. **Implement.** Make the test pass. No production code without a red test pointing at it.
5. **Archive.** Once merged, move the proposal folder into `openspec/changes/archive/` with the merge date.

`CLAUDE.md` is the authoritative description of this pipeline. When in doubt, defer to it.

## Pure docs, refactors, and tooling changes

Changes that do not alter observable behaviour (typo fixes, code formatting, README rewording, internal refactors that preserve every test) do not need an OpenSpec proposal. Open a PR with a clear description and move on.

## Dev setup

```bash
git clone https://github.com/dmastrorillo/tai.git
cd tai
go build ./...
go test ./...
```

You need a recent stable Go (anything still in support should work). No other external tooling required to build or test.

### Pre-commit hooks (recommended)

The repo ships a [`.pre-commit-config.yaml`](.pre-commit-config.yaml) that runs gofmt, `go vet`, golangci-lint, and Conventional Commits validation locally before every commit. CI runs the same checks, but the pre-commit hooks catch issues at the moment you make them, which is much cheaper than discovering them in a PR.

One-time setup:

```bash
# Install the pre-commit framework (one of these):
brew install pre-commit
# or:
pip install pre-commit
# or:
pipx install pre-commit

# Install golangci-lint (used by the lint hook):
brew install golangci-lint
# or:
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install the hooks into your local clone:
pre-commit install
pre-commit install --hook-type commit-msg
```

On Windows, `winget install pre-commit` or `scoop install pre-commit` also work; see the [pre-commit installation docs](https://pre-commit.com/#install) for other options. golangci-lint has a similar set: `scoop install golangci-lint`, `choco install golangci-lint`, or `go install ...@latest`.

The commitlint hook provisions its own Node environment automatically. You do not need Node installed globally.

After that, every `git commit` runs the formatting/lint/vet hooks on the staged Go files, and every commit message is checked against [`.commitlintrc.yml`](.commitlintrc.yml) for Conventional Commits conformance (including the enforced scope list).

To run all hooks against the whole repo on demand:

```bash
pre-commit run --all-files
```

Pre-commit hooks are not strictly mandatory (CI is the safety net), but installing them is the path of least friction.

## Local checks before pushing

Run these. CI runs the same plus `-race` and the linter.

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .   # output must be empty
```

`gofmt -l .` emits nothing when everything is formatted. If it lists files, run `gofmt -w <files>` (or your editor's formatter) and re-run.

`golangci-lint run` must be zero-issues. If it reports problems, fix them locally and re-run before pushing — the same command runs in CI. See [`.golangci.yml`](.golangci.yml) for any active exclusions.

## Where new code goes

- Core CLI behaviour (config, sync, repo init, workflow runner, standards loader, install-commands, plugin host) → `core/`.
- Shared framework code that plugin authors should be able to import → `pkg/`. Anything published here is on a stability contract: append-only error codes, no renaming or repurposing exported symbols without a major version bump.
- Anything plugin-specific (a SQLite schema, a domain model, plugin-only commands) → `plugins/<name>/`.
- Anything that no other module should touch → `core/internal/` or `plugins/<name>/internal/`.

When you are unsure which side of the line something belongs on, open an issue or draft a short OpenSpec proposal first. Putting code in the wrong place is one of the more expensive mistakes to undo.

## Commit and PR conventions

We use [Conventional Commits](https://www.conventionalcommits.org/). CI enforces this on every PR going forward. Commits that landed on `main` before this convention was introduced are not retroactively rebased; the rule applies to all new commits and PR titles from this point on.

Format:

```
<type>(<scope>): <imperative subject>

<optional body explaining the why>

<optional footer(s), e.g. "BREAKING CHANGE: ..." or "Closes #123">
```

Accepted types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.

Scopes are **required and enforced**. Every commit (and every PR title) must include a scope from this list:

| Scope | Use for |
| --- | --- |
| `core` | core/ CLI verbs that are part of the core binary |
| `pkg` | pkg/ public Go API (errcode, cliout, exitcode, taiplugin) |
| `triage` | the Triage plugin (or any plugin name when one is added) |
| `openspec` | proposals and specs under openspec/ |
| `ci` | CI workflows, lint configuration, release pipeline |

The list is deliberately small. Sub-areas of core (sync, repo, workflow, standards, plugin-host, install-commands, config) all use the `core` scope; the subject line carries the detail. Cross-cutting housekeeping picks the scope of the largest affected area (e.g. a Go-version bump that touches everything still goes under `core`).

Documentation-only commits use the scope of the area the doc describes. README, CONTRIBUTING, NORTHSTAR, CONTEXT, and CLAUDE.md changes use `core`. Changes purely under `openspec/` use `openspec`. Changes to GitHub workflows or lint config use `ci`. The type still distinguishes intent — `docs(core)` for documentation, `chore(core)` for housekeeping, `feat(core)` for new behaviour.

To add a new scope (typically when a new plugin lands), update the table above AND the `scope-enum` block in [`.commitlintrc.yml`](.commitlintrc.yml) AND the `scopes:` list in [`.github/workflows/commit-lint.yml`](.github/workflows/commit-lint.yml) in the same PR.

Examples:

```
feat(core): add --prune to `tai sync` to delete orphans tracked in the manifest
fix(triage): handle empty review body without panicking
docs(core): rewrite README contributing section to point at CONTRIBUTING.md
refactor(pkg): split storage codes into their own file
chore(ci): enforce Conventional Commits on PRs
```

Additional rules:

- Subject is imperative ("add foo", not "added foo" or "adds foo"). Sentence case after the type. No trailing period.
- Body explains the why; the diff explains the what.
- One logical change per commit when possible. Reformatting and rename commits should stand alone from behaviour commits.
- Breaking changes either go in a `feat!:` / `fix!:`-style subject or include a `BREAKING CHANGE:` footer.
- PR title follows the same Conventional Commits format because most merges produce a single commit with the PR title as the subject.
- PR description: link the OpenSpec proposal (or explain why none was needed), describe the user-visible change, and call out any deferred work.

There is no DCO sign-off requirement.

## Plugins

If you want to ship a third-party plugin, you do not need to commit anything to this repo. Build a binary that follows the plugin wire contract (env vars, error template, namespacing rules in [`README.md`](README.md#authoring-a-plugin)), publish a GitHub release with assets named `tai-plugin-<name>-<os>-<arch>`, and document the explicit source users should pass to `tai plugins <name> install --source <...>`.

To propose your plugin be added to the built-in first-party registry, open an issue with the **Feature request** template explaining the use case. The bar is high: the plugin needs to be useful to a broad audience and reliably maintained.

## Reporting bugs / requesting features

Use the issue templates:

- **Bug report** — for anything that does not behave the way the spec or README say it should.
- **Feature request** — for new behaviour, especially behaviour that touches the north star.

Pick the right one. Bug reports without reproduction steps and feature requests without a problem statement are the two things that take the longest to resolve.

## Security

If you find a security issue, do not open a public GitHub issue. Open a [private security advisory](https://github.com/dmastrorillo/tai/security/advisories/new) on the repository instead. The advisory thread is the right place to coordinate a fix, embargo, and disclosure timeline.

## License

By contributing, you agree your contribution is licensed under the project's [MIT license](LICENSE).
