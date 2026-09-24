# Fix: Authenticate only when a requested value needs credentials

**Date:** 2026-09-23

## Summary

List and describe commands defer default-identity authentication until a requested
value needs credentials. Unused values never trigger authentication, including
when the configured default identity would fail.

## Context

Listing stack names in `my-landing-zone` attempted to authenticate its default
emulator identity before determining which fields the output needed. A stopped
emulator therefore prevented inventory output, even though no displayed value
depended on credentials. Tree/provenance and deferred-merge reprocessing also
needed the same evaluation boundary as the normal output path.

## Changes

- Added `pkg/deferred` for required sections/fields, transitive template dependencies,
  conservative dynamic-expression handling, invocation-local authentication caching,
  and per-value recovery. Existing evaluators retain execution responsibilities.
- Kept provider error classification at provider boundaries. Shared evaluation
  consumes a provider-neutral authentication-unavailable error, not SDK codes or
  emulator-specific exceptions. AWS classification lives in
  `pkg/auth/cloud/aws/autherrors`; emulator classification remains in its identity
  implementation.
- Applied required-field filtering consistently, including filters, dependency
  traversal, static queries, tree/provenance, and final deferred merges.
- Preserve successful sibling values when requested credentials are unavailable.
  `warn` substitutes typed `(computed)` values with one accurately counted summary;
  `silent` substitutes without a summary; `strict` fails.
- Explicit `--identity` and `ATMOS_IDENTITY` still authenticate before output and
  fail fast. `--identity=false` disables Atmos authentication. Failed configured
  identities cannot fall through to ambient credentials.
- Malformed configuration, invalid expressions, and dependency cycles remain
  fatal. Terraform execution/preflight and narrow Terraform/YQ defaults retain
  their existing strict policies.
- PR preparation also preserves the workspace's TUI entrypoint restoration:
  bare `atmos` opens the picker when stacks exist and shows help otherwise.
  Extracted small TUI helpers and corrected test path joins to satisfy pre-commit
  lint without changing behavior.
- PR review fixes make environment-selected identities fail fast in `describe
  component`, including identity-backed stores, and restrict the bare-command
  picker to interactive stdin and stdout.
- Restored resolved-value caching for deferred Terraform state and component
  references. Caches are invocation-local and partitioned by target stack,
  component, and configuration (including inherited identities), not identity
  names alone. Failed, masked, and computed results cannot populate these caches.
  Warm static reads retain missing-output errors, and deferred backend reads
  discard stale caller credentials when no authenticated context is resolved.
- Shared stores explicitly reset runtime credentials and SDK clients before
  rebinding; disabled or absent authentication cannot retain a prior account.
- AWS access-denied responses remain authorization errors, not missing
  credentials. Authentication classification preserves remediation hints.
- Added direct unit coverage for store parsing/query/default behavior, lazy secret
  identities, provider context mapping, recovery policy, deferred-merge demand
  boundaries, and TUI selection. Coverage targets and ignore rules are unchanged.

## Validation

- Added deterministic default-emulator fixtures with unused Terraform state,
  AWS, and template values. Tested inventory formats and requested-value modes,
  cache isolation, nested defaults, disabled authentication, sibling preservation,
  and zero backend access after failed authentication.
- Focused list/describe, deferred evaluation, provider, auth, store, and template
  tests passed. `go build ./...` and the documentation website build passed.
- PR-preparation root/TUI tests, affected-package builds, and patch-scoped lint
  passed after the TUI lint cleanup.
- Verified the workspace binary against `/tmp/my-landing-zone` with the emulator
  stopped: default/table/JSON/YAML stack names and tree/provenance succeed in strict
  mode. Requested KMS values produce three computed markers and one three-value
  summary in warn mode, no summary in silent mode, and a failure in strict mode.
  Explicit flag/environment identities fail; disabled authentication succeeds.
- The repository-wide `atmos test` run did not pass: the `tests` package reached
  its timeout. A subsequent `atmos test --full` run was interrupted when PR
  preparation stopped at the pre-commit gate; full-suite validation is incomplete.
- The demo configuration, emulator state, and installed Homebrew binary were
  left unchanged.
- Review regressions passed for explicit environment selection, headless root
  fallback, store auth/client reset, same-name identity isolation, repeated state
  and template reads, failure and masked-value cache isolation, AWS authorization
  classification, and authentication remediation hints.
- Review validation also passed the affected command/auth/store/deferred short
  suites, focused exec tests, website build, and patch-scoped lint. Rechecked the
  workspace binary against the stopped demo emulator, including explicit
  environment selection with YAML-function processing disabled.
- Coverage follow-up: `go test ./pkg/deferred ./pkg/store/authbridge -count=1
  -coverprofile=.context/deferred-coverage.out` passed with 99.8% and 100.0%
  statement coverage respectively. The same packages passed `go test -race`.
  Focused exec and command coverage tests passed. Combining their profiles with
  the prior Codecov report projects 89.36% patch coverage (865/968 lines); this
  local projection is not a replacement for Codecov's post-push calculation.

## Follow-ups

None.
