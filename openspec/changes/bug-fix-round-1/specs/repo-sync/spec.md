## MODIFIED Requirements

### Requirement: Background fetch poll surfaces source-repo updates

The system SHALL run a non-blocking background goroutine on every TAI invocation that, when the cached update-check is stale (older than `update-check-interval`), performs a remote-ref check against the source repo's default branch and records the result into `<TAI_DATA_DIR>/state/update-check.json`. The recorded state is consumed by the update-banner capability.

The background check MUST NOT delay command completion. If it errors (network unreachable, auth failure, 5xx), the failure is silently absorbed: the cache file is not modified, no error is written to stdout or stderr, and the next invocation retries per the cadence rule.

The background check SHALL run all `git` invocations with the following environment variables set in addition to the inherited environment, so that git fails fast on missing credentials instead of prompting interactively:

- `GIT_TERMINAL_PROMPT=0` — disables git's built-in terminal credential prompt.
- `GIT_ASKPASS=/bin/echo` — overrides any inherited `GIT_ASKPASS` to a non-interactive no-op.
- `GCM_INTERACTIVE=Never` — instructs Git Credential Manager (when installed) to skip interactive flows.

These env vars MUST NOT propagate to foreground git invocations (e.g. those executed by `tai sync`'s eager fetch, the initial clone, or any other foreground operation), so that the user retains the ability to authenticate interactively on the foreground path. Credential helpers (osxkeychain, `gh auth setup-git`, `.netrc`, cached Git Credential Manager tokens) continue to operate normally on the background path because they resolve BEFORE git's interactive prompt fallback.

When credentials are unavailable to the background poll (e.g. a private HTTPS source repo with no helper), the poll SHALL fail silently per the absorption rule. No prompt is shown, no banner row is suppressed (the cache simply isn't updated), and the next invocation retries.

#### Scenario: Stale-cache poll refreshes the state file

- **WHEN** `<TAI_DATA_DIR>/state/update-check.json` has a timestamp older than `update-check-interval` and the configured source repo is reachable with valid credentials
- **AND** the user runs any TAI command
- **THEN** the foreground command completes without blocking on the poll
- **AND** within a short bounded wait after exit, `<TAI_DATA_DIR>/state/update-check.json` has a timestamp newer than the stale timestamp

#### Scenario: Fresh-cache poll does not touch the state file

- **WHEN** the cache file's timestamp is within `update-check-interval`
- **AND** the user runs any TAI command
- **THEN** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

#### Scenario: Poll error is silently absorbed

- **WHEN** the cache file is stale and the configured source repo is unreachable
- **AND** the user runs any TAI command
- **THEN** the foreground command's stdout and stderr contain no error or warning attributable to the background poll
- **AND** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

#### Scenario: Background git invocation does not prompt for credentials

- **GIVEN** the configured `repo-url` is an HTTPS URL to a private repository AND no credential helper is configured
- **WHEN** the user runs any TAI command and the background poll fires
- **THEN** no `Username for ...` prompt is written to any stream
- **AND** stdin is not read by the background process
- **AND** the cache file is not updated (the poll fails silently per the absorption rule)
- **AND** the user's keystrokes intended for the foreground command are not consumed by git

#### Scenario: Foreground sync still prompts when interactive auth is needed

- **GIVEN** the configured `repo-url` is an HTTPS URL to a private repository AND no credential helper is configured
- **WHEN** the user runs `tai sync` in a TTY
- **THEN** the foreground `git fetch` (or `git clone` on first sync) MAY prompt for credentials normally
- **AND** the user can complete authentication interactively
