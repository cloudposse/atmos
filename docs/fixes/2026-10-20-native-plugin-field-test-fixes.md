# Fix: Native plugin field-test findings

**Date:** 2026-10-20

## Summary

A hands-on field test of the `atmos plugin` family (native Go plugins, OCI install, lockfile, policy,
registry auth) turned up silent state changes, cache and journal recovery gaps, an orphaned plugin
process, weak error messages, and doc drift. This change fixes them on the branch. It also adds
publish-time content guards so a published image can only ever contain the plugin manifest, its
schemas, and the declared platform executables.

## Context

The plugin system is not merged into `main`, so everything is fixed in the active branch and nothing
is tracked in GitHub issues. Findings came from running the built CLI against a local OCI registry
with a scaffolded OIDC plugin, plus code reading. Each fix below was written test-first and
re-verified by hand against a rebuilt binary.

Findings are numbered as in the field-test report.

## Changes

**Published image contents (never `.git`, `.terraform`, `.atmos` credential caches, `.env`, keys)**
- `pkg/plugin/oci` gained a bundle path guard (`bundleguard.go`). It rejects hidden path segments,
  credential/key/state-like names, symlinks and special files. It applies to the executable of every
  platform, every schema file, every tar entry at publish time, and every entry at extract time.
- `binaryformat.go` checks that each platform executable is a native Mach-O, ELF or PE binary for the
  declared OS and architecture. `pack` and `publish` enable it; it is opt-in for library callers.
- `Publish` re-inspects every platform layer (streaming, bounded) before any registry write, so a
  hand-built layout cannot smuggle files in. `plugin validate` runs the path guard too.
- `pack` and `publish` print the per-platform file inventory (table rows, JSON/YAML `files`).
- Not done: scanning manifest and schema text for secrets. See Follow-ups.

**Silent mutation**
- F1: re-running `plugin install <src>` no longer widens a constraint to `*`. A new declaration is
  pinned to the installed major (`^1.4.2`, `^0.1.0` for 0.x, exact for pre-releases), and the
  "already up to date" feedback now appears without `--as`.
- F2: ordinary commands install a plugin only when they use a plugin-provided auth kind. Unlocked
  plugins are refused in CI, and every automatic install prints source, digest and signature status.
- F3: `install --dev` rejects `--version`, `--from-layout` and `--frozen`, reports success, and
  discloses the `.gitignore` edit. Policy denial uses the new `ErrPluginDevForbidden`.
- F10: path-like sources fail locally with a `--dev` hint instead of querying catalogs.
- F13: `plugin install` skips `enabled: false` packages. F15: empty `source`/`version` is rejected
  up front.
- F7/F9: a corrupt cached release (appended byte, extra file, changed mode, deleted file) is
  quarantined and re-fetched by the locked digest. `--frozen` and offline runs fail with hints
  without mutating anything. `plugin list` shows `corrupt`.
- F8: an unparseable `transaction.json` is moved aside and the install proceeds. A parseable but
  conflicting journal is preserved and now has a precise hint.
- F6: SIGINT and SIGTERM stop the plugin (process group, graceful then forced) before Atmos exits.
  Call deadlines produce `ErrPluginTimeout`. New optional `plugins.packages.<alias>.timeout`.
- Config edits (`install`, `update`, `remove`, `registry add/remove`) use a byte-splice editor that
  preserves comments, blank lines and block style.

**Publish, delete, flags, output**
- `publish --to` and the signing-only flags are validated before any push; `delete` requires `--yes`
  non-interactively and confirms interactively.
- `registry add` honors `-i` and `ATMOS_IDENTITY`. `registry login` refuses `--password-stdin` on a
  TTY and the memory/noop keyrings. Unknown registries use `ErrPluginRegistryNotFound`.
- `plugin test` failures show expected versus actual detail (text and JSON), `--run` with no match
  fails and lists test names, and the failure is printed once.
- Concurrent install reports every failing package in alias order, holds success lines until the
  outcome is known, and says "No plugin changes were written." on failure.
- Catalog commands normalize a leading `v`, require a tag or digest, and explain `oci://`.
- `list` and `info` validate config like ordinary commands.
- Many bare `ErrPluginInvalidConfig` returns now carry actionable hints.

**Policy decision**
- `plugin build` is not a policy operation. An early draft added one, and a broad `on: plugin` rule
  that rejects `origin: development` then blocked compiling a plugin, which admits nothing and runs no
  plugin code. It was removed. `plugin test` keeps its `test` operation.

