---
title: Debugging & Troubleshooting
sidebar_position: 4
sidebar_label: Debugging
---

import Terminal from '@site/src/components/Terminal'

This guide covers debugging and troubleshooting techniques for Atmos, including using verbose mode, logging levels, and understanding error messages.

## Verbose Error Mode

The `--verbose` flag enables detailed error output with full context and stack traces, making it easier to debug issues.

### When to Use Verbose Mode

Use `--verbose` when:
- Debugging unexpected errors
- Understanding the context of a failure
- Reporting issues to the Atmos team
- Troubleshooting configuration problems
- Investigating component or stack issues

### Enabling Verbose Mode

**CLI Flag:**
```shell
atmos terraform plan vpc -s prod --verbose
```

**Environment Variable:**
```shell
export ATMOS_VERBOSE=true
atmos terraform plan vpc -s prod
```

**Configuration File:**
```yaml
# atmos.yaml
errors:
  format:
    verbose: true
```

### What Verbose Mode Shows

When verbose mode is enabled, errors include:

1. **Context Tables** - Structured information about where the error occurred:
   ```
   ┏━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━┓
   ┃ Context   ┃ Value                 ┃
   ┣━━━━━━━━━━━╋━━━━━━━━━━━━━━━━━━━━━━━┫
   ┃ component ┃ vpc                   ┃
   ┃ stack     ┃ prod                  ┃
   ┃ region    ┃ us-east-1             ┃
   ┗━━━━━━━━━━━┻━━━━━━━━━━━━━━━━━━━━━━━┛
   ```

2. **Full Error Chains** - Complete stack traces showing the error path

3. **Detailed Hints** - Actionable suggestions with markdown formatting

4. **Additional Context** - Extra debugging information that's hidden in normal mode

### Example: Normal vs Verbose Output

**Normal Mode:**
```shell
$ atmos terraform plan nonexistent -s prod
Error: invalid component: 'nonexistent' points to the Terraform component 'nonexistent', but it does not exist

💡 Use `atmos list components` to see available components
💡 Verify the component path: /path/to/components/terraform
```

**Verbose Mode:**
```shell
$ atmos terraform plan nonexistent -s prod --verbose
Error: invalid component: 'nonexistent' points to the Terraform component 'nonexistent', but it does not exist

┏━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ Context              ┃ Value                           ┃
┣━━━━━━━━━━━━━━━━━━━━━━╋━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫
┃ component            ┃ nonexistent                     ┃
┃ terraform_component  ┃ nonexistent                     ┃
┃ stack                ┃ prod                            ┃
┃ base_path            ┃ /path/to/components/terraform   ┃
┗━━━━━━━━━━━━━━━━━━━━━━┻━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

💡 Use `atmos list components` to see available components
💡 Verify the component path: /path/to/components/terraform

Stack trace:
  github.com/cloudposse/atmos/internal/exec.(*ComponentInfo).Process
      /path/to/atmos/internal/exec/component.go:123
  ... (additional stack frames)
```

## Logging Levels

Atmos has separate **logging levels** that control system log output (not error output). Use these to understand what Atmos is doing internally.

### Available Log Levels

| Level   | Description                                  | Use Case                          |
|---------|----------------------------------------------|-----------------------------------|
| Error   | Only error logs                              | Production (default)              |
| Warn    | Warnings and errors                          | Production monitoring             |
| Info    | Informational messages, warnings, and errors | General troubleshooting           |
| Debug   | Detailed debugging information               | Development and deep debugging    |
| Trace   | Extremely detailed trace information         | Low-level debugging               |

### Setting Log Levels

**CLI Flag:**
```shell
atmos terraform plan vpc -s prod --logs-level Debug
```

**Environment Variable:**
```shell
export ATMOS_LOGS_LEVEL=Debug
atmos terraform plan vpc -s prod
```

**Configuration File:**
```yaml
# atmos.yaml
logs:
  level: Debug
```

### Verbose vs Logging: What's the Difference?

It's important to understand the distinction:

