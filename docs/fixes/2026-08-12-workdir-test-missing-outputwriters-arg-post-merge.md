# Fix: workdir test call missing new Provision OutputWriters argument

**Date:** 2026-08-12

## Summary

`TestServiceProvision_NestedComponentName_SanitizesLikeBuildPath`
(`pkg/provisioner/workdir/workdir_test.go`) failed to compile with `not
enough arguments in call to service.Provision` after merging
`origin/main`'s `fix(terraform): prevent concurrent output corruption
(#2898)`, which added a fourth `provisioner.OutputWriters` parameter to
`(*Service).Provision`. Acceptance Tests CI (linux/windows/macOS) reported
this build failure across two consecutive pushes, even though the exact
pushed commit's content, verified byte-for-byte against GitHub's own blob
SHAs and a fully isolated fresh clone, compiled cleanly in every case.

## Context

GitHub Actions' `pull_request`-triggered workflows check out the *implicit
merge* of the PR branch with the base branch's current tip
(`refs/pull/<N>/merge`), not the PR branch's raw HEAD commit. Each
Acceptance Tests run therefore silently re-merges this branch against
whatever `main` looked like at trigger time. `main` had already merged
`#2898`'s `Provision` signature change into every call site *it* knew
about, but this branch's own
`TestServiceProvision_NestedComponentName_SanitizesLikeBuildPath` (added by
an earlier, unrelated fix on this branch, docs/fixes/2026-08-07-*) called
`Provision` with the old three-argument form and didn't exist in `main`'s
history — so `git`'s line-based auto-merge had nothing on `main`'s side to
reconcile it against and left the stale call in place. Because the CI
checkout is a synthetic merge computed fresh per run, this surfaced on
every CI run once `main` carried `#2898`, even before this branch's own
`git merge origin/main` made the same conflict locally reproducible.

## Changes

- `pkg/provisioner/workdir/workdir_test.go`: added the missing
  `provisioner.OutputWriters{}` argument to the one stale `Provision` call,
  matching every other call site in the package.

## Validation

- `go vet ./pkg/provisioner/workdir/...` — clean (was
  `workdir_test.go:617:77: not enough arguments in call to
  service.Provision`, confirmed failing pre-fix against the post-merge
  tree).
- `go build ./...` — clean.
- `go test ./pkg/provisioner/...` — all pass.
- Broad regression sweep (`internal/exec`, `pkg/component`, `pkg/hooks`,
  `pkg/provisioner`, `pkg/runner`, `pkg/scanners/tflint`, `pkg/scheduler`,
  `pkg/terraform/output`, `pkg/ui`, `tests`) — all pass; one transient
  failure in `github.com/cloudposse/atmos/tests` (a subprocess-read panic
  under heavy concurrent package load) did not reproduce when that package
  was re-run in isolation (`ok`, 693s).
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

## Follow-ups

None. Falling behind `origin/main` on a long-lived feature branch will keep
producing this class of CI-only failure (real locally only after the next
`git merge origin/main`) for as long as the gap persists -- merging
frequently is the only real mitigation, not a code change.
