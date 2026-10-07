# Fix: Review every Starlark stack layer automatically

**Date:** 2026-10-06

## Summary

Enable CodeRabbit automatic reviews for each base branch in the Starlark PR stack.

## Context

CodeRabbit's automatic-review configuration allowed `main` at the bottom of the
stack and only some intermediate bases in later layers. Updates targeting the
remaining branches therefore needed manual review requests.

## Changes

List each actual stack base explicitly in the review configuration. Keep the
existing review rules, required change-request workflow, and draft policy.

Also merge the current main branch, retaining its scaffold navigation links and
the YAML-function guide anchor from this stack. This resolves the README
conflict that prevented pull-request checks from starting.

The resumed review also identified two YAML loader inconsistencies:

- Limit included-script provenance to typed script steps and typed script-hook
  payloads. Carry the parent and plain-data-section context through the tag walker
  so variables, settings, environment, metadata, and other data gain no internal
  provenance fields even when their contents resemble a script step. Child context
  is derived once during indexed traversal and survives `!append` node rewrites.
- Expand configured key delimiters before decoding an already parsed YAML node,
  matching the file entrypoint while retaining tag evaluation and source positions.

## Validation

- Compared the allowlist with the base branches reported by GitHub for all ten PRs.
- Checked the patch for whitespace errors.
- Automatic review results will be verified after the configuration reaches
  each layer.

- Both YAML regressions failed before the fixes: plain data gained undeclared
  provenance fields, and parsed-node decoding kept delimited keys literal.
- The focused include-provenance, script-source, parsed-node, and shared-tag-walker
  tests passed with `GOMAXPROCS=4 go test -p 2 ./pkg/utils` and the targeted test
  filter (7.627 seconds). Existing local, remote, queried, inline, and hook/list
  cases retain their behavior. Fixtures now explicitly declare their hook type.
- Final focused tests, including appended data and appended script steps, passed
  with coverage enabled (5.237 seconds).
- Formatted the affected Go files with `gofumpt`; `git diff --check` passed.

## Follow-ups

None.
