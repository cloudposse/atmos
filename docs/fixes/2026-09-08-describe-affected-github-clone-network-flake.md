# Fix: `TestDescribeAffectedWithTargetRefClone` flake from a transient github.com blip

**Date:** 2026-09-08

## Summary

The `Acceptance Tests (windows, shard 7/10)` CI job failed with
`--- FAIL: TestDescribeAffectedWithTargetRefClone`. The real GitHub clone this
test performs failed with a `*net.OpError` reaching `github.com`, not a code
regression. Added a bounded retry around the clone call to absorb this class
of one-off CI network hiccup — the same pattern used for
`TestTerraformPluginCache`'s `registry.terraform.io` flake earlier the same
day (see
[`2026-09-08-terraform-plugin-cache-windows-registry-flake.md`](2026-09-08-terraform-plugin-cache-windows-registry-flake.md)).

## Context

GitHub Job ID `102144896568`, run `34249365397`, head sha `3e98277092`
(unrelated to this test: that commit only touched `cmd/secret/*` and the
homebrew skill doc). Pulled the full log via
`gh api repos/cloudposse/atmos/actions/jobs/102144896568/logs`:

```text
--- FAIL: TestDescribeAffectedWithTargetRefClone (0.18s)
    describe_affected_test.go:53:
        Error Trace: D:/a/atmos/atmos/pkg/describe/describe_affected_test.go:53
        Error:       Expected nil, but got: &url.Error{Op:"Get", URL:"https://github.com/cloudposse/atmos/info/refs?service=git-upload-pack", Err:(*net.OpError)(0x96175228050)}
        Test:        TestDescribeAffectedWithTargetRefClone
```

The test calls `tests.RequireGitHubAccess(t)` before the real work, which
does an HTTP `HEAD https://github.com` reachability probe and skips the test
if it fails — but CI sets `ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true` for
every acceptance-test job (confirmed in this same job's own env dump), making
every precondition helper in `tests/preconditions.go` a no-op there. The test
proceeds straight into `e.ExecuteDescribeAffectedWithTargetRefClone`, which
clones `github.com/cloudposse/atmos` for real via `go-git`'s own HTTP
transport, with zero retry. The 0.18s failure duration (this test normally
takes ~30-36s to actually clone) confirms the connection failed immediately,
not a hang — a one-off DNS/TLS blip reaching `github.com` from that Windows
runner, not a systemic or config issue (`github.com:443` is already
allowlisted in this job's `harden-runner` `allowed-endpoints`).

## Changes

- `pkg/describe/describe_affected_test.go`: `TestDescribeAffectedWithTargetRefClone`
  now retries `ExecuteDescribeAffectedWithTargetRefClone` within a new
  `describeAffectedCloneRetryBudget` (30s) instead of failing on the first
  error. A real failure (bad ref, auth) fails identically on every attempt
  and still fails the test once the budget is spent — this only absorbs a
  one-off network hiccup. Implemented as a local loop (not the `tests`
  package's `pollUntil`, which lives in a `_test.go` file there and so isn't
  importable from another package's tests).

## Verification

- `go build ./pkg/describe/... && go vet ./pkg/describe/...` — passed.
- `ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true go test ./pkg/describe/... -run '^TestDescribeAffectedWithTargetRefClone$' -v -timeout 120s`
  — passed locally (31.23s, real network access to `github.com`), confirming
  the retry wrapper doesn't change the happy-path behavior or timing
  materially.
- `atmos fix lint` (patch-scoped, this repo's real PR gate) — passed, 0 issues.

## Follow-ups

None. If this recurs frequently enough to suggest a systemic (not one-off)
GitHub connectivity problem from Windows runners, revisit.
