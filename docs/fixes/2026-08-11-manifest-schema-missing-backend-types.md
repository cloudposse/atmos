# Fix: Stack-manifest JSON Schema rejected real, documented, Go-supported fields (backend types, overrides, retry, required_version/providers, source.ttl, and a root oneOf bug)

**Date:** 2026-08-11

## Summary

The embedded stack-manifest JSON Schema (`schemas.atmos.manifest`) hardcoded an 8-value `backend_type` /
`remote_state_backend_type` enum (`local, s3, remote, vault, static, azurerm, gcs, cloud`) and a
`backend_manifest` object with `additionalProperties: false` covering only those same 8 keys. Any manifest
using `backend_type: http` (or `consul`, `cos`, `kubernetes`, `oss`, `pg`) failed hard schema validation,
even though `website/docs/components/terraform/backends.mdx` already documents Atmos as supporting all
Terraform/OpenTofu backends, including those six ([GitHub issue #2919](https://github.com/cloudposse/atmos/issues/2919)).

Fixing that surfaced the same bug class elsewhere: a `field-test` pass over the fix (and the class of
problem it represents) found the embedded schema also hard-rejecting `terraform.overrides.{hooks,generate,
secrets,auth,retry,required_providers,required_version}`, component-level `retry:` entirely, component-level
`required_version`/`required_providers`, `source.ttl`, and `container_runtime.provider: "auto"` — all real,
documented, Go-read fields. It also found one field where the schema was too *permissive*
(`workflow_step.output.mode: "raw"`, which Go rejects), a silent (non-erroring) misconfiguration bug in
backend-type/backend-key handling, and a root-schema `oneOf` bug that made `workflows:` and ordinary
stack-manifest fields (`vars:`, `settings:`, etc.) mutually exclusive in one manifest with a useless "valid
against schemas at indexes 0 and 1" error. All of these are fixed here in one pass.

## Context

`atmos describe stacks` (and anything that calls it internally, e.g. `terraform plan`, custom commands)
failed schema validation for manifests using `backend_type: http` — a real, generic Terraform/OpenTofu
backend (used e.g. for GitLab-managed state via `.../api/v4/projects/:id/terraform/state/:name`). The
failure wasn't opt-in (`atmos validate stacks`); it blocked ordinary stack computation with no supported
way to allow it short of hand-patching the embedded schema.

**Why now (regression trigger).** The enum has been missing these types for a long time, but it only
started failing between v1.222.0 and v1.225.0:

- `atmos describe stacks` has always wired `exec.ValidateStacks` directly into its command pipeline
  (`cmd/describe_stacks.go:33`) — schema validation isn't opt-in for `describe stacks`, it's part of
  normal execution.
- Until PR #2749 (merged 2026-07-15, commit `9ceed635a1`, *"fix(validate): wire embedded schema path when
  no manifest override is set"*), `ValidateStacks` had a bug: when `schemas.atmos.manifest` wasn't
  configured (the default — matches the issue reporter's environment), it resolved the embedded default
  schema file but assigned it to the wrong local variable (`manifestSchema.Manifest = f` instead of
  `atmosManifestJsonSchemaFilePath = f`), so the path actually used downstream stayed empty and JSON
  Schema validation **silently no-op'd**. Two acceptance tests never caught it because they always passed
  an explicit schema path.
- That fix correctly wired up the default embedded schema. For the first time, `describe stacks` (and
  anything that calls it) actually validated against the embedded manifest schema by default — turning a
  whole class of pre-existing schema gaps (not just `backend_type`) from a silent pass into hard failures.
- Confirmed empirically: building this repo's HEAD (before this fix) and running `describe stacks` against
  a `backend_type: http` fixture reproduced the exact error text from the issue.

So this wasn't new schema drift — it was a previously-silent gap (or several) that a legitimate, unrelated
validation bug fix turned into user-visible hard failures.

Scope: schema validation and the backend_type/backend key-matching check only. `!terraform.state`
(`internal/terraform_backend/`) still only implements backend readers for `local/s3/gcs/azurerm` — issue
#1384 tracks adding an `http` reader there separately; that's a different, larger feature, not a schema
gap. `vault` and `static` were left untouched in the `backend_type` enum: `static` is a real Atmos-specific
`remote_state_backend_type` (documented in `remote-state.mdx`), and `vault` is unclear/likely stale, but
removing either was out of scope and carries its own risk.

## Changes

### 1. Missing backend types (issue #2919)

Added `consul`, `cos`, `http`, `kubernetes`, `oss`, `pg` to the `backend_type` enum, the
`remote_state_backend_type` enum, and as new keys under `backend_manifest.properties` in all three
hand-maintained schema copies (`pkg/datafetcher/schema/atmos/manifest/1.0.json` — the embedded, default-
enforced source of truth; `pkg/datafetcher/schema/stacks/stack-config/1.0.json` — a near-duplicate kept in
sync by hand, no generator; `tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` — test
fixture copy). `website/docs/components/terraform/remote-state.mdx`'s two stale inline
`backend_type`/`remote_state_backend_type` comments were updated to match.

### 2. Same bug class, other fields (found via `field-test`)

All added to the same three schema files, ported from wherever a copy already had the correct shape
(`stack-config/1.0.json` already had `retry`/`required_version`/`required_providers` definitions for
terraform):

- `terraform.overrides`: added `hooks`, `generate`, `secrets`, `auth`, `retry`, `required_providers`,
  `required_version` (previously only `command`/`vars`/`env`/`settings`/`providers`) —
  `website/docs/stacks/overrides.mdx` documents `overrides.hooks`/`overrides.generate` directly.
- Component-level `retry:`: added a new shared `#/definitions/retry` (ported from `stack-config/1.0.json`,
  which already had it correct, including the `conditions` field) and wired it into `terraform`,
  `terraform_component_manifest`, `helmfile`, `helmfile_component_manifest`, `packer`,
  `packer_component_manifest`, and `kubernetes_component_manifest` — `retry` is extracted for every
  component type by `internal/exec/stack_processor_process_stacks_helpers_extraction.go`, but the embedded
  schema had no `retry` property anywhere before this.
- `terraform.required_version` / `required_providers` at the component level: added new
  `#/definitions/required_version` / `#/definitions/required_providers` (ported from `stack-config/1.0.json`)
  and wired into `terraform` and `terraform_component_manifest`.
- `source.ttl`: added to the `source` definition — read by `pkg/provisioner/source/extract.go` and
  documented at `website/docs/cli/commands/terraform/source/source.mdx`.
- `container_runtime.provider`: added `"auto"` to the enum (was `["", "docker", "podman"]`) —
  `pkg/schema/container_config.go` documents `"auto"` as a distinct literal from `""`, and
  `website/docs/cli/commands/container/usage.mdx` shows `provider: auto` in examples.
- `helmfile` (global section) and `helmfile_component_manifest`: added `hooks` — `helmfile` is in
  `supportsComponentHooks()`'s allow-list in `stack_processor_process_stacks_helpers_extraction.go` but the
  schema never had the property. (`packer` was checked and correctly has no `hooks` — it's not in that
  Go allow-list, so its absence there was not a bug.)
- `kubernetes_component_manifest`: added `secrets` (it already had `auth`) — secrets declarations are
  extracted for all component types per that same extraction file's comment.
- `workflow_step.output.mode`: **removed** `"raw"` from the enum (`grouped`/`prefixed`/`none` remain) — the
  opposite direction of every fix above: the schema was too permissive and Go's
  `pkg/schema/task_validate.go` `validateParallelOutput` never accepted `"raw"`, so it passed validation
  and then failed at workflow-execution time with a much less actionable error.

### 3. Root schema `oneOf` bug (confusing "valid against schemas at indexes 0 and 1" error)

The root schema's `oneOf` had two branches: `{required: [workflows]}` and an `anyOf` whose *first* disjunct
alone was `{additionalProperties: true, not: {required: [workflows]}}` — which matches ANY object lacking
`workflows`, making every other disjunct in that `anyOf` (`required: [vars]`, `required: [terraform]`, ...)
dead weight. Combining `workflows:` with e.g. `vars:` matched *both* root branches at once (branch 0 via
`workflows`, branch 1 via the unrelated `vars` disjunct), so `oneOf` failed with an uninformative
`(root): valid against schemas at indexes 0 and 1` error at position `0:0`.

First attempted fix: wrap the `not: {required: [workflows]}` guard around the whole `anyOf` instead of just
its first disjunct. This resolved the ambiguity but was **wrong** — it silently changed behavior for
manifests with none of the enumerated fields, which broke a real fixture
(`tests/test-cases/validate-type-mismatch/stacks/mixins/subnet-config.yaml`, a merge-only mixin fragment
whose only top-level key is `subnets:`, not in the `anyOf` list) that only ever passed because of the old
catch-all disjunct. Caught immediately by `go test ./internal/exec/...`
(`TestValidateStacksWithMergeContext`, `TestMergeContextErrorFormatting` failed). The real, minimal, correct
fix: collapse branch 1 to exactly `{not: {required: [workflows]}}`, dropping the field-enumeration `anyOf`
entirely — it was never doing anything except by accident. This resolves the ambiguity while exactly
preserving the schema's original (if accidental) permissiveness for any non-workflow shape.

### 4. Silent backend_type/backend key mismatch (Go-level fix, not schema)

Found live during the field test: `backend_type: http` with `backend: {s3: {...}}` (a plausible copy-paste
mistake) passed schema validation and silently resolved to an **empty** backend config — no error,
no warning. Root cause: `processTerraformBackend` in `internal/exec/stack_processor_backend.go` looked up
`finalComponentBackendSection[finalComponentBackendType]`, and on a miss just left `finalComponentBackend`
as `{}` instead of treating it as a config error. Same bug existed for `remote_state_backend_type` /
`remote_state_backend` in `processTerraformRemoteStateBackend`.

Fixed by erroring (new sentinel `errors.ErrBackendTypeMismatch`, built via the error-builder pattern with
`WithContext`/`WithHintf`) when the resolved type doesn't match any key actually configured under
`backend:`/`remote_state_backend:`, but *only* when the section is non-empty and the type itself is
non-empty — an unset `backend_type` with a malformed/flat `backend:` section is a separate, already-handled
case (`terraform_generate_backends.go` skips generation and logs a warning), not a mismatch; the first
attempt at this check didn't have that guard and broke ~40 existing `internal/exec` tests
(`TestBackendGenerationSkipsComponentsWithoutBackend` and every `Auth*`/`Nested*` test that happened to
share a component with no `backend_type` configured) before the guard was added and the suite went green
again.

## Validation

- `go build ./...`, `go vet ./...` — pass.
- `go test ./pkg/datafetcher/... ./pkg/validator/... ./internal/exec/... ./errors/...` — pass, including
  new tests: `TestManifestSchema_BackendTypeCoverage` (14 backend types × backend + remote_state_backend,
  across 4 schema copies, plus an enum-isolated negative case and a separate key-rejection negative case),
  `TestManifestSchema_OverridesFieldCoverage`, `TestManifestSchema_ComponentLevelRetry`,
  `TestManifestSchema_RequiredVersionAndProviders`, `TestManifestSchema_KubernetesComponentSecrets`,
  `TestManifestSchema_ContainerRuntimeProviderAuto`, `TestManifestSchema_SourceTTLField`,
  `TestManifestSchema_ParallelStepOutputMode`, `TestManifestSchema_RootOneOfAllowsWorkflowsWithStackFields`,
  `TestProcessTerraformBackend_TypeKeyMismatch`, `TestProcessTerraformRemoteStateBackend_TypeKeyMismatch`.
- New CLI-level regression fixture: `tests/fixtures/scenarios/manifest-schema-coverage/` (a single stack
  manifest combining `backend_type: http`, `overrides.*`, component-level `retry`/`required_version`/
  `required_providers`, `source.ttl`, and `workflows:` alongside `vars:`/`settings:`) plus
  `tests/test-cases/manifest-schema-coverage.yaml`, which runs `atmos validate stacks` and
  `atmos describe stacks --format=json` against it through the real CLI binary and asserts success —
  `go test ./tests -run 'TestCLICommands/manifest-schema-coverage'` passes. Confirmed this test fails
  against the pre-fix schema (reproduced the exact validation errors first, then fixed). Also ran the full
  `go test ./tests -run TestCLICommands -short` suite (270s) clean.
  `container_runtime.provider: "auto"` is covered at the schema-unit level only, not in this CLI fixture,
  since exercising it end to end needs a separate container-component fixture.
- Manual repro (built the CLI from this branch): before the fix, `atmos describe stacks` against a fixture
  using `backend_type: http` failed with the exact errors from issue #2919; after, it succeeds. Ran this
  before AND after the schema JSON changes to confirm cause and effect. Also manually reproduced and then
  fixed the root-`oneOf` ambiguity and the silent backend-mismatch bug the same way.
- `cd website && npm run build` — succeeds (the one broken-anchor warning in the build output is
  pre-existing, in an unrelated blog post, not touched by this change).
- Not run: `./build/atmos test` (the full suite via the `atmos test` wrapper) hit a pre-existing timeout in
  this sandboxed environment unrelated to this change (an outbound HTTPS/HTTP2 connection stuck in I/O
  wait). Ran the equivalent `go test ./tests -run TestCLICommands -short` directly instead, successfully.
- **Environment note:** the CLI test harness (`tests/cli_test.go`) resolves the `atmos` binary via
  `exec.LookPath("atmos")`, i.e. whatever is first on `$PATH` — in this environment that's a pre-existing
  `/opt/homebrew/bin/atmos`, not this branch's build. Every CLI-level test run above used
  `PATH="$(pwd)/build:$PATH"` to make sure the locally-built binary (with these fixes) was actually being
  exercised, not a stale system install.

## Follow-ups

- Issue #1384 (already open, not filed by this fix): `!terraform.state` still lacks an `http` backend
  reader in `internal/terraform_backend/`. Schema validation now accepts `backend_type: http`, but reading
  remote state from an `http` backend via the `!terraform.state` YAML function still requires that
  separate feature.
- The three-way hand-maintained schema duplication remains a standing risk for this exact class of bug
  recurring on the next new field/enum value. No generator unifies them today; the tests added here and the
  pre-existing `TestManifestSchema_KubernetesComponentValidateField` are the only guardrails, plus the new
  CLI-level fixture for the fields most likely to matter together in practice. Not opening a new issue for
  this — it's a known, accepted structural tradeoff, not a gap introduced by this fix.
- `stack-config/1.0.json` has no `component_secrets` definition wired anywhere (component-level `secrets:`
  is unsupported there for every component type, not just kubernetes) and its `workflow_manifest` /
  `workflow_step` definitions are far more restrictive than the embedded schema's (found during the field
  test, Tier E in that report). Neither blocks default CLI validation (`stack-config.json` isn't the schema
  `atmos describe stacks`/`validate stacks` enforce by default — mainly SchemaStore/IDE and docs), so left
  as-is rather than expanding scope further in this pass.
