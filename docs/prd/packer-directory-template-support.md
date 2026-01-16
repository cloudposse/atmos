# PRD: Packer Directory-Based Template Support

## Problem Statement

**GitHub Issue:** [#1937](https://github.com/cloudposse/atmos/issues/1937)

Currently, Atmos requires users to specify a single HCL file via `--template` flag or `settings.packer.template`
configuration. This prevents users from organizing Packer components using the standard multi-file pattern where:

- `variables.pkr.hcl` - Variable declarations
- `template.pkr.hcl` or `main.pkr.hcl` - Source and build blocks
- `locals.pkr.hcl` - Local values
- `plugins.pkr.hcl` - Required plugins

When users split their Packer configuration across multiple files (a recommended practice), Atmos only loads the
specified template file, causing "Unsupported attribute" errors when variables are defined in separate files.

### Current Behavior

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

This results in: `packer build -var-file vars.json template.pkr.hcl`

### Packer's Native Behavior

From [HashiCorp Packer documentation](https://developer.hashicorp.com/packer/docs/templates/hcl_templates):

> "When a directory is passed, all files in the folder with a name ending with `.pkr.hcl` or `.pkr.json` will be parsed
> using the HCL2 format."

Native Packer supports:

- `packer build .` - Load all `*.pkr.hcl` files from current directory
- `packer build ./my-component/` - Load all `*.pkr.hcl` files from specified directory
- `packer build template.pkr.hcl` - Load single file (current Atmos behavior)

## Solution

Make the `template` parameter optional with a default of `"."` (current directory), allowing Packer to load all
`*.pkr.hcl` files from the component directory.

### Behavior Changes

| Scenario                   | Current                              | Proposed                                 |
|----------------------------|--------------------------------------|------------------------------------------|
| No template specified      | Error: `packer template is required` | Uses `"."` - loads all `*.pkr.hcl` files |
| `template: "."`            | Error (if not specified)             | Passes `"."` to Packer                   |
| `template: "main.pkr.hcl"` | Works                                | Works (no change)                        |
| `--template .` flag        | Not commonly used                    | Passes `"."` to Packer                   |

## Implementation Plan

### Phase 1: Core Changes

#### 1.1 Remove Required Template Validation

**File:** `internal/exec/packer.go`

```go
// BEFORE (lines 194-196):
if template == "" {
return errUtils.ErrMissingPackerTemplate
}

// AFTER:
if template == "" {
template = "." // Default to current directory (component path)
}
```

#### 1.2 Update Error Definition (Optional Cleanup)

**File:** `errors/errors.go`

Consider deprecating or removing `ErrMissingPackerTemplate` since template will always have a default value.
Alternatively, keep it for cases where explicit validation is needed.

#### 1.3 Update Logging

**File:** `internal/exec/packer.go`

Update the debug log to clarify when using directory mode:

```go
log.Debug("Packer context",
"executable", info.Command,
"command", info.SubCommand,
"atmos component", info.ComponentFromArg,
"atmos stack", info.StackFromArg,
"packer component", info.BaseComponentPath,
"packer template", template,
"template mode", getTemplateMode(template), // "directory" or "file"
"working directory", workingDir,
"inheritance", inheritance,
"arguments and flags", info.AdditionalArgsAndFlags,
)
```

### Phase 2: Configuration Schema Updates

#### 2.1 Update Schema Documentation

**File:** `pkg/datafetcher/schema/atmos-manifest/settings.json`

Update the JSON schema to document the new default behavior:

```json
{
  "packer": {
    "type": "object",
    "properties": {
      "template": {
        "type": "string",
        "description": "Packer template file or directory. Defaults to '.' (current directory) which loads all *.pkr.hcl files.",
        "default": "."
      }
    }
  }
}
```

#### 2.2 Update atmos.yaml Schema

**File:** `pkg/datafetcher/schema/atmos.yaml/packer.json` (if exists)

Ensure the configuration schema reflects the optional nature of the template field.

### Phase 3: Documentation Updates

#### 3.1 Update Packer Component Documentation

**File:** `website/docs/stacks/components/packer.mdx`

Add section explaining directory-based templates:

```markdown
## Template Configuration

Atmos supports two ways to specify Packer templates:

### Directory Mode (Recommended)

By default, Atmos passes the component directory to Packer, which loads all `*.pkr.hcl` files:

```yaml
components:
  packer:
    my-ami:
      # No template specified - uses "." (loads all *.pkr.hcl files)
      vars:
        region: us-east-1
```

This allows you to organize your Packer component with multiple files:

```
components/packer/my-ami/
├── variables.pkr.hcl    # Variable declarations
├── main.pkr.hcl         # Source and build blocks
├── locals.pkr.hcl       # Local values (optional)
└── plugins.pkr.hcl      # Required plugins (optional)
```

### Single File Mode

For simple components or legacy configurations, specify a single template file:

```yaml
components:
  packer:
    my-ami:
      settings:
        packer:
          template: main.pkr.hcl
      vars:
        region: us-east-1
```

```

#### 3.2 Update CLI Documentation

**File:** `website/docs/cli/commands/packer/usage.mdx`

Update the `--template` flag documentation:

```markdown
<dt>`--template` <em>(alias `-t`)</em><em>(optional)</em></dt>
<dd>
    Packer template file or directory path. Defaults to `.` (current directory),
    which tells Packer to load all `*.pkr.hcl` files from the component directory.

    - Use `.` or omit to load all HCL files (recommended for multi-file components)
    - Use a specific filename like `main.pkr.hcl` for single-file templates

    Can also be specified via `settings.packer.template` in the stack manifest.
    The command line flag takes precedence.
</dd>
```

#### 3.3 Update packer-build.mdx

**File:** `website/docs/cli/commands/packer/packer-build.mdx`

Add examples for directory mode:

```markdown
## Examples

### Directory Mode (Multi-File)

```shell
# Uses all *.pkr.hcl files in the component directory
atmos packer build aws/bastion --stack prod

# Explicit directory mode
atmos packer build aws/bastion --stack prod --template .
```

### Single File Mode

```shell
# Use specific template file
atmos packer build aws/bastion --stack prod --template main.pkr.hcl
```

```

### Phase 4: Test Updates

#### 4.1 Add Directory Mode Tests

**File:** `internal/exec/packer_test.go`

```go
func TestExecutePacker_DirectoryTemplate(t *testing.T) {
    t.Run("default to directory mode when no template specified", func(t *testing.T) {
        // Setup component with multiple .pkr.hcl files
        // Verify Packer is called with "." as the template argument
    })

    t.Run("explicit directory mode with dot", func(t *testing.T) {
        // Verify template="." works correctly
    })

    t.Run("single file mode still works", func(t *testing.T) {
        // Verify backward compatibility with explicit template filename
    })
}
```

#### 4.2 Add Test Fixtures

**Directory:** `tests/fixtures/scenarios/packer/components/packer/multi-file/`

Create a multi-file Packer component for testing:

```
multi-file/
├── variables.pkr.hcl
├── main.pkr.hcl
└── locals.pkr.hcl
```

#### 4.3 Update Existing Tests

Review and update existing tests that expect `ErrMissingPackerTemplate` to reflect the new default behavior.

### Phase 5: Backward Compatibility

#### 5.1 Ensure No Breaking Changes

The change is backward compatible:

- Existing configurations with `settings.packer.template` continue to work
- Existing `--template` flag usage continues to work
- Only behavior change: missing template now defaults to "." instead of error

#### 5.2 Migration Path

No migration required. Users who were previously receiving errors due to missing template configuration will now have
their components work automatically.

## File Changes Summary

| File                                                | Change Type | Description                                     |
|-----------------------------------------------------|-------------|-------------------------------------------------|
| `internal/exec/packer.go`                           | Modify      | Default template to "." instead of error        |
| `internal/exec/packer_test.go`                      | Add         | New tests for directory mode                    |
| `errors/errors.go`                                  | Optional    | Consider deprecating `ErrMissingPackerTemplate` |
| `website/docs/stacks/components/packer.mdx`         | Modify      | Document directory mode                         |
| `website/docs/cli/commands/packer/usage.mdx`        | Modify      | Update --template flag docs                     |
| `website/docs/cli/commands/packer/packer-build.mdx` | Modify      | Add directory mode examples                     |
| `tests/fixtures/scenarios/packer/`                  | Add         | Multi-file test component                       |

## Acceptance Criteria

1. **Default Behavior:** Running `atmos packer build component -s stack` without `--template` or
   `settings.packer.template` should work by defaulting to "."

2. **Multi-File Support:** A Packer component with separate `variables.pkr.hcl` and `main.pkr.hcl` files should build
   successfully without specifying a template

3. **Backward Compatibility:** Existing configurations with explicit template settings continue to work

4. **CLI Override:** `--template` flag should override both default and settings values

5. **Documentation:** All documentation reflects the new default behavior

## Out of Scope

- Automatic detection of whether component should use single-file or directory mode
- Support for `.pkr.json` files (Packer supports these, but Atmos doesn't need special handling)
- Glob patterns for selective file loading (Packer doesn't support this natively)

## References

- [GitHub Issue #1937](https://github.com/cloudposse/atmos/issues/1937)
- [Packer HCL Templates Documentation](https://developer.hashicorp.com/packer/docs/templates/hcl_templates)
- [Packer Build Command](https://developer.hashicorp.com/packer/docs/commands/build)
