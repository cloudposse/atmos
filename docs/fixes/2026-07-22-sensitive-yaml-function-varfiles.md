# Fix: Sensitive YAML-function values no longer leak into Terraform varfiles

**Date:** 2026-07-22

## Summary

Terraform inputs declared with `sensitive = true` are now excluded from generated
`*.terraform.tfvars.json` files while masking is enabled. Resolved values are supplied
to Terraform only through `TF_VAR_<name>` environment variables.

## Context

The previous secret partition relied only on values that had been registered with the
masker. A value resolved by `!terraform.state` from a source that was not registered as
a secret therefore bypassed that partition, even when the consumer Terraform variable
was declared sensitive.

A local, real-state reproduction confirmed the JSON leak in both Atmos `1.221.0` and
`1.223.0`. The same fixture resolved the explicit `!terraform.state` tag correctly in
both releases; the separately reported literal-argument symptom was not reproduced.

## Changes

- Inspect the component's Terraform module metadata and treat supplied `sensitive = true`
  variables as secret-bearing inputs when masking is enabled.
- Exclude those inputs from normal and `--with-secrets` JSON varfile output, while retaining
  the `--mask=false` typed-JSON compatibility behavior.
- Pass secret-bearing inputs through `TF_VAR_*`, and reject degraded `(computed)` values
  before Terraform varfile, execution, or shell handoff.
- Add function-aware debug logging for lenient YAML-function degradation.
- Add a neutral producer/consumer regression fixture and coverage for explicit-tag decoding,
  state resolution, JSON exclusion, and `TF_VAR_*` transport.

## Validation

- `go test ./internal/exec ./pkg/utils -count=1`
- Repository pre-commit hooks: go-fumpt, Go build, `go mod tidy`, and golangci-lint.
- Manual local-state reproduction with `atmos --use-version=1.221.0` and
  `atmos --use-version=1.223.0`.

## Follow-ups

None.
