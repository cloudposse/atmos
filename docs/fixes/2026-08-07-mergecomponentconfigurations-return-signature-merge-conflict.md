# Fix: `mergeComponentConfigurations` return-signature mismatch after merging origin/main

**Date:** 2026-08-07

## Summary

- All 5 CI jobs on PR #2892 (`build`, `Build (macos)`, both Kubernetes e2e jobs, and
  `website-deploy-preview`, which also runs `go build`) failed with the same compile error:
  `internal/exec/stack_processor_merge.go:426:15: not enough return values — have (nil, error),
  want (map[string]any, ComponentDeferredContexts, error)`.
- Root cause: a silent semantic merge conflict, not a textual one. This branch had already changed
  `mergeComponentConfigurations` to return a third value, `ComponentDeferredContexts`. `origin/main`
  advanced with #2874 (Kubernetes single-file GitOps delivery and Kustomize `metadata.name`
  exemption), which added a new `finalComponentValidate` merge step to the *same function* using
  the old two-value `return nil, err` — plus 7 test call sites in
  `stack_processor_merge_test.go` still destructuring only `(comp, err)`. Git's auto-merge combined
  both diffs cleanly (no overlapping lines), but the result didn't compile.
- GitHub's PR-preview build always merges the PR head with the *current* tip of `origin/main`, so
  this broke as soon as #2874 landed on main — independent of anything committed on this branch
  today.

## Context

The user attached 5 failing-job log files and asked to fix the CI failures. Grepping each log for
`##[error]` showed all 5 pointed at the identical line and error, so this was one root cause, not
five separate ones. `git log --oneline HEAD..origin/main` showed this branch was 2 commits behind
main, including #2874. Merging `origin/main` locally reproduced the exact compile error, confirming
it wasn't present on this branch in isolation — it only appears in the merge.

## Changes

- `internal/exec/stack_processor_merge.go` — the `finalComponentValidate` error-check block (added
  by #2874) now returns `nil, nil, err` instead of `nil, err`, matching the enclosing function's
  three-value signature.
- `internal/exec/stack_processor_merge_test.go` — 7 call sites in the `validate-*` subtests (also
  added by #2874) now destructure `comp, _, err := mergeComponentConfigurations(...)` instead of
  `comp, err := ...`, matching every other call site in the file.
- Both changes committed on top of a merge commit (`git merge origin/main --no-edit`) bringing this
  branch up to date with main, since the CI failure only reproduces in that merged state.

## Validation

- `go build ./...` — clean (was failing before the fix).
- `go vet ./...` — clean (also caught the test-file mismatch `go build` alone didn't surface).
- `go test ./internal/exec/... -run TestMergeComponentConfigurations -v` — all subtests pass,
  including the previously-broken `validate-*` cases.
- `go test ./internal/exec/... ./pkg/component/kubernetes/... ./pkg/provisioner/... ./pkg/git/...`
  — all pass.
- `./custom-gcl run --new-from-rev=origin/main internal/exec/...` — 0 issues.
- `gofumpt -l` on both changed files — clean.

## Follow-ups

None.
