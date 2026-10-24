# Fix: `atmos exit code should be same as command exit code (2)` failed on a transient registry outage, not a code bug

**Date:** 2026-08-31

## Summary

`Acceptance Tests (linux, shard 9/10)` failed with:

```text
--- FAIL: TestCLICommands/atmos_exit_code_should_be_same_as_command_exit_code_(2) (13.04s)
    cli_test.go:1569: Reason: Expected exit code 2, got 1
    cli_test.go:1363: Description: Ensure the exit code equals the command exit code for a passing terraform plan (expected to be 2)
```

Captured stderr showed the real cause was never in Atmos:

```text
Error: Failed to resolve provider packages

Could not resolve provider hashicorp/null: could not connect to
registry.opentofu.org: failed to request discovery document: Get
"https://registry.opentofu.org/.well-known/terraform.json": context
deadline exceeded
```

## Context

`tests/test-cases/exec-command.yaml`'s `(2)` case runs a real, uncached
`atmos terraform plan component1 -s test -- -detailed-exitcode` against
`tests/fixtures/scenarios/exitCode` and expects OpenTofu's own
`-detailed-exitcode` exit code of `2` (diff present). That fixture's
`atmos.yaml` has no provider-mirror/registry-cache configuration, so `tofu
init` resolves `hashicorp/null` directly from `registry.opentofu.org` on
every run -- by design for this minimal fixture, not a gap introduced by
this branch.

On this run, the discovery-document request to `registry.opentofu.org`
itself timed out (`context deadline exceeded`) before `tofu init` could even
start downloading the provider. `tofu` correctly failed with a generic exit
code `1`, and Atmos correctly propagated that exit code -- the test's
expectation of `2` assumes `init` succeeds and only the plan diff drives the
exit code, which is a reasonable assumption that a registry outage breaks
regardless of anything in this codebase.

Checked and ruled out as codebase causes:
- No provider-mirror/network-mirror config exists anywhere in
  `.github/workflows/` or the `exitCode` fixture that this run could have
  regressed.
- Atmos's own `terraform cache`/registry-mirror feature (see
  `docs/fixes/2026-08-25-provider-mirror-concurrency-test-flake.md` for a
  prior, code-fixable flake in that subsystem) is opt-in and not wired into
  this fixture at all -- this test has always talked to the real registry.
- No test-framework-level retry mechanism exists for network-dependent CLI
  acceptance tests in `tests/cli_test.go` (the only `retry` support in
  `tests/test-cases/*.yaml` is Atmos's own workflow-retry *feature* under
  test, unrelated to the test harness retrying itself).

## Changes

None. There is no code, test, or configuration change in this repository
that fixes a transient DNS/network failure reaching a public,
third-party registry from a CI runner. Making a speculative change here
(e.g., loosening the exit-code assertion) would mask a real regression in
the CLI's exit-code propagation if one ever occurs, which is exactly the
property this test exists to catch.

## Validation

- Confirmed via the fixture's `atmos.yaml` and a repo-wide grep that no
  registry-mirror/cache config applies to this test, ruling out a
  configuration regression.
- Confirmed no other subtest in the same shard's run failed from the same
  cause (the sibling `(0)` and `(1)` exit-code cases in the same file
  passed), consistent with an isolated, transient outage rather than a
  systemic issue.

## Follow-ups

None. Re-running the failed CI job is expected to pass; no issue is being
opened since there is no actionable code change to track.
