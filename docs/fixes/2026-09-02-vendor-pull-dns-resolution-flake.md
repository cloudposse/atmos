# Fix: `atmos_vendor_pull` failed on a runner-level DNS outage, not a code bug

**Date:** 2026-09-02

## Summary

`Acceptance Tests (macos, shard 4/10)` failed with:

```text
--- FAIL: TestCLICommands (159.39s)
    --- FAIL: TestCLICommands/atmos_vendor_pull (122.77s)
        cli_test.go:1591: Reason: tty did not match pattern "Vendored 3 components".
```

The captured tty output shows the real cause was never in Atmos:

```text
Failed to vendor github/stargazers: error : github/stargazers: failed to
download package: failed to download file: error downloading
'***github.com/cloudposse/atmos.git?depth=1&ref=main': git command exited
with non-zero status: /opt/homebrew/bin/git exited with 128: Cloning into
'/private/var/folders/.../temp'...
fatal: unable to access 'https://github.com/cloudposse/atmos.git/': Could
not resolve host: github.com
```

## Context

`tests/test-cases/vendor-test.yaml`'s `atmos_vendor_pull` case runs a real,
uncached `atmos vendor pull` against `tests/fixtures/scenarios/vendor`,
which genuinely clones `github/stargazers` from
`github.com/cloudposse/atmos.git` over the network -- by design for this
fixture (it also exercises an OCI-sourced component and a plain HTTPS
component in the same run), not a gap introduced by this branch.

On this run, `git` itself failed to resolve `github.com` at the OS level
(`Could not resolve host: github.com`), before any TLS handshake or HTTP
request could even begin. The GitHub-hosted macOS runner's own
StepSecurity Harden Runner network log (embedded later in the same job
log) independently corroborates a DNS-layer problem at the same timestamp,
consistent with a transient resolver outage on the runner rather than
anything under this repository's control.

Checked and ruled out as codebase causes:
- No proxy, DNS, or network-mirror configuration exists anywhere in
  `.github/workflows/` or the `vendor` fixture that this run could have
  regressed.
- The failure is a plain OS-level `getaddrinfo`/resolver failure
  (`Could not resolve host`), not an HTTP error, TLS error, or rate limit
  from GitHub itself -- ruling out an auth/token regression from the
  `github_token` precondition this test declares.
- Only this one subtest failed in the entire shard; no other test in the
  same run hit a network-dependent path and failed, consistent with an
  isolated, transient DNS blip rather than a systemic issue.

## Changes

None. There is no code, test, or configuration change in this repository
that fixes a transient DNS resolution failure on a GitHub-hosted CI
runner. Making a speculative change here (e.g., adding retries around the
git clone inside `go-getter`, which this repo doesn't own) would risk
masking a real vendoring regression if one ever occurs, which is exactly
the property this test exists to catch.

## Validation

- Confirmed via the fixture's `vendor.yaml` and a repo-wide grep that no
  network-mirror/proxy config applies to this test, ruling out a
  configuration regression.
- Confirmed the failure text is an OS-level resolver error
  (`Could not resolve host: github.com`), not an application-level error
  path in `internal/exec`/`pkg/vendor`.
- Confirmed no other subtest in the same shard's run failed from the same
  or a related cause.

## Follow-ups

None. Re-running the failed CI job is expected to pass; no issue is being
opened since there is no actionable code change to track.
