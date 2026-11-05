# Fix: Enforce CloudFormation metadata and detect shared configuration changes

**Date:** 2026-10-10

## Summary

CloudFormation affected detection now checks shared configuration sections, execution honors abstract and locked metadata, and stack validation accepts component locals.

## Context

Changes to `vars`, `env`, `source`, or `provision` alone were omitted from CloudFormation affected results. Single-component execution could provision abstract or locked components. The stack-config schema rejected `locals` even though the manifest schema supported it.

## Changes

- Compare shared sections for additions, removals, and value changes using their existing affected reasons.
- Reject abstract components for every operation except local render, and reject apply/delete on locked components before validation, authentication, source provisioning, hooks, and dry-run dispatch. Locked components retain preview and read operations.
- Allow CloudFormation `locals` through the existing object schema and verify malformed locals remain invalid.
- Simplify the output-test substring helper with `strings.Index`, preserving the existing start-index boundary behavior.

## Validation

Before the fixes, new regressions reproduced 12 missed affected changes, 14 forbidden execution cases, and the stack-config rejection of locals.

With Go 1.26.9:

- `go test -race ./pkg/component/aws/cloudformation -count=1` passed; full package coverage was 97.5%, and the metadata guard and single-component executor both reached 100% function coverage.
- `go test -race ./internal/exec ./pkg/datafetcher ./pkg/output -run 'CloudFormation|Format' -count=1` passed, including shared-section changes and unchanged/absent cases, schema acceptance/rejection, and output formatting.
- `go build ./...` passed.
- The custom golangci-lint binary reported zero issues for changed lines in the four affected packages.
- Local coverage profiles covered all 15 executable changed production lines. This local measurement does not replace the final Codecov patch report.
- `gofumpt`, `git diff --check`, and the fix-record validator passed.

## Follow-ups

None.
