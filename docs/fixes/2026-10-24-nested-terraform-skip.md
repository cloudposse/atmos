# Fix: Propagate skipped functions into nested Terraform lookups

**Date:** 2026-10-24

## Summary

Nested `!terraform.output` and `!terraform.state` evaluations now retain the caller’s skipped YAML functions.

## Context

On main at `cb23b3f701`, a dependency component with an uninitialized `!secret` failed despite the caller using `--skip=secret`. Cached results could also cross evaluation policies.

## Changes

Carry skip and masking options through component descriptions and Terraform lookups. Partition reference caches by skip policy and bypass the invocation-local state cache for skipped evaluations. Put shared lookup options and offline integration coverage in `pkg/terraform`; retain compatibility adapters in `internal/exec`. Document nested skip behavior.

## Merge-queue interaction with explicit YAML function tags

Merge queue failed twice because `main` gained explicit function tags (#3463) after this branch was cut. A skipped function now travels as transported source, and `EvaluateYqExpression` serialized it as a custom tag. Decoding the yq result to Go values drops custom tags, so a skipped `!secret CREDENTIAL` came back as `CREDENTIAL` through static remote-state lookups. `EvaluateYqExpression` now selects over the ordinary expression form, so the tag name survives. A yq-level regression covers scalars, list elements, and maps.

## Validation

New offline regressions failed before the fix and passed afterward, including eager/deferred references and skip → normal → skip cache isolation. Ran `go test ./pkg/terraform ./pkg/terraform/output ./pkg/terraform/refcache -count=1`, focused existing `internal/exec` cache/secret tests, changed-code lint, and `go build ./...`. The website build passed on the combined implementation.

## Follow-ups

Pre-existing related paths: `atmos.Component` and CloudFormation component descriptions still omit caller skip options. Per the task’s scope boundary, these are reported in [PR #3513](https://github.com/cloudposse/atmos/pull/3513), without expanding this Terraform fix or opening new issues. `!store` reads its store directly and does not describe a dependency component.
