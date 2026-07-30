# Fix: `atmos git clone`'s no-arg CI checkout passed a raw ref instead of a branch name

**Date:** 2026-07-30

## Summary

Writing a real end-to-end CI job to exercise the no-arg `atmos git clone` CI-checkout path (the
`actions/checkout` replacement documented in `docs/prd/git-ops.md`) surfaced a genuine bug:
`runCICheckout` (`cmd/git/clone.go`) passed `ciCtx.Ref` — the raw ref a CI provider reports (e.g.
`refs/heads/main`, or `refs/pull/123/merge` for a PR) — as the `git clone --branch` argument.
`git clone --branch` requires a short branch/tag name, not a full ref path, so every real
push/PR-triggered no-arg checkout would fail with `fatal: Remote branch refs/heads/main not found
in upstream origin`. Fixed by using `ciCtx.Branch` (the already-parsed short name the same provider
context struct exposes) instead.

## Context

This surfaced while adding the `ci-bootstrap-clone` job to `.github/workflows/native-ci.yml` (PR
#2812) to give the CI git-clone bootstrap feature (`cmd/git/bootstrap.go`) real, non-stubbed test
coverage for the first time. The new job clones a throwaway local bare fixture repository under
realistic GitHub Actions env vars (`GITHUB_REF=refs/heads/main`, matching real production values)
and failed immediately with the ref-not-found error above.

The bug predates PR #2812 entirely — it was introduced in `10f19c24ef` ("feat(ci): fork-PR safety
gate for `atmos git clone` (#2661)"), merged 2026-07-05, well before this branch existed. It was
never a failing test because the existing unit test asserting this behavior
(`TestRunCICheckout_BranchFromRef`, since renamed) fed `ciCtx.Ref` an unrealistic bare branch name
(`"feature/x"`) instead of a real full ref — so it "passed" while validating the wrong invariant.
A stub-based test can't catch this class of bug: it never shells out to a real `git` binary, so a
malformed `--branch` argument never actually gets rejected.

## Changes

- `cmd/git/clone.go`: `runCICheckout` now resolves branch precedence from `ciCtx.Branch` (parsed
  short name) instead of `ciCtx.Ref` (raw ref path).
- `cmd/git/provider_stub_test.go`: `TestRunCICheckout_BranchFromRef` renamed to
  `TestRunCICheckout_BranchFromCIContext` and given both a realistic full `Ref` and the `Branch`
  short name, asserting the checkout branch comes from `Branch` even when `Ref` is also populated.
  `TestRunCICheckout_WithStub`'s fixture also gained a `Branch` field to match real provider output.
- `.github/workflows/native-ci.yml`: new `ci-bootstrap-clone` job exercises the real, non-stubbed
  clone path end-to-end against a local fixture repo — the regression test for this bug going
  forward — plus a negative-path job step confirming explicit CI-bootstrap opt-out
  (`ATMOS_CI=false`) still fails without a repository/config to act on.
- `docs/prd/git-ops.md`: documented the config-init-error tolerance mechanism
  (`cmd/git.CICloneBootstrapRequested`) this same PR added, under "Native CI Behavior" — it was
  implemented with no PRD coverage at all.

## Validation

- Local reproduction: built `atmos` from the pre-fix source, ran the new job's exact fixture +
  clone script locally — reproduced `fatal: Remote branch refs/heads/main not found in upstream
  origin`.
- Rebuilt after the fix, re-ran the identical script — clone succeeded, checked-out marker file
  content verified.
- `go test ./cmd/git/...` — all pass, including the renamed/corrected test.
- `go build ./...` — clean.
- Ran the new negative-path script locally (`ATMOS_CI=false` in an empty, unconfigured directory)
  — correctly fails with `ErrGitRepositoryRequired`.

## Follow-ups

None — the new `ci-bootstrap-clone` native-ci job is permanent regression coverage for this exact
scenario going forward.
