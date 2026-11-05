# Fix: Terraform README formatting and regression coverage

**Date:** 2026-10-17

## Summary

Honor Terraform documentation formatting settings and absolute source paths, and
correct the recovered README template's empty sections, module links, and multiline types.

## Context

A field test of PR #3404 rendered Cloud Posse's Kinesis Stream, S3 Bucket, and Null
Label modules. It reproduced ignored formatter settings, silent omission for
absolute paths, and template defects that a fixture containing every section did
not expose. The native Terraform objects and sorting already worked.

## Changes

- Pass configured visibility, empty-section, and heading-level options to the
  formatter. Optional visibility booleans distinguish omission from explicit false.
  Preserve the existing built-in readme defaults and terraform-docs defaults for
  omitted settings in other generators.
- Gate corrected formatter behavior at edition `2026-10-17`; older pins retain
  historical formatted output. Native objects and sorting work in both paths.
- Resolve absolute base/source paths in the docs package; simplify the exec adapter.
- Guard actual displayed input/resource groups, prefix every multiline type line
  inside its blockquote, and avoid inventing registry links for local/Git sources.
- Preserve the recovered source template. The independent reference comparison
  applies only the documented multiline blockquote correction.
- Add reduced module fixtures, Markdown-to-HTML checks, formatting and edition tests,
  command-level absolute-path coverage, and a CLI formatting fixture.
- Explain that Atmos 1.232.0 cannot parse the new function, even inside a guard;
  adopting repositories must upgrade before switching templates.
- Rename the optional-only fixture's reserved `count` input to `item_count`.
- Exercise datasource function isolation, metadata preservation, temporary-file
  cleanup, and errors from invalid metadata, missing temporary directories,
  failed downloads, and unwritable README destinations.

## Validation

- New regression tests failed before the fixes and passed afterward.
- `go test ./pkg/docs ./pkg/template/datasource ./pkg/edition ./pkg/config -count=1` passed.
- Focused docs command and exec compatibility tests passed, including absolute base paths.
- All seven documentation-generation CLI cases passed (five together, then the two new empty-group cases).
- `go build ./...`, a fresh Atmos binary build, and the custom changed-file linter passed.
- Rerendered pinned checkouts of Kinesis Stream, S3 Bucket, and Null Label. Verified
  corrected groups, links, HTML type blocks, visibility, heading levels, and absolute paths.
- Confirmed the earlier-edition CLI output matches released Atmos 1.232.0 byte for byte.
- Markdown link checking passed with no errors; the original-design golden was regenerated
  through the production renderer and reviewed (only three blockquote prefixes changed).
- `npm run build` completed successfully, including 22 website prebuild tests. It emitted
  existing MDX-normalization fallback and broken-anchor warnings on unrelated pages.
- Terraform validation reproduced the reserved-name error and passed after the rename.
- `go test ./pkg/docs ./pkg/template/datasource -coverprofile=.context/readme-review-coverage.out -count=1`
  passed with 95.5% and 82.6% statement coverage respectively. These focused local
  measurements do not replace Codecov's CI patch calculation (85% target).
- Focused package tests also passed with `-race`; `go build ./...` and the
  changed-file linter passed. Offline link validation confirmed the helper fixture
  target exists, so the earlier missing-link review finding is already addressed.

## Follow-ups

The published release version introducing the function must be assigned by the
release process. Documentation deliberately does not promise an unverified 1.233.0.
Publishing shared templates and regenerating other repositories remain outside this PR.
