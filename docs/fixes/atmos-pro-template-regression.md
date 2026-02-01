# Regression: Template variables `.atmos_component` and `.atmos_stack` fail in 1.205

**Date:** 2026-01-28

## Issue Summary

Starting in Atmos 1.205, stack manifests that use `{{ .atmos_component }}` or `{{ .atmos_stack }}`
in non-template files (without `.tmpl` extension) fail with:

```text
Error: failed to execute describe stacks: invalid stack manifest: template: mixins/atmos-pro.yaml:4:21:
executing "mixins/atmos-pro.yaml" at <.atmos_component>: map has no entry for key "atmos_component"
```

This worked correctly in Atmos 1.204.

## Affected Configuration

Example `mixins/atmos-pro.yaml`:

```yaml
plan-wf-config: &plan-wf-config
  atmos-terraform-plan.yaml:
    inputs:
      component: "{{ .atmos_component }}"
      stack: "{{ .atmos_stack }}"

apply-wf-config: &apply-wf-config
  atmos-terraform-apply.yaml:
    inputs:
      component: "{{ .atmos_component }}"
      stack: "{{ .atmos_stack }}"
      github_environment: "{{ .atmos_stack }}"

settings:
  pro:
    enabled: true
    pull_request:
      opened:
        workflows: *plan-wf-config
      # ... more workflow configs
```

## Root Cause Analysis

The regression was introduced in commit `6ae0a2715` ("Resolve file-scoped locals in stack configurations").

### Previous Behavior (1.204)

In 1.204, template processing during import was controlled by this condition in `stack_processor_utils.go`:

```go
if !skipTemplatesProcessingInImports && (u.IsTemplateFile(filePath) || len(context) > 0) {
    stackManifestTemplatesProcessed, tmplErr = ProcessTmpl(...)
}
```

For non-`.tmpl` files imported without explicit context:
- `u.IsTemplateFile(filePath)` = false (no `.tmpl` extension)
- `len(context)` = 0 (no context passed)
- **Result: Templates NOT processed during import** ✓

Templates like `{{ .atmos_component }}` were left unresolved during import and only processed
later in `describe_stacks.go` when component context was available.

### New Behavior (1.205)

The locals feature added `extractAndAddLocalsToContext()` which extracts `settings`, `vars`,
`env`, and `locals` from the file itself and adds them to the context:

```go
// New code in extractAndAddLocalsToContext
if extractResult.settings != nil {
    context[cfg.SettingsSectionName] = extractResult.settings
}
if extractResult.vars != nil {
    context[cfg.VarsSectionName] = extractResult.vars
}
```

Now for files with a `settings`, `vars`, `env`, or `locals` section:
- `u.IsTemplateFile(filePath)` = false (no `.tmpl` extension)
- `len(context)` > 0 (context now contains settings/vars/env/locals from the file!)
- **Result: Templates ARE processed during import** ✗

But `atmos_component` and `atmos_stack` are NOT in the context because they're only set
later during component processing in `describe_stacks.go`.

## Fix

The fix tracks whether context was "originally provided" vs "populated from file extraction".
Template processing during import should only occur when:

1. The file has a `.tmpl` extension, OR
2. Context was explicitly passed from outside (not just extracted from the file itself)

### Implementation

Modified `ProcessBaseStackConfig` in `internal/exec/stack_processor_utils.go`:

1. Added tracking of original context before locals extraction:
```go
// Track whether context was originally provided from outside (e.g., via import context).
originalContextProvided := len(context) > 0
```

2. Changed the template processing condition from:
```go
// OLD (1.205 bug):
if !skipTemplatesProcessingInImports && (u.IsTemplateFile(filePath) || len(context) > 0) {
```

To:
```go
// NEW (fixed):
if !skipTemplatesProcessingInImports && (u.IsTemplateFile(filePath) || originalContextProvided) {
```

This ensures templates like `{{ .atmos_component }}` are NOT processed during import when the
only reason context is non-empty is because of file-extracted locals/settings/vars/env.

## Steps to Reproduce

1. Create `mixins/atmos-pro.yaml` with `settings` section and `{{ .atmos_component }}` templates
2. Import this mixin in a stack
3. Run `atmos describe stacks`

### Expected Result (1.204)

Stack description succeeds. Template variables are resolved later when component context is available.

### Actual Result (1.205)

```text
Error: failed to execute describe stacks: invalid stack manifest: template: mixins/atmos-pro.yaml:4:21:
executing "mixins/atmos-pro.yaml" at <.atmos_component>: map has no entry for key "atmos_component"
```

## Test Case

See `internal/exec/stack_processor_utils_test.go`:
- `TestTemplateProcessingWithAtmosComponentInNonTemplateFile`
