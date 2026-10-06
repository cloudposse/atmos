# Fix: Starlark script steps stream output, honor dry-run, and work in every execution context

**Date:** 2026-10-03

## Summary

A hands-on field test of embedded Starlark (`type: script`, `interpreter: starlark`) across workflows, custom
commands, hooks, and `type: test` found a dry-run leak, missing component context, buffered output, noisy
errors, and a 15-second startup stall in terminals that never answer color queries. This change fixes all of
them, documents the resulting behavior, and adds portable CLI regression tests.

## Context

Findings came from running a real `atmos` binary against a dedicated fixture
(`tests/fixtures/scenarios/starlark-field-test/`). The most severe issues were:

- `atmos workflow --dry-run` executed Starlark children of `parallel`/`matrix` steps for real.
- `type: script` steps used the `log` output mode, which buffered all output until the step finished.
- Atmos took ~16.5s to start in a PTY that never answers terminal queries. Goroutine dumps showed three
  OSC color queries during package init, each waiting termenv's 5s timeout: one from bubbletea's `init`, two
  from `charmbracelet/log.Default()` building a color-cached renderer on stderr (`pkg/logger/global.go`).
- `ctx.component` and `components.get` were unavailable in `parallel`/`matrix` children and in hooks.
  `ctx.component` resolved eagerly, evaluating component YAML functions twice.
- `{{ }}` in `script:` was not rendered in control children.
- Direct workflow script steps ignored `retry:`.
- Custom-command `working_directory` was ignored by control children.
- Starlark's default dialect rejected top-level `if`/`for`, so the natural test assertion
  `if x: fail(...)` failed.
- Error messages repeated "starlark execution failed" up to six times, and markdown flattened tracebacks.

User decisions: keep `retry.Do` and unlimited-retry defaults unchanged. Enable top-level control flow, `while`,
`set()`, and recursion, but keep globals single-assignment, with a hint instead. Pass string `output` through
raw. Use `exec.run(output="stream"|"capture", check=True|False)`, and drop the unreleased `stream=` alias.
Allow absolute and `..` paths consistently.

Path rules, agreed after reviewing `docs/prd/base-path-resolution-semantics.md` and
`docs/prd/git-root-discovery-default-behavior.md`:
- **Workflow and custom-command `!include`.** Previously undocumented there, and workflow manifests resolved
  against the process CWD, which broke the "run from any subdirectory" MUST. They now follow the documented
  `!include` rules: `./x` is manifest-relative and `x/y` is project-root-relative.
- **`load()`.** Inside a file it is relative to that file. Inside an inline `script: |` it is relative to
  `working_directory`.
- **`fs.read_file` and `exec.run`.** Both stay relative to `working_directory`.
- **Rejected:** a new `//` root-label convention, and a settable `exec.working_directory`.

Hook scripts carry `!include` provenance in-band (`script_source` plus `script_source_sha256`). This matches how
`describe` already shows provenance fields such as `component_info` and `sources`.

The bubbletea v2 migration is out of scope by decision. With the logger fix and Atmos PTYs answering terminal
queries, only external harnesses that never answer still wait 5s. The documented workaround for them is
`TERM=screen-256color`.

## Changes

- **Streaming:** `pkg/runner/step/output_mode.go` log mode forwards each complete line through `data.Write` /
  `ui.Write` as it arrives, keeping masking and header/footer. Captured values stay byte-identical.
- **Startup:** `pkg/logger` binds charm loggers to a non-`*os.File` stderr wrapper with an explicit color
  profile, so termenv never queries the terminal. This removes 10s of the 15s stall.
- **Dry-run:** `ControlCommandExecutor.DryRun` gates every child type in `pkg/workflow/control_executor.go`.
  Embedded children are parsed for syntax errors but never executed. Top-level Starlark dry-run also parses
  only.
- **Component context:**
  - `ctx.component` is lazy and memoized (`pkg/script/starlark/lazy_component.go`), and `components.get` is
    memoized per run.
  - Control children receive the component ref, resolver, process overrides, and hook context.
  - Hooks install a component resolver (`pkg/hooks/script_context.go`, `ExecContext.ComponentResolver`).
  - Component handles print compactly.
- **Templating and containers:**
  - `resolveControlStep` renders `script` and `interpreter`.
  - Container checks use the rendered interpreter via the `script.Get` registry.
  - Workflows and custom commands reject embedded interpreters under an enabled container before any step
    runs (`pkg/workflow/embedded_validation.go`).
- **Workflow parity:**
  - Direct workflow script steps run under `retry.Do` with toolchain PATH and the auth manager.
  - Custom-command `working_directory` is passed to control children (`internal/exec/custom_command_control_adapter.go`).
- **Engine (`pkg/script/starlark`):**
  - Dialect options with a "globals are assigned once" hint.
  - Raw string `output`, and `ErrStarlarkOutputEncode` for non-encodable values.
  - `exec.run`/`component.exec` gain `output=`/`check=`, the stderr tail in failures, and
    "failed to start" for start errors.
  - Per-task line-atomic, prefixed output in `steps.parallel`.
  - Fail-fast missing `working_directory`; consistent absolute/relative path policy.
- **Errors:** the `ErrStarlark` sentinel is joined once, with the backtrace in a fenced explanation.
  - New sentinels: `ErrStarlarkInvalidArgument`, `ErrStarlarkTaskTimeout`, `ErrStarlarkProcessFailed`,
    `ErrStarlarkOutputEncode`.
  - The workflow step error omits the empty "command failed" block for script steps.
  - Wrong-case interpreter names get a "Did you mean" hint.
  - Loose `require.Error` assertions now use `require.ErrorIs`.
