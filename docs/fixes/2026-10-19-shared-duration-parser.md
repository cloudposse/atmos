# Fix: One duration format for every Atmos setting

**Date:** 2026-10-19

## Summary

All durations Atmos reads from configuration, flags, and environment variables now go through
`pkg/duration`. A bare number means seconds everywhere, Go durations (`500ms`, `1h30m`), days
(`2d`), keywords (`daily`), and ISO-8601 (`PT8H`) are accepted everywhere, and a YAML integer
in a retry delay no longer decodes as nanoseconds.

## Context

The v1.232.0 → main regression test (`.context/regression-test/2026-10-19-v1.232.0-main`,
finding R6) showed `timeout: 10` on a shell step failing with `missing unit in duration "10"`.
A survey of the code found four different behaviors for the same input:

- Step `timeout:`, `mcp.timeout`, `settings.pro.exec.sync_timeout`, the Terraform cache TTL,
  and the AWS and Azure session durations called `time.ParseDuration` directly, so a bare
  number was rejected.
- `retry.initial_delay`, `max_delay`, and `max_elapsed_time` were decoded by mapstructure's
  `StringToTimeDurationHookFunc`. A quoted `"10"` was rejected, but an unquoted YAML `10`
  decoded silently as ten **nanoseconds**, and the generated JSON schema documented that as
  the intended meaning.
- `version.check.frequency`, cache TTLs for remote imports, source provisioning, and workdir
  cleanup used the existing `pkg/duration.Parse`, which treated a bare number as seconds but
  rejected `500ms` and compound values such as `1h30m`.
- `aws/user` session durations used `pkg/auth/utils.ParseDurationFlexible`, which accepted
  seconds, Go durations, and days; Azure PIM (#3253) had its own ISO-8601 parser.

## Changes

- `pkg/duration` is now dependency-free (standard library plus mapstructure) so `pkg/schema`
  and the `errors` package can import it. It provides `ParseString`, `ParseAny` (numbers are
  seconds), `ParseDuration` (positive), `ParseNonNegative`, `Parse` (whole seconds, positive),
  `ParseISO8601`/`FormatISO8601` (moved from `pkg/auth/identities/azure`), and `DecodeHook`,
  a mapstructure hook that replaces `StringToTimeDurationHookFunc`. `errors.ErrInvalidDuration`
  aliases `duration.ErrInvalid`, so existing `errors.Is` checks keep working.
- Call sites migrated: step `timeout:` (`pkg/runner/step/timeout.go`), `spin`, `sleep`, `http`,
  and `cast` steps; the Terraform registry cache TTL; AWS SSO, SAML, assume-role, assume-root
  and `credential_process --min-validity`; Azure PIM activation duration and policy maximum;
  `pkg/auth/utils.ParseDurationFlexible` (now a thin wrapper). The decode hooks in
  `pkg/schema/retry_decode.go`, `pkg/schema/task.go`, and `pkg/config/load.go` use
  `duration.DecodeHook()`.
- The Azure package's private ISO-8601 parser and its tests moved to `pkg/duration`.
- JSON schemas: the generated `atmos.yaml` schema (`go generate ./pkg/config/schema`) and the
  hand-maintained stack-config and manifest schemas accept a number (seconds) as well as a
  string for retry durations.
- Docs: `website/docs/steps/retry.mdx` gains a "Duration values" section; the `timeout` fields
  of the shell and atmos steps, the Terraform component `retry` page, and the MCP `timeout`
  link to it.

## Editions

No edition entry. Accepting a bare number as seconds widens what strings are accepted, and
the only stored values whose meaning changes are unquoted integers in retry delays, which
previously decoded to nanoseconds, a value nobody can have relied on. Edition transitions
preserve intentional behavior; they do not preserve defects.

## Validation

- `pkg/duration` unit tests cover every accepted form, bare integers and floats as seconds,
  negative and zero handling, overflow, ISO-8601 parsing and round-tripping, and the decode hook
  (including the former nanosecond case: `timeout: 10` now decodes to `10s`).
- `go test` passes for `pkg/duration`, `pkg/schema/...`, `pkg/auth/...`, `pkg/terraform/cache`,
  `pkg/provisioner/...`, `pkg/stack/imports/...`, `pkg/config/...`, and `errors/...`.
- `pkg/runner/step`: every test passes except seven cast session-mode tests
  (`TestCastPlaybackSessionEndToEnd`, `TestCastHandlerExecutesSessionModeEndToEnd`, and
  five siblings) that time out waiting for the PTY helper. They time out identically on a
  pristine `HEAD` worktree on this macOS host with the machine idle, so they are unrelated to
  this change.
- `go generate ./pkg/config/schema` regenerated the `atmos.yaml` schema; the stack-config and
  manifest schemas were updated by hand and parse as JSON.
- `atmos lint --changed` reports no findings in the changed files. `pkg/duration` is added to
  the perf-tracking lint rule's exclusions because it must stay free of Atmos imports.
- End to end with a fresh build: a shell step with `timeout: 10` in a custom command and in a
  workflow runs; `timeout: 1s` on a three-second step still fails with `step timed out`.
- `cd website && npm run build` passes (pre-existing MDX-normalization warnings and AI-skill
  anchor warnings only).

## Follow-ups

The `Parse` (whole seconds) contract still rejects sub-second values for TTLs and the
version-check frequency; those settings never meant anything below one second.
