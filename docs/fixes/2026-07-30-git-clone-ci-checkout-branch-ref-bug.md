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

This surfaced while prototyping a `ci-bootstrap-clone` job for `.github/workflows/native-ci.yml`
(PR #2812), meant to give the CI git-clone bootstrap feature (`cmd/git/bootstrap.go`) real,
non-stubbed test coverage. That job redirected the clone target to a throwaway local fixture repo
by overriding `GITHUB_SERVER_URL`/`GITHUB_REPOSITORY` at the step level. Locally (a plain shell,
not the Actions runner) that override worked and reproduced the ref-not-found failure below. Run
for real in GitHub Actions, it didn't: GitHub Actions silently ignores attempts to override
`GITHUB_`-prefixed environment variables via a step's `env:` key, so the job actually cloned the
real `cloudposse/atmos` repo (using the ambient real values) instead of the intended fixture, then
failed a later assertion that expected the fixture's marker file. That's a platform restriction,
not a fixable scripting bug — the redirection approach cannot work in real Actions runs. The job
was reverted rather than reworked; see Follow-ups.

The underlying bug this was chasing is real and unrelated to that job's failure: `runCICheckout`
(`cmd/git/clone.go`) passed `ciCtx.Ref` — the raw ref a CI provider reports (e.g.
`refs/heads/main`, or `refs/pull/123/merge` for a PR) — as the `git clone --branch` argument.
`git clone --branch` requires a short branch/tag name, not a full ref path, so every real
push/PR-triggered no-arg checkout would fail with `fatal: Remote branch refs/heads/main not found
in upstream origin`. Fixed by using `ciCtx.Branch` (the already-parsed short name the same provider
context struct exposes) instead. This was found and confirmed via a local shell reproduction (built
`atmos` from source, ran a hand-built fixture + clone script directly, outside any CI runner), not
via the reverted job.

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
- `docs/prd/git-ops.md`: documented the config-init-error tolerance mechanism
  (`cmd/git.CICloneBootstrapRequested`) this same PR added, under "Native CI Behavior" — it was
  implemented with no PRD coverage at all.
- `.github/workflows/native-ci.yml`: the `ci-bootstrap-clone` job was added, found broken against
  the real Actions runner (see above), and reverted in full rather than kept as a job that looks
  like coverage but silently exercises the wrong repository.

## Validation

- Local reproduction: built `atmos` from the pre-fix source, ran a hand-built fixture + clone
  script locally (plain shell, not a CI runner) — reproduced `fatal: Remote branch refs/heads/main
  not found in upstream origin`.
- Rebuilt after the fix, re-ran the identical script — clone succeeded, checked-out marker file
  content verified.
- `go test ./cmd/git/...` — all pass, including the renamed/corrected test.
- `go build ./...` — clean.
- Ran the negative-path script locally (`ATMOS_CI=false` in an empty, unconfigured directory) —
  correctly fails with `ErrGitRepositoryRequired`.
- The `ci-bootstrap-clone` job itself was run for real in GitHub Actions once (see above) — that
  run is what revealed the `GITHUB_*` env-override restriction, and is not evidence for or against
  the branch/ref fix, since it never reached that job's fixture-repo assertions on the intended
  target.

## Follow-ups

There is still no real, non-stubbed CI coverage of the no-arg CI-checkout path
(`runCloneNoArg`/`runCICheckout`) — only the corrected unit test above. A genuine end-to-end test
would need to run without checking out `cloudposse/atmos` first (since the whole point is
exercising the pre-checkout bootstrap case) while still testing this PR's own built binary, and
would need a clone target `ci.Detect()` can be pointed at without relying on overriding
`GITHUB_*` variables, which Actions does not allow. No such approach was identified this session;
left as an open gap rather than papered over with another synthetic redirection.