- **Path provenance:**
  - `pkg/yaml/includescope` and `pkg/workflow/manifest.go` resolve workflow `!include` per the rules above
    and record `WorkflowStep.ScriptSource`.
  - `pkg/config/include_scope.go` does the same for custom commands, honoring `--base-path` /
    `ATMOS_BASE_PATH`.
  - The stack-manifest include path injects `script_source` + `script_source_sha256` next to a hook step's
    `script`. `pkg/hooks/step_script_source.go` applies the source only when the pre-render script still
    matches the hash, so a stale inherited source is ignored.
  - `script.Spec.SourcePath` makes `load()` file-relative and tracebacks name the file. `ProjectRoot`
    renders those paths project-relative.
  - The cast demo's `load()` was updated.
- **Masking:** `pkg/io` masker caches its sorted literals and compiled regexes, and gains
  `HoldbackLen` + `NewStreamingMaskWriter`, so a secret split across writes is never emitted. This applies to:
  - log, raw, and viewport output modes;
  - shell steps;
  - `ExecuteShell` / `ExecuteShellCommand`;
  - custom-command and workflow shell steps;
  - the docs pager, git stderr capture, and the PTY recorder.

  Whole-record writers (the logger, bubbletea frames, scheduler buffers) keep `MaskWriter`.
- **`log` builtin:** `log.trace/debug/info/warn/error(message, **fields)` writes to the Atmos logger. It
  adds `step=`/`task=` fields automatically, masks values, and never writes stdout. The convention is: `print`
  is data, `ui.*` is status, `log.*` is diagnostics.
- **`type: test` summary:** `TestFailureError` reports `N of M tests failed` and matches `ErrTestsFailed`.
  Leaf errors stay reachable through `errors.Is`/`As`.
- **PTY replies:** the new `pkg/terminal/query` answers OSC 10/11 and CSI 6n for both the asciicast recorder
  and `pkg/terminal/pty`. `website/docs/cli/configuration/settings/terminal.mdx` documents
  `TERM=screen-256color` for harnesses that never answer.
- **Cleanup:** the `exec.run(stream=...)` alias is removed. The superseded `starlark-field-test` fixture is
  deleted. A dry-run test no longer shells out to `sh`.
- **Docs:** `script.mdx`, `include.mdx`, `atmos-starlark`, `atmos-hooks`, and `atmos-custom-commands`
  skills, plus the PRD, describe the new behavior. The broken `ctx.component` workflow example is fixed.
- **Regression tests:** `tests/fixtures/scenarios/starlark-steps/` and `tests/test-cases/starlark-steps.yaml`
  (20 portable cases at first; later extended with `!include`/`load()` path cases run from subdirectories,
  task timeout, hook provenance and stale-inheritance, and `log` level filtering), plus unit tests in every
  touched package.

## Validation

- `go build ./...` passed.
- `go test -race -count=1 ./pkg/script/... ./pkg/runner/step/... ./pkg/workflow/... ./pkg/hooks/... ./pkg/logger/... ./errors/...` passed.
- `go test -count=1 ./internal/exec/ ./cmd/... ./pkg/retry/...` passed (64 packages ok, 0 failures).
- `go test -count=1 ./tests -run 'TestCLICommands/starlark'` passed (20 cases; snapshots generated with
  `-regenerate-snapshots`).
- `go test ./tests -run 'TestCLICommands/.*(workflow|custom|script|command|step|shell|Workflow).*'` passed
  with no golden changes after the streaming change.
- PTY timing (Python `pty` harness that never answers queries): before, 3 queries and 17.4s; after, 1
  query (bubbletea) and 7.3s. When the harness answers: 2.7s.
- PTY streaming: `type: script` lines (sh and Starlark) now arrive about 0.7s apart as produced. Before,
  they arrived all at once at step end.
- Field-test matrix re-run on the combined binary. Dry-run leaves `.probe/` empty. Templates render in
  children. `components.get` works in children and hooks. The component's `!exec` is evaluated once.
  Error, container, and working-directory cases behave as documented.
- `./custom-gcl run --new-from-rev=origin/main` on the touched packages showed no findings in changed
  files. Remaining findings are in unmodified files that differ only because `origin/main` advanced.
- `cd website && npm run build` passed with no broken links. One later doc edit, which only moved a
  section, was not rebuilt.
- Final integration pass, after all changes:
  - `go build ./...` passed.
  - `go test -race -count=1` passed across `pkg/script`, `pkg/runner/step`, `pkg/workflow`, `pkg/hooks`,
    `pkg/logger`, `pkg/io`, `pkg/terminal`, `pkg/asciicast`, `pkg/utils`, `pkg/config`, `pkg/schema`,
    `pkg/yaml`, `pkg/git`, and `errors`.
  - `go test -count=1 ./internal/exec/ ./cmd/...` passed.
  - `go test -count=1 ./tests -run 'TestCLICommands'` (the full CLI suite) had one failure:
    `atmos_list_instances`. Its TTY golden had recorded the three startup terminal queries that the logger
    fix removed. That snapshot was regenerated with `-regenerate-snapshots` and now records only
    bubbletea's query. The case passes.
- Masking: secrets split at every byte offset across writes are never emitted, in every streaming mode.
  `Mask` benchmark on ~1 KB: 56.9µs → 10.5µs with 10 literals, 2.29ms → 0.66ms with 1000.
- `./custom-gcl run --new-from-rev=origin/main` on every touched package: 0 findings in changed files. The
  10 remaining findings are in files unchanged on this branch.
- Not run: Windows CI. The new CLI cases avoid shell binaries.

## Follow-ups

None.
