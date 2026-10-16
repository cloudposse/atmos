# Fix: CloudFormation prompt persistence and backend warning configuration

**Date:** 2026-10-16

## Summary

Persist prompted flag selections in Viper and pass CloudFormation artifact-bucket warning overrides through the backend configuration.

## Context

CodeRabbit identified three issues in PR #3137: prompted values did not reach callers using Viper, backend warning overrides were placed above the map consumed by deletion, and example-authoring guidance described a legacy related-docs lookup as the current mechanism.

## Changes

- Bind successfully updated prompted flags using their configured Viper keys; leave failed updates unbound.
- Place CloudFormation state-file warning overrides inside the nested backend map.
- Document README `related_docs` frontmatter for new examples and explain migration from the removed legacy `DOCS_MAP`.
- Exercise real prompt selection and the deletion path through `DeleteBackendWithParams`, including Terraform warning preservation.
- Strip ANSI sequences before checking warning content so terminal styling cannot split expected phrases.
- Correct the diff help's offline-render claim and regenerate its help screengrab.

## Validation

The new prompt and CloudFormation deletion regressions failed against the original implementation, reproducing both bugs. Go 1.26.9 race tests passed for `pkg/flags`, `pkg/component/aws/cloudformation`, `pkg/provisioner/backend`, and `pkg/provisioner`. The invalid-value negative regression also passed under the race detector. `go build ./...`, applicable pre-commit hooks, fix-record validation, and all 23 website prebuild tests passed. The website production build passed with existing normalization and broken-anchor warnings. Broader lint against `origin/main` found one existing, untouched `revive` complexity finding in `resolveSpecAndTemplate` (`executor.go:189`); the changed-file pre-commit lint passed.

The backend deletion assertion was additionally reproduced with forced ANSI color (`TERM=xterm-256color`, empty `NO_COLOR`, `ATMOS_FORCE_COLOR=1`) under Go 1.26.9 and the race detector. Stripping ANSI before semantic assertions makes both forced-color and ordinary terminal runs pass. The diff help now explains that rendering may provision sources or resolve authenticated remote values without creating a changeset. The command package tests and executable build passed. The diff help cast was authentically regenerated and passed targeted Starlark checks for corrected guidance, styling, and absence of errors or local paths; the full cast-library validator still encounters the pre-existing unrelated cast without ANSI color.

## Follow-ups

Publish the validated fix on PR #3137 and verify CI and CodeRabbit review on the final commit.
