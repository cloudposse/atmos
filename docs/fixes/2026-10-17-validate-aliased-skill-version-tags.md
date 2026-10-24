# Fix: Validate aliased skill version tags

**Date:** 2026-10-17

## Summary

Follow YAML aliases when rejecting `!version` tags inside configuration sequences.

## Context

PR #3352 review found that sequence validation visited mapping and sequence
children but skipped alias targets. An anchor under `commands`, which is decoded
separately, could supply a tagged mapping to `devcontainer.examples` through an
alias. Direct and nested alias cases loaded successfully; an append list failed
later with an error naming an empty tag.

## Changes

Traverse alias targets and track visited node pointers for each validation walk.
Shared anchors are checked once, cycles terminate, and errors retain the alias's
configuration path. Validation continues through siblings after a visited node.

Add loading regressions for direct, nested, and append-list aliases, plus parsed
YAML tests for scalar and container targets, nested aliases, shared literals,
cycles, and invalid tags following a cycle.

## Validation

- Loading regressions reproduced accepted invalid aliases and the misleading
  append diagnostic before the fix; all pass afterward.
- Focused alias validation tests pass, including shared anchors and cycles.
- Configuration, schema, skill-source, and skill CLI package tests pass. The
  scoped `atmos test` short suite also passes.
- Configuration-package lint against the pre-fix HEAD reports zero issues.
- `go build -p 4 ./...` passes after the final validator refactor.
- Repository changed-code lint also reports eight unrelated findings in unchanged
  parser, template, Helm, Kubernetes, and Terraform output code. None of those
  files differ between this PR and its merge base with `origin/main`.

## Follow-ups

None.
