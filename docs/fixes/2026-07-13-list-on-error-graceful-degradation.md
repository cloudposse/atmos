# List/Describe Graceful Degradation (v2)

## Problem

`atmos list stacks`/`list components`/`list settings` aborted the entire command the instant
any one component's `!terraform.state`/`!terraform.output` failed to resolve — most commonly
because the component's Terraform backend had not been provisioned yet. One unprovisioned
component made it impossible to list every other, already-provisioned stack. The same failure
mode affected `describe stacks`, `describe affected`, `list affected`, and `describe
dependents`, all of which process YAML functions by default.

The only workaround, `--process-functions=false --process-templates=false`, disabled all
function/template rendering — losing every computed column, not just the unresolvable ones.

## Root Cause

`internal/exec/yaml_func_utils.go`'s `processNodesWithContext` — the single recursive walker
used by every YAML function type — treated the first per-value error as fatal for the whole
call: it set `firstErr` and unwound without processing any remaining keys, stacks, or
components.

## Fix (v2)

Graceful degradation is now **on by default** (`--error-mode=warn`) for `list stacks`, `list
components`, `list settings`, `describe stacks`, `describe affected`, `list affected`, and
`describe dependents`. Three modes are supported:

- `warn` (default) — a recoverable error (backend/state not yet provisioned, per
  `isRecoverableTerraformError`, sentinels `ErrTerraformStateNotProvisioned` /
  `ErrTerraformOutputNotFound`) is substituted with `degradation.AtmosComputedValue{}` and
  processing continues with the next key, component, and stack. A single summary warning is
  printed at the end of the command (not one per degraded value). Full per-value detail
  (stack/component/function/reason) is always logged via `log.Debug`, visible with
  `--logs-level=Debug`.
- `silent` — identical substitution and debug logging as `warn`, but no end-of-command summary
  is printed.
- `strict` — the v1 behavior: the first unresolvable value fails the command immediately.

Every other error class — auth failures, malformed YAML, misconfiguration — still fails the
command exactly as before, in all three modes.

`degradation.AtmosComputedValue{}` (`pkg/degradation`) replaces v1's raw `nil` substitution. It
renders as the literal `"(computed)"` in every output path — table (via `fmt.Stringer`, picked
up automatically by both `pkg/list/format/table.go`'s cell formatter and the Go-template column
pipeline used by `list stacks`/`list components`), JSON (`json.Marshaler`), and YAML
(`yaml.Marshaler`) — instead of rendering inconsistently as `<nil>`, `""`, or `null` depending on
the output path.

`internal/exec`'s `OnErrorMode` stays two-valued (`OnErrorStrict`/`OnErrorWarn`) — `silent` is a
cmd-layer-only distinction: `warn` and `silent` both set `OnError: OnErrorWarn` and wire the same
`degradation.Collector.Add` callback; they differ only in whether the cmd-layer caller invokes
`Collector.Summary()` after rendering. `ErrorOptionsFromMode`/`PrintErrorModeSummary`
(`internal/exec/describe_stacks.go`) are the single canonical conversion shared by every command
above.

The flag was renamed from `--on-error` to `--error-mode` (env `ATMOS_LIST_ERROR_MODE` for `list`
commands, `ATMOS_DESCRIBE_ERROR_MODE` for `describe` commands) to match the existing
`--failure-mode=fail-fast|keep-going` naming convention used by `atmos terraform
{plan,apply,destroy,deploy}` for an analogous "how should failures be handled" policy — kept as a
separate flag (different vocabulary, different mechanism) rather than merged with
`--failure-mode`.

The project-wide `atmos.yaml` default is deliberately two separate settings —
`list.error_mode` and `describe.error_mode` — rather than one shared value: `list` and
`describe` are independent command families (e.g. `describe` is more likely driven by CI,
`list` by interactive use) and may want independent defaults. `exec.ResolveErrorMode` takes
the already-selected setting value as a plain parameter rather than reading a shared field
itself, so each call site resolves against its own family's config
(`atmosConfig.List.ErrorMode` vs. `atmosConfig.Describe.ErrorMode`).

This deliberately does *not* reuse or reverse the auth fail-fast decision described in
[list-instances-auth-fail-fast.md](2026-05-15-list-instances-auth-fail-fast.md): that fix made a
fatal *misconfiguration* (a declared default identity that isn't configured) fail fast instead of
silently falling back to the wrong identity. This fix only degrades a fundamentally different
failure class — *infrastructure not yet provisioned* — and defaults to doing so, with an explicit
opt-out (`--error-mode=strict`) for callers that want the old fail-fast behavior.

## Expected Behavior

- `atmos list stacks` / `list components` / `list settings` / `describe stacks` / `describe
  affected` / `list affected` / `describe dependents` with no `--error-mode` flag (or
  `--error-mode=warn`): an unresolvable `!terraform.state`/`!terraform.output` value becomes
  `(computed)` in table/JSON/YAML output, a single summary line is printed to stderr naming how
  many values were affected, and the rest of the command completes normally with exit code 0.
- `--error-mode=silent`: same substitution, no summary line. `--logs-level=Debug` still shows
  per-value detail.
- `--error-mode=strict`: the first unresolvable value aborts the command, matching pre-v1
  behavior.
- Errors unrelated to backend provisioning (auth, malformed YAML, etc.) still fail the command
  in all three modes.
- `describe component` is **not yet covered** — deferred as a follow-up (structurally different:
  a 4-layer `ExecuteDescribeComponent → ExecuteDescribeComponentWithContext → ProcessStacks →
  ProcessComponentConfig` call chain with no existing `onWarning` threading, and
  `ProcessStacks`/`ProcessComponentConfig` have other callers beyond this command).
