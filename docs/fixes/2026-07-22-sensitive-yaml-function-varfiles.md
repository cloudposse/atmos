# Fix: Sensitive YAML-function values no longer leak into Terraform varfiles

**Date:** 2026-07-22

## Summary

Terraform inputs declared with `sensitive = true` are now excluded from generated
`*.terraform.tfvars.json` files while masking is enabled. Resolved values are supplied
to Terraform only through `TF_VAR_<name>` environment variables. This fix was opened
after an explicit YAML function's raw arguments appeared in a generated varfile instead
of its resolved value; the tests now protect that tag-preservation boundary as well.

## Context

The previous secret partition relied only on values that had been registered with the
masker. A value resolved by `!terraform.state` from a source that was not registered as
a secret therefore bypassed that partition, even when the consumer Terraform variable
was declared sensitive.

A local, real-state reproduction confirmed the sensitive JSON leak in both Atmos
`1.221.0` and `1.223.0`. The original report that opened this fix also showed the
separate raw-argument symptom. The direct source path that produces that symptom is now
identified and covered by regression tests.

### Root cause: template preprocessing discarded explicit tags

An explicit YAML scalar has two distinct parts after YAML parsing: its tag
(`!terraform.state`) and its scalar value (`producer ".fields.PRIVATE_KEY"`). The
runtime resolver only runs when it receives the reconstructed complete expression:
`!terraform.state producer ".fields.PRIVATE_KEY"`.

The execution-critical structured template pre-processing path decoded raw YAML into an
untyped Go map with plain `yaml.Unmarshal` and then re-rendered it. Plain decoding
preserves the scalar value but drops its explicit tag, so it transformed `!terraform.state producer
".fields.PRIVATE_KEY"` into `producer ".fields.PRIVATE_KEY"`. The final function
processor consequently received an ordinary string and correctly did not invoke the
state resolver; Terraform was handed the raw arguments. Graceful degradation was not
involved: it emits `(computed)`, never function arguments.

The pre-pass now uses Atmos's tag-aware YAML decoder, which reconstructs custom tags as
`tag + " " + value`. The regression test follows that route and verifies that the complete
expression reaches final configuration. Other raw YAML parsing in stack processing is
either node-only diagnostics or display-only inspection; normal stack configuration
decoding already uses the tag-aware decoder. Controlled `1.221.0` and `1.223.0` runs took
a simpler route and therefore resolved correctly. This was not introduced by YAML
function validation: commit `c1fd583de7` added the structured pre-pass on 2026-07-08,
after those releases. That commit is the change that made a raw explicit tag pass through
plain map decoding before the normal tag-aware stack decoder.

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
- Preserve explicit YAML function tags through the context/import template pre-processing
  path that previously stripped them.

## Validation

- `go test ./internal/exec ./pkg/utils -count=1`
- Repository pre-commit hooks: go-fumpt, Go build, `go mod tidy`, and golangci-lint.
- Manual local-state reproduction with `atmos --use-version=1.221.0` and
  `atmos --use-version=1.223.0`.

## Follow-ups

None.
