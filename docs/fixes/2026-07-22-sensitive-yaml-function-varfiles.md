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
separate raw-argument symptom. The controlled fixture did not recreate that exact path:
both versions preserved the explicit tag and resolved the state value correctly.

### Hypothesis: how raw function arguments could become a Terraform value

An explicit YAML scalar has two distinct parts after YAML parsing: its tag
(`!terraform.state`) and its scalar value (`producer ".fields.PRIVATE_KEY"`). The
runtime resolver only runs when it receives the reconstructed complete expression:
`!terraform.state producer ".fields.PRIVATE_KEY"`.

The observed raw-argument value is therefore consistent with a tag-preservation failure
at a decode or validation boundary: if that boundary clears or drops the explicit tag
without rebuilding `tag + " " + value`, the runtime sees only
`producer ".fields.PRIVATE_KEY"`. It is then an ordinary string, not a recognized YAML
function, and can be serialized unchanged as a Terraform input. Graceful degradation
does not explain that symptom; it substitutes `(computed)`, not the function arguments.

The exact decode or validation boundary that dropped the tag in the original incident
was not isolated from the available configuration. Controlled `1.221.0` and `1.223.0`
runs both preserved the explicit tag and resolved the real state value. The regression
tests added by this fix assert preservation in mapping and list values and assert that
`TF_VAR_*` receives only the resolved value, so the reported failure mode is now covered
even though its original environmental trigger remains unconfirmed.

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
