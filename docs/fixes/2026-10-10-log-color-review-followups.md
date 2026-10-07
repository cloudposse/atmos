# Fix: Correct vendor receipts and cover log-color edge cases

**Date:** 2026-10-10

## Summary

Address review feedback on PR #3345 by making the local vendor receipts
reproducible, testing uncovered color-policy paths, and documenting production
helpers whose behavior was not explained.

## Context

CodeRabbit identified different source digests for `comp-a` and `comp-b`, even
though both use the same local source. Earlier test runs had left ignored
Terraform artifacts in the source directory, so regenerating directly from the
working directory would preserve machine-specific state in the receipts.

The attached Codecov report for `9c8071e515` showed 92.33% patch coverage and
24 uncovered/partial changed lines. The repository target is 85%; the missing
paths were still useful regression-test candidates. Coverage policy is unchanged.

## Changes

- Regenerate both local-source digests using independently staged copies of
  tracked source files, checking their identity and recorded inventories with
  the production downloader and lockfile code.
- Test root startup flag precedence, global color vetoes, forced color after a
  log destination changes, invalid color configuration, fallback stderr output,
  positional boolean tokens, and environment-value normalization.
- Exercise both logo output functions on a pseudo-terminal, including opt-outs
  and forced color, instead of covering only the specified-output function.
- Add behavioral comments to eight production helpers. Keep the user-requested
  vendor lockfiles and CI allowlist changes; explain that scope in the PR body.

## Validation

- Passed focused tests with coverage:
  `go test ./cmd ./pkg/config ./pkg/logger ./pkg/terminal/env ./internal/tui/utils -run 'Color|Bool|Logger|StyledText' -coverprofile=.context/review-3345/after.out -count=1`.
- Local coverage is 100% for the logger color-policy functions, log-color
  validation/resolution functions, and terminal environment package. This
  focused profile is not a replacement for Codecov's full CI patch report.
- Passed the source identity/inventory validator, focused downloader and
  lockfile tests, the application build, and `git diff --check`.
- Repository-wide changed-code lint reported an existing import-order issue in
  untouched `internal/exec/template_funcs_store.go`.
- Lint scoped to this update (`--new-from-rev=HEAD`) passed with zero issues;
  the fix-log format validator also passed.

## Follow-ups

None.
