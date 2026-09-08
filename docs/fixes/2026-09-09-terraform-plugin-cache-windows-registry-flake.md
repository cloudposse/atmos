# Fix: `TestTerraformPluginCache` flake from a transient registry.terraform.io blip

**Date:** 2026-09-09

## Summary

The `Acceptance Tests (windows, shard 3/10)` CI job failed with
`--- FAIL: TestTerraformPluginCache`. The underlying `terraform init` call
failed with a genuine network error connecting to `registry.terraform.io`, not
a code regression. Added a bounded retry around the test's `terraform init`
helper to absorb this class of one-off CI network hiccup.

## Context

GitHub Job ID `102116372680`. The attached failure log's "last 1000 lines"
window contained only StepSecurity harden-runner post-job diagnostic noise
(`pid reused`, orphan-process cleanup) — the real failure had already scrolled
past it, same pattern as
[`2026-08-31-terraform-registry-cache-windows-runner-degradation.md`](2026-08-31-terraform-registry-cache-windows-runner-degradation.md).
Pulled the full log via `gh api repos/cloudposse/atmos/actions/jobs/102116372680/logs`
and found:

```
--- FAIL: TestTerraformPluginCache (38.71s)
    cli_plugin_cache_test.go:57: Failed to run terraform init component-a -s test: exit status 1
    Terraform init stderr:
      Error: Failed to query available provider packages
      Could not retrieve the list of available versions for provider
      hashicorp/null: could not connect to registry.terraform.io: failed to
      request discovery document: GET
      https://registry.terraform.io/.well-known/terraform.json giving up after 4
      attempt(s): context deadline exceeded
```

This is not an egress-allowlist problem — `registry.terraform.io:443` is
already in the `test` job's `harden-runner` `allowed-endpoints` list in
`.github/workflows/test.yml`. It's also not the whole-runner degradation
pattern from the 2026-08-31 incident above: that run had every step (dependency
download, test execution, cache save) uniformly ~10x slower and the job hit
its own timeout; this run's test failed fast (38.71s total, well inside the
4-minute per-`terraform-init` timeout) with a real, immediate connection
error from Terraform's own DNS/TLS layer — a one-off blip reaching
`registry.terraform.io` from that specific Windows runner during that
specific window, not a systemic slowdown. `head_sha` for this job
(`8d999050839adc94884d4442568a274d97a0d395`) is unrelated to this test: that
commit only touched `cmd/secret/shared.go`/`shared_test.go`.

`TestTerraformPluginCache` (and its five siblings in the same file) call real
`terraform init` against the real `registry.terraform.io` with zero retry —
any transient DNS/TLS failure fails the test outright, unlike production
`atmos terraform` invocations, which have `ExecuteShellCommandWithRetry`
available for exactly this kind of recoverable subprocess error (opt-in via a
component's `retry:` stack config, not applicable here since these tests
shell out to the compiled `atmos` binary directly rather than going through a
stack with a `retry:` section).

## Changes

- `tests/cli_plugin_cache_test.go`: `runTerraformInitWithEnv` (used by all six
  `terraform init` call sites in this file) now retries within a new
  `terraformInitRetryBudget` (90s) using the existing `pollUntil` helper
  (already used elsewhere in the `tests` package, e.g.
  `tests/floci_harness_test.go`'s Floci endpoint readiness check), instead of
  failing on the first error. A real failure (bad fixture config, missing
  provider) fails identically on every attempt and still fails the test once
  the budget is spent — this only absorbs a one-off network hiccup.

## Verification

- `go build ./tests/...` — passed.
- `go test ./tests/... -run '^TestTerraformPluginCache$' -v -timeout 90s` —
  passed locally (37.30s, real network access to `registry.terraform.io`),
  confirming the retry wrapper doesn't change the happy-path behavior or
  timing materially.
- `atmos fix lint` (patch-scoped, this repo's real PR gate) — passed, 0 issues.

## Follow-ups

None. If this recurs frequently enough to suggest a systemic (not one-off)
registry connectivity problem from Windows runners, revisit — e.g. a local
Terraform registry mirror for this fixture, similar to
`terraform-registry-cache`'s approach elsewhere in the suite.