**Docs and tests**
- Plugin command pages, a new `plugins` configuration page, the policy pages, PRD command-surface
  claims, stale `pkg/automation` references and the help screengrabs were updated.
- Tests that touched the real macOS keychain now use an isolated keyring (`credtest`), applied in
  18 packages.
- A flaky timeout test in `pkg/plugin/runtime/native` was loosened (500ms to 3s) and timeout
  detection now also accepts a gRPC `DeadlineExceeded` status when the call carried a deadline.

## Validation

Ran and passed:
- `go build ./...`
- `go vet` on `pkg/plugin`, `cmd`, `pkg/ui`, `pkg/oci`, `pkg/auth`
- `go test ./pkg/plugin/... ./pkg/ui/... ./pkg/oci/... ./pkg/policy/... ./pkg/config/... -short -count=1`
- `go test ./pkg/auth/... -short -count=1`
- `go test ./cmd -run 'Plugin|Policy|Auth' -short -count=1`
- `go test ./pkg/plugin/runtime/native -run TestCallerDeadlineIsReportedAsPluginTimeout -count=25 -race`
- Website build succeeded.
- Manual re-verification against a rebuilt binary and a local registry: pin default, idempotent
  re-install, no network for commands that need no plugin, path-source error, `--dev` conflicts,
  publish pre-flight, delete confirmation, tamper detection and repair (appended byte, extra file,
  chmod), corrupt journal recovery, hostile-bundle rejection, test failure detail, `--run` no-match,
  multi-error batch output, and SIGTERM stopping the plugin.

Not run or not verified:
- `plugin sign` and `publish --sign` (they write to the public transparency log).
- Behavior against real registries and cloud credentials.
- Live pty rendering of the repair warning inside the batch progress UI.
- `golangci-lint` (blocked in the agent sandbox) and the full `./tests` suite.
- A wide run of `pkg/runner/step` showed Cast* timeouts that were not investigated; whether they
  predate this change is unknown.

## Follow-ups

Remaining work stays in `docs/prd/plugins/` and the branch task list, not in issues:
- A company-registry rule that rejects `origin: development` also blocks `plugin test`. That is intended:
  rules can now allow local development through `input.origin`, `input.operation`, `input.path` and
  `input.profiles` (documented in the plugin policy page and the local-development tutorial).
- `plugin run` for a `path` package whose executable is missing reports "plugin is not installed" in its
  headline; the hint below it is accurate. The headline should name the missing executable.
- A plugin whose host dies from a closed pipe (SIGPIPE) is left running with PPID 1, because that signal
  skips the exit cleanup. A parent-death watchdog in the plugin SDK, or handling SIGPIPE in the host,
  would close it.
- Secret scan of manifest and schema text (`ErrPluginBundleSecretDetected` exists but is unused). The
  repo's output masker has no reusable, rule-identified detection API.
- Plugins run with the cache payload as their working directory, so a plugin that writes there is
  flagged as tampered on its next run. Run plugins from the user's directory instead.
- Not addressed from the field test: credential-resolver mutex held across interactive prompts,
  missing browser-login fallback URL, `whoami` showing a revoked token as valid, `go.sum` for the
  released-SDK scaffold path, Windows executable naming, hashing every cached bundle on each
  command, and the double layer download on registry installs.
- `plugin create --capability a --capability b` fails with a bare error and no hint, and the usage
  text for `plugin run <alias> -- ...` does not say that arguments belong to the plugin command.
- `plugin publish` no longer has a `--catalog` flag in this tree (removed by another session); the
  docs match the code.

## Later changes

The F3 `install --dev` fixes above describe the surface as it was fixed. That surface was removed afterwards:

- `atmos plugin install --dev`, `atmos plugin remove --dev` and the machine-local `.atmos/plugins/dev.yaml`
  (including its `.gitignore` edit) were replaced by `plugins.packages.<alias>.path`. A developer sets `path`
  in a profile of their own choosing, so the profile deep-merge is the override mechanism. `source` and
  `version` stay valid beside `path` and apply when the profile is not active.
- `plugins.policy.allow_dev_overrides` was removed. Policy rules over `origin: development` and the new
  `input.path` (plus `input.profiles`) are the gate for local builds and `plugin test`;
  `plugins.policy.require_signature` governs released packages only because a `path` package has no signature.
- Activating a path package prints one warning per alias, `plugin list` and `plugin info` report state `path`
  with the name and version from the local manifest, `install --frozen` refuses while the active configuration
  sets `path`, and `remove` explains that the path comes from configuration.
