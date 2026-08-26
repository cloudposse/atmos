# Fix: CloudFormation `template:` becomes inline body, new `path:` key for file references

**Date:** 2026-08-25

## Summary

`aws/cloudformation` components had no way to author a CloudFormation template directly in stack
YAML — `template:` was strictly a file path, unlike Kubernetes's `manifests:` (string-or-inline-object
entries). This is a breaking rename: **`template:` now means the inline template body** (a literal
string or a structured map), and **a new `path:` key takes over `template:`'s old file-reference
role**. Setting both is an error.

## Context

Found during a follow-up audit this session (paired with the `pkg/ci/artifact/s3` auth fix and CFN
hooks live-verification). CFN's one-template-per-stack cardinality doesn't have Kubernetes's
"additive multiple sources" case, so — after discussion — the design settled on an explicit
two-key rename rather than auto-detecting inline-vs-file from the value shape: auto-detection would
degrade a typo'd file path's error from a clear "file not found" to a confusing CFN-side "invalid
template," the same class of silent-misinterpretation bug fixed elsewhere this session.

## Changes

**Config/schema:**
- `pkg/config/const.go`: new `TemplatePathSectionName = "path"`.
- `errors/errors.go`: new sentinels `ErrAwsCloudFormationTemplateAndPathMutuallyExclusive`,
  `ErrAwsCloudFormationTemplateMissingResources`, `ErrAwsCloudFormationFmtRequiresPath`;
  `ErrMissingAwsCloudFormationTemplate`'s message updated to name both keys.
- `pkg/datafetcher/schema/atmos/manifest/1.0.json`: `template` changed from a plain string to
  `oneOf: [string, object]`; added `path` (string).

**`pkg/component/aws/cloudformation/spec.go`**: new `resolveTemplateSection` — reads both
`template` (polymorphic: `string` used as the body verbatim, `map[string]any` marshaled to YAML via
`gopkg.in/yaml.v3`, reusing this package's existing YAML dependency from `fmt.go` rather than adding
`sigs.k8s.io/yaml` for one call site) and `path` (unchanged, string-only). Both set → mutual-exclusion
sentinel. `buildStackSpec` pre-populates `spec.TemplateBody` when `template:` resolves to a non-empty
body — this is the signal `resolveSpecAndTemplate` uses to skip the disk read.

**`pkg/component/aws/cloudformation/executor.go`**: `resolveSpecAndTemplate` guards the existing
`loadTemplateBody` disk read with `if spec.TemplateBody == ""` — inline case already has it
populated, `path:` case still reads from disk exactly as before.

**`pkg/component/aws/cloudformation/fmt.go`**: `runFmt` rejects an inline (no on-disk file) template
with the new `ErrAwsCloudFormationFmtRequiresPath` instead of attempting `os.WriteFile("", ...)` —
`fmt` formats a file in place; an inline `template:` body has no file to format.

**`pkg/component/aws/cloudformation/validate.go`**: `validateComponentConfig` (the local,
no-AWS-credentials-needed pre-flight check) now: rejects both-set (mutual exclusion) and
neither-set (existing missing-template sentinel); for an inline `template:` (string or map), runs
`sanityCheckInlineTemplate` — parses string bodies with `yaml.Unmarshal` (surfacing the parser's own
line:column in its error text on a syntax mistake) and checks for a top-level `Resources` key (CFN's
one truly-required section) before ever reaching AWS's `ValidateTemplate` API. File-existence
checking for `path:` was intentionally **not** added here: this function is called through the
shared `component.ComponentProvider.ValidateComponent(config map[string]any) error` interface, which
receives only the raw component config, not the resolved component base path — changing that
interface would ripple across every component type (Kubernetes, Helm, Ansible, Container, Custom,
Emulator). A typo'd `path:` is instead caught, with a clear file-naming error, by the existing
`loadTemplateBody` disk read one step later in the real (non-abstract) execution path.

