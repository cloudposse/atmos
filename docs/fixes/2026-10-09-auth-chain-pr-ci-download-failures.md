# Fix: Allow Go setup downloads in authentication-chain PR checks

**Date:** 2026-10-09

## Summary

Unblock Go installation in the `govulncheck` and Kubernetes end-to-end jobs for
[PR #3351](https://github.com/cloudposse/atmos/pull/3351), retaining blocked-by-default
network access and the existing security checks.

## Context

The attached first-attempt logs identified two deterministic network policy gaps:
`govulncheck` blocked `raw.githubusercontent.com` and `dl.google.com`, while the
Kubernetes job blocked `raw.githubusercontent.com` and `golang.org`. These are used
by `actions/setup-go` for its version manifest and official fallback download
chain. Both jobs failed before tests or vulnerability analysis ran.

Other reported failures were infrastructure-related: GitHub installation-token
rate limits prevented semver-label and changed-file lookups, and Atmos bootstrap
waited for the release API quota to reset. The race shard spent about 21 minutes
waiting for bootstrap, then exhausted its 30-minute job budget without reporting
a test assertion or data race. Pre-commit exhausted its 15-minute budget during
bootstrap. Dependency review reported unavailable repository support, but a
subsequent read-only dependency comparison succeeded and returned no changes.

## Changes

- Allow the Go manifest host and missing fallback download hosts in the two
  affected jobs, including module checksum verification for Kubernetes tests.
- Upload `govulncheck.sarif` only when the file exists. A failed setup step still
  fails the job, without an additional misleading missing-report error.
- Retry the failed dependency-review, Atmos autofix, and pre-commit runs to
  recover from the transient service failures. Preserve the existing semver
  label, license policy, vulnerability checks, and test timeouts.

## Validation

- `actionlint .github/workflows/codeql.yml .github/workflows/test.yml` passed.
- Compared allowlist changes with the exact denied domains in the attached logs.
- The dependency comparison API returned an empty successful result; the PR has
  exactly one semver label, `patch`.
- The pre-commit retry advanced past the previously stalled Atmos installation.
- Hosted CI must exercise the updated runner policy after the commit is pushed;
  local validation does not emulate the runner's network enforcement.

## Follow-ups

None.
