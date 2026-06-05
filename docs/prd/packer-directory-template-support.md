# PRD: Packer Directory-Based Template Support

## Problem Statement

**GitHub Issue:** [#1937](https://github.com/cloudposse/atmos/issues/1937)

**Status:** ✅ Implemented

Currently, Atmos requires users to specify a single HCL file via `--template` flag or `settings.packer.template`
configuration. This prevents users from organizing Packer components using the standard multi-file pattern where:

- `variables.pkr.hcl` - Variable declarations
- `template.pkr.hcl` or `main.pkr.hcl` - Source and build blocks
- `locals.pkr.hcl` - Local values
- `plugins.pkr.hcl` - Required plugins

When users split their Packer configuration across multiple files (a recommended practice), Atmos only loads the
specified template file, causing "Unsupported attribute" errors when variables are defined in separate files.

### Previous Behavior

```go
// internal/exec/packer.go:194-196
if template == "" {
    return errUtils.ErrMissingPackerTemplate
}
```

The template is then appended as the last argument to Packer:

```go
// internal/exec/packer.go:238
allArgsAndFlags = append(allArgsAndFlags, template)
```

This resulted in: `packer build -var-file vars.json template.pkr.hcl`

### Packer's Native Behavior

From [HashiCorp Packer documentation](https://developer.hashicorp.com/packer/docs/templates/hcl_templates):

> "When a directory is passed, all files in the folder with a name ending with `.pkr.hcl` or `.pkr.json` will be parsed
> using the HCL2 format."

Native Packer supports:

- `packer build .` - Load all `*.pkr.hcl` files from component working directory
- `packer build ./my-component/` - Load all `*.pkr.hcl` files from specified directory
- `packer build template.pkr.hcl` - Load single file (previous Atmos behavior)

## Solution

Make the `template` parameter optional with a default of `"."` (component working directory), allowing Packer to load all
`*.pkr.hcl` files from the component directory.

### Behavior Changes

| Scenario                   | Previous                             | Current                                  |
|----------------------------|--------------------------------------|------------------------------------------|
| No template specified      | Error: `packer template is required` | Uses `"."` - loads all `*.pkr.hcl` files |
| `template: "."`            | Error (if not specified)             | Passes `"."` to Packer                   |
| `template: "main.pkr.hcl"` | Works                                | Works (no change)                        |
| `--template .` flag        | Not commonly used                    | Passes `"."` to Packer                   |

## Implementation Summary

### Core Change

**File:** `internal/exec/packer.go` (lines 194-200)

```go
// If no template specified, default to "." (component working directory).
// Packer will load all *.pkr.hcl files from the component directory.
// This allows users to organize Packer configurations across multiple files
// (e.g., variables.pkr.hcl, main.pkr.hcl, locals.pkr.hcl).
if template == "" {
    template = "."
}
```

### Test Fixtures Created

**Directory:** `tests/fixtures/scenarios/packer/components/packer/aws/multi-file/`

| File                | Purpose                                         |
|---------------------|-------------------------------------------------|
| `variables.pkr.hcl` | Variable declarations (region, ami_name, etc.)  |
| `main.pkr.hcl`      | Source and build blocks that reference vars     |
| `manifest.json`     | Pre-generated manifest for output command tests |
| `README.md`         | Documentation for the test component            |

**Stack Configurations:**

| File                                            | Changes                                      |
|-------------------------------------------------|----------------------------------------------|
| `stacks/catalog/aws/multi-file/defaults.yaml`   | Component defaults with no template setting  |
| `stacks/deploy/prod.yaml`                       | Added `aws/multi-file` component             |
| `stacks/deploy/nonprod.yaml`                    | Added `aws/multi-file` component             |

### Unit Tests Added

**File:** `internal/exec/packer_test.go`

| Test Function                          | Description                                          |
|----------------------------------------|------------------------------------------------------|
| `TestExecutePacker_DirectoryMode`      | Tests directory mode with no template and explicit "." |
| `TestExecutePacker_MultiFileComponent` | Tests multi-file component with separate variables.pkr.hcl |

### Integration Tests Added

**File:** `tests/test-cases/packer.yaml`

| Test Name                                   | Description                                     |
|---------------------------------------------|-------------------------------------------------|
| `packer version`                            | Basic version command                           |
| `packer validate single-file`               | Validate with explicit template (existing)      |
| `packer validate directory-mode`            | Validate with default "." template              |
| `packer validate explicit-dot`              | Validate with explicit --template .             |
| `packer inspect directory-mode`             | Inspect multi-file component                    |
| `packer output directory-mode`              | Output from multi-file component manifest       |
| `packer describe component directory-mode`  | Describe component with no template setting     |

### Documentation Updates

| File                                                | Changes                                        |
|-----------------------------------------------------|------------------------------------------------|
| `website/docs/stacks/components/packer.mdx`         | Added "Template Configuration" section         |
| `website/docs/cli/commands/packer/usage.mdx`        | Updated `--template` flag description          |
| `website/docs/cli/commands/packer/packer-build.mdx` | Added Directory Mode and Single File examples  |

## File Changes Summary

| File                                                                        | Change Type | Description                                |
|-----------------------------------------------------------------------------|-------------|--------------------------------------------|
| `internal/exec/packer.go`                                                   | Modified    | Default template to "." instead of error   |
| `internal/exec/packer_test.go`                                              | Modified    | Added directory mode unit tests            |
| `tests/test-cases/packer.yaml`                                              | Created     | Integration tests for Packer commands      |
| `tests/fixtures/scenarios/packer/components/packer/aws/multi-file/*`        | Created     | Multi-file test component                  |
| `tests/fixtures/scenarios/packer/stacks/catalog/aws/multi-file/defaults.yaml` | Created   | Stack defaults for multi-file component    |
| `tests/fixtures/scenarios/packer/stacks/deploy/prod.yaml`                   | Modified    | Added multi-file component                 |
| `tests/fixtures/scenarios/packer/stacks/deploy/nonprod.yaml`                | Modified    | Added multi-file component                 |
| `website/docs/stacks/components/packer.mdx`                                 | Modified    | Document directory mode                    |
| `website/docs/cli/commands/packer/usage.mdx`                                | Modified    | Update --template flag docs                |
| `website/docs/cli/commands/packer/packer-build.mdx`                         | Modified    | Add directory mode examples                |

## Acceptance Criteria

All criteria have been met:

1. ✅ **Default Behavior:** Running `atmos packer build component -s stack` without `--template` or
   `settings.packer.template` works by defaulting to "."

2. ✅ **Multi-File Support:** A Packer component with separate `variables.pkr.hcl` and `main.pkr.hcl` files builds
   successfully without specifying a template

3. ✅ **Backward Compatibility:** Existing configurations with explicit template settings continue to work

4. ✅ **CLI Override:** `--template` flag overrides both default and settings values

5. ✅ **Documentation:** All documentation reflects the new default behavior

6. ✅ **Unit Tests:** Tests verify directory mode, explicit dot, and multi-file components

7. ✅ **Integration Tests:** CLI integration tests verify end-to-end behavior

## Test Results

### Unit Tests

```text
=== RUN   TestExecutePacker_DirectoryMode
=== RUN   TestExecutePacker_DirectoryMode/directory_mode_with_no_template_specified
=== RUN   TestExecutePacker_DirectoryMode/directory_mode_with_explicit_dot_template
--- PASS: TestExecutePacker_DirectoryMode (1.27s)
=== RUN   TestExecutePacker_MultiFileComponent
--- PASS: TestExecutePacker_MultiFileComponent (0.71s)
PASS
```

### Integration Tests

Run with: `go test ./tests -run 'TestCLICommands/packer' -v`

## Out of Scope

- Automatic detection of whether component should use single-file or directory mode
- Support for `.pkr.json` files (Packer supports these, but Atmos doesn't need special handling)
- Glob patterns for selective file loading (Packer doesn't support this natively)

## References

- [GitHub Issue #1937](https://github.com/cloudposse/atmos/issues/1937)
- [Packer HCL Templates Documentation](https://developer.hashicorp.com/packer/docs/templates/hcl_templates)
- [Packer Build Command](https://developer.hashicorp.com/packer/docs/commands/build)
