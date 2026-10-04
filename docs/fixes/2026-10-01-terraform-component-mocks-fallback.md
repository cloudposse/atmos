# Fix: `--use-mocks` treats component mocks as fallbacks instead of replacing real state

**Date:** 2026-10-01

## Summary

With `--use-mocks`, `!terraform.state` and `!terraform.output` now use the real value when it exists
and fall back to the referenced component's `mocks` only when its state is not provisioned or the
requested output is missing. The previous mocks-only behavior remains available as
`--use-mocks=always` or `components.terraform.mocks.mode: always`, and is the edition default for
projects pinned before 2026-10-01.

## Context

Component mocks (v1.224.0) were intended as fallbacks, but `--use-mocks` short-circuited every lookup
to the `mocks` map before any real lookup ran, so partly deployed environments got mock values even
for components with real state. The original PRD described this as deliberate fail-closed behavior;
the intended design is real state first, mocks for the gaps.

## Changes

- New atmos.yaml setting `components.terraform.mocks.mode` (`fallback` | `always`), env var
  `ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE`, and sentinel `ErrInvalidMocksMode`.
- Edition journal `KindValue` entry (2026-10-01, `always` → `fallback`) following the
  `components.terraform.init.mode` precedent, so pinned projects keep the old behavior.
- `--use-mocks` is now value-bearing (`true`/`false`/`fallback`/`always`; bare means the configured
  mode) on `terraform plan` and `describe component`; mutating commands still reject it.
- Fallback resolution runs only on recoverable misses (not provisioned, output not found, missing or
  null direct output); credential, network, and backend errors are returned unchanged. Precedence:
  real value, mock, YQ `//` default, original error.
- An invalid `mocks.mode` from atmos.yaml fails loudly under `--use-mocks` instead of silently
  disabling mocks.
- Updated PRD, mocks/flag/function/configuration/environment-variable docs, Terragrunt migration
  page, example README, agent skills, help-text snapshots, defaults snapshot, and atmos.yaml schema;
  added a changelog post and roadmap milestone.

## Validation

- `go build ./...` and `go vet` on the changed packages: clean.
- `go test` passed for `./pkg/edition/...`, `./pkg/config/...`, `./pkg/schema/...`, `./pkg/flags/...`,
  `./cmd/terraform/...`, `./cmd/` (describe/mock tests), and `./internal/exec/` mock, state, and output
  tests. A full `./internal/exec/` run timed out locally on the unrelated `TestVendorPullBasicExecution`.
- End-to-end on a copy of `examples/terraform-component-mocks`: mock with no state, real value after
  applying `vpc`, `--use-mocks=always` and `ATMOS_USE_MOCKS=always` forcing the mock, an earlier
  `edition` pin behaving like `always`, invalid values and `terraform apply --use-mocks` rejected.
- `cd website && npm run build`: succeeded.
- `./custom-gcl run --new-from-rev=origin/main`: 0 issues. Screengrabs were not regenerated locally.

## Follow-ups

None.
