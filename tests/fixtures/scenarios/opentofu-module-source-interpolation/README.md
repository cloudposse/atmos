# OpenTofu Module Source Interpolation Test Fixture

This test fixture reproduces and validates the fix for [issue #1753](https://github.com/cloudposse/atmos/issues/1753),
which addresses OpenTofu 1.8+ module source variable interpolation support in Atmos.

## Problem Statement

OpenTofu 1.8+ introduced the ability to use variable interpolation in module sources:

```hcl
module "example" {
  source = "${var.context.build.module_path}"
  # ...
}
```

However, Atmos's `terraform-config-inspect` validation phase rejected this syntax before any OpenTofu commands could
execute, preventing users from using this OpenTofu-specific feature.

## Solution

Atmos now automatically detects whether the configured command is OpenTofu (via `command: "tofu"` or any path
containing "tofu") and skips validation errors for known OpenTofu-specific features.

This is a zero-configuration solution:

- No `lenient: true` flags required
- No manual workarounds needed
- Just works when using OpenTofu

## Test Structure

### Configuration (`atmos.yaml`)

- Configures `command: "tofu"` to use OpenTofu
- Enables `init.pass_vars: true` to pass varfile to terraform init

### Stack Configuration (`stacks/test-stack.yaml`)

Defines nested variable structure:

```yaml
vars:
  context:
    build:
      module_path: "./modules/example"
      module_version: "v1.0.0"
  simple_var: "simple_value"
  another_var: "another_value"
```

### Component (`components/terraform/test-component/main.tf`)

Uses OpenTofu 1.8+ module source interpolation:

```hcl
variable "context" {
  type = object({
    build = object({
      module_path    = string
      module_version = string
    })
  })
}

module "themodule" {
  source     = "${var.context.build.module_path}"
  test_input = var.simple_var
}
```

### Module (`components/terraform/test-component/modules/example/main.tf`)

Simple test module that receives inputs:

```hcl
variable "test_input" {
  type = string
}

output "test_output" {
  value = "Module loaded successfully with input: ${var.test_input}"
}
```

## What Gets Tested

### Integration Test (`internal/exec/opentofu_module_source_interpolation_test.go`)

1. **Component Description**: Verifies `atmos describe component` works with OpenTofu module source interpolation
2. **Nested Variable Preservation**: Confirms nested `context.build.*` variables are preserved in varfile
3. **Auto-Detection**: Validates that OpenTofu is detected and validation is automatically skipped

### Unit Tests (`internal/exec/terraform_detection_test.go`)

1. **Fast Path Detection**: Basename contains "tofu"
2. **Slow Path Detection**: Execute version command
3. **Caching**: Results cached by command path
4. **Thread Safety**: Concurrent access to detection cache
5. **Pattern Matching**: Known OpenTofu feature detection
6. **Edge Cases**: Empty errors, long messages, whitespace

## Expected Behavior

When running:

```bash
atmos describe component test-component -s test
```

Should return component configuration with nested variables intact:

```yaml
vars:
  context:
    build:
      module_path: ./modules/example
      module_version: v1.0.0
  simple_var: simple_value
  another_var: another_value
```

No validation errors should occur, even though the component uses OpenTofu-specific module source interpolation.

## Root Cause Analysis

The issue was NOT related to varfile generation or PR #1639's performance optimizations. The actual problem was:

1. Atmos uses `terraform-config-inspect` library for validation
2. This library uses Terraform's HCL parser, which rejects OpenTofu-specific syntax
3. Validation happens before any terraform/tofu commands execute
4. Users with `command: "tofu"` couldn't use OpenTofu-specific features

## Detection Strategy

**Two-tier approach:**

1. **Fast Path**: Check if executable basename contains "tofu" (e.g., `/usr/bin/tofu`, `/opt/homebrew/bin/tofu`)
2. **Slow Path**: Execute `<command> version` and check output for "OpenTofu"
3. **Caching**: Results cached by command path to avoid repeated subprocess calls

**Pattern Matching:**

When OpenTofu is detected, skip validation errors matching known OpenTofu-specific patterns:

- `"Variables not allowed"` - Module source interpolation (OpenTofu 1.8+)

## References

- Issue: https://github.com/cloudposse/atmos/issues/1753
- OpenTofu 1.8 Release: https://opentofu.org/blog/opentofu-1-8-0/
- PRD: `docs/prd/opentofu-module-source-interpolation.md`