| Feature | Purpose | Controls | Use Case |
|---------|---------|----------|----------|
| `--verbose` | **Error verbosity** | Context tables, stack traces | Debugging errors |
| `--logs-level` | **Logging verbosity** | System log messages | Understanding execution flow |

**Example combining both:**
```shell
# Show detailed errors AND detailed logs
atmos terraform plan vpc -s prod --verbose --logs-level Debug
```

## Common Debugging Scenarios

### Scenario 1: Component Not Found

**Error:**
```
Error: invalid component: component not found in stack
```

**Debug steps:**
1. List available components:
   ```shell
   atmos list components
   ```

2. Check component configuration:
   ```shell
   atmos describe component <component> -s <stack> --verbose
   ```

3. Verify component path in `atmos.yaml`:
   ```yaml
   components:
     terraform:
       base_path: "components/terraform"
   ```

### Scenario 2: Stack Configuration Issues

**Error:**
```
Error: invalid stack: no config found for component
```

**Debug steps:**
1. List available stacks:
   ```shell
   atmos list stacks
   ```

2. Inspect stack configuration with verbose mode:
   ```shell
   atmos describe stacks --verbose
   ```

3. Check for typos in stack files and imports

### Scenario 3: Validation Failures

**Error:**
```
Error: validation failed: JSON schema validation errors
```

**Debug steps:**
1. Run validation with verbose output:
   ```shell
   atmos validate component <component> -s <stack> --verbose
   ```

2. Review the schema file mentioned in the error context table

3. Check the specific validation errors listed

4. Verify your configuration matches the schema requirements

### Scenario 4: Template Errors

**Error:**
```
Error: invalid template settings
```

**Debug steps:**
1. Check template delimiters in `atmos.yaml`:
   ```yaml
   templates:
     settings:
       delimiters: ["{{", "}}"]  # Must be exactly 2 non-empty strings
   ```

2. Run with verbose mode to see the exact delimiter values:
   ```shell
   atmos <command> --verbose
   ```

3. Verify template syntax in your stack files

### Scenario 5: Workflow Issues

**Error:**
```
Error: workflow file not found
```

**Debug steps:**
1. List available workflows:
   ```shell
   atmos list workflows
   ```

2. Check workflow directory in `atmos.yaml`:
   ```yaml
   workflows:
     base_path: "stacks/workflows"
   ```

3. Verify workflow file exists and has correct YAML syntax

## Debugging with Sentry Integration

If your organization uses Sentry for error tracking, Atmos can send error reports automatically.

### Enable Sentry in Configuration

```yaml
# atmos.yaml
errors:
  sentry:
    enabled: true
    dsn: "https://examplePublicKey@o0.ingest.sentry.io/0"
    environment: "production"
    sample_rate: 1.0
    capture_stack_context: true
    tags:
      team: "platform"
```

### What Gets Sent to Sentry

- **Error message** and error chain
- **Hints** as breadcrumbs
- **Context** (component, stack, region) as tags with `atmos.` prefix
- **Stack traces** when `capture_stack_context: true`
- **Exit codes** as `atmos.exit_code` tag

:::note Privacy
Only PII-safe context is sent to Sentry. Sensitive values are automatically redacted.
:::

## Performance Profiling

For performance troubleshooting, see [Profiling](/troubleshoot/profiling).

## Getting Help

When reporting issues:

1. **Run with verbose mode** and include the full output:
   ```shell
   atmos <command> --verbose 2>&1 | tee debug.log
   ```

2. **Include context** from error tables (component, stack, etc.)

3. **Share configuration** (redact sensitive values):
   - `atmos.yaml` (relevant sections)
   - Stack files (relevant sections)
   - Component configurations

4. **Provide version information**:
   ```shell
   atmos version
   ```

5. **Report in GitHub Issues**: https://github.com/cloudposse/atmos/issues

## Related Documentation

- [Global Flags](/cli/global-flags) - Complete list of CLI flags
- [Error Handling](/cli/errors) - Understanding error messages
- [Configuration](/cli/configuration) - Atmos configuration reference
- [Profiling](/troubleshoot/profiling) - Performance troubleshooting