**Two real plumbing gaps found and fixed during live verification** (not originally in scope, but
required for the feature to work end-to-end — unit tests alone didn't catch these since they
construct `map[string]any` directly, bypassing real stack processing):
1. `internal/exec/stack_processor_process_stacks_helpers_extraction.go`'s
   `cloudFormationComponentSectionKeys` — the allowlist `extractCloudFormationComponentSection` uses
   to carry native CFN fields through base-component inheritance — was missing the new `path` key,
   so it was silently dropped from every component's merged config before `ComponentProvider.
   ValidateComponent`/`Execute` ever saw it. Added `cfg.TemplatePathSectionName`.
2. `internal/exec/describe_component.go`'s `FilterComputedFields` — the separate allowlist gating
   `describe component`'s default (`describe.component.filter: schema`) output — was also missing
   `path`, so `atmos describe component` silently hid it from users even after fix #1. Added
   `cfg.TemplatePathSectionName` alongside the other CFN-specific keys already there.

   Both are the same class of bug this session already found and fixed once for `provision`/`source`/
   Helm's chart/values/values_files/repositories (see the pre-existing comments in
   `FilterComputedFields` referencing that prior fix) — a new component-config key needs plumbing in
   at least these two allowlists (in addition to the JSON schema and typed struct), confirmed via a
   completely isolated, zero-inheritance repro fixture (a fresh `atmos.yaml`/stack outside this repo)
   before landing the fix, to rule out anything specific to `examples/cloudformation`'s catalog/local
   inheritance chain.

**Fixtures/docs (breaking-rename update):**
- `examples/cloudformation/stacks/catalog/demo.yaml`: `demo`/`demo-broken`'s `template:
  template.yaml` → `path: template.yaml`; added `demo-inline`, a new component using inline
  `template:` (string form) with `{{ .vars.stage }}` interpolation, to close the zero-existing-coverage
  gap for this new capability.
- `examples/cloudformation/stacks/deploy/local.yaml`: added `demo-inline` (`metadata.type: real`).
- `tests/fixtures/scenarios/aws-cloudformation-outputs/stacks/test.yaml`: same rename (unaffected
  functionally — this fixture never reaches `buildStackSpec`/`validateComponentConfig` — updated for
  accuracy since it references a real on-disk file).
- `website/docs/stacks/components/aws-cloudformation.mdx`: `template`/`stack_policy` section rewritten
  to document both keys and both `template:` forms, with a new inline-template example; "Source
  Provisioning" section's `template:` references updated to `path:`.
- `website/docs/migration/from-rain.mdx`: all four `template: template.yaml` examples → `path:
  template.yaml`; the "Atmos never preprocesses..." sentence scoped to file-based (`path:`) templates.

## Validation

- Bug Fixing Workflow: new tests written first for `buildStackSpec`/`validateComponentConfig`/
  `runFmt`, confirmed failing against the old behavior, then made to pass by the implementation.
- New/updated tests: `spec_test.go` (`TestBuildStackSpec_PathIsFileReference/InlineStringTemplate/
  InlineMapTemplate/TemplateAndPathMutuallyExclusive`, `TestBuildStackSpec_FullConfig` updated to
  `path:`), `validate_test.go` (5 new `TestValidateComponentConfig` cases), `fmt_test.go`
  (`TestRunFmt_InlineTemplate_RequiresPath`), `executor_test.go`/`cloudformation_test.go` (existing
  `template:` fixtures renamed to `path:` throughout), `pkg/datafetcher/schema_section_coverage_test.go`
  (`"path"` added to `nonManifestSections` for the new CFN sub-field).
- `go build ./... && go test ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/...
  ./internal/exec/... ./pkg/datafetcher/... ./pkg/config/... ./errors/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains:
  `cmd/terraform/utils.go:484`).
- Live, against a real local Floci AWS emulator (`examples/cloudformation`):
  - `path:` regression: `demo`/`demo-broken` `validate` and `demo-broken` `validate` both succeed
    unchanged.
  - New capability: `demo-inline` `describe component` confirms `{{ .vars.stage }}` resolves to
    `local` inside the inline body before it's sent anywhere; `validate` and `deploy --auto-approve`
    both succeed, creating a real SSM parameter (`/atmos/demo-inline/local/marker`) in the emulator;
    cleaned up with `delete --auto-approve`.
  - Mutual exclusion: a component with both `template:` and `path:` set fails
    `validate` with `ErrAwsCloudFormationTemplateAndPathMutuallyExclusive`, naming the stack.
  - Confirmed via an isolated, zero-inheritance scratch fixture (outside this repo) that a brand-new
    `path` key survives `describe component`/real command execution end-to-end after both allowlist
    fixes — this is what caught plumbing gap #1 and #2 above; both were invisible to the package's own
    unit tests.
- `cd website && npm run build` — succeeded (pre-existing, unrelated broken-anchor warning on the
  `mcp-for-ai-coding-assistants` changelog page).

## Follow-ups

None.
