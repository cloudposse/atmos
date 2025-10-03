# Atmos Schema Validation Documentation

## Overview

This document describes the Atmos schema validation system and how it handles YAML functions and dynamic fields.

## Schema Types

Atmos uses several JSON schemas to validate different configuration files:

### 1. CLI Configuration Schema (`cli.json`)
- **File Pattern**: `atmos.yaml`, `atmos.yml`, `.atmos.yaml`, `.atmos.yml`
- **Location**: `website/static/schemas/atmos/1.0/cli.json`
- **Purpose**: Validates Atmos CLI configuration files

### 2. Stack Manifest Schema (`stack.json`)
- **File Pattern**: `stacks/**/*.yaml`
- **Location**: `website/static/schemas/atmos/1.0/stack.json`
- **Purpose**: Validates stack configuration files

### 3. Vendor Configuration Schema (`vendor.json`)
- **File Pattern**: `vendor.yaml`, `vendor.yml`
- **Location**: `website/static/schemas/atmos/1.0/vendor.json`
- **Purpose**: Validates vendor configuration files

### 4. Workflow Schema (`workflow.json`)
- **File Pattern**: `workflows/**/*.yaml`
- **Location**: `website/static/schemas/atmos/1.0/workflow.json`
- **Purpose**: Validates workflow definitions

## YAML Functions Support

Atmos supports the following YAML functions that can appear in configuration files:

| Function | Description | Example |
|----------|-------------|---------|
| `!include` | Include another YAML file | `!include path/to/file.yaml` |
| `!include.raw` | Include file as raw string | `!include.raw path/to/file.txt` |
| `!env` | Get environment variable | `!env MY_VAR` |
| `!exec` | Execute command | `!exec echo hello` |
| `!template` | Process Go template | `!template '{{ .vars.name }}'` |
| `!terraform.output` | Get Terraform output | `!terraform.output vpc dev vpc_id` |
| `!terraform.state` | Query Terraform state | `!terraform.state vpc dev outputs.vpc_id` |
| `!store` | Get value from store | `!store redis dev app key` |
| `!store.get` | Alternative store syntax | `!store.get aws-ssm /path/to/param` |
| `!repo-root` | Get repository root path | `!repo-root` |

## Field Categories

### Static Fields (Strictly Typed)
These fields do NOT accept YAML functions and have strict type requirements:

#### CLI Config (`atmos.yaml`)
- `base_path` - Must be a string path
- `components.terraform.command` - Must be a string command
- `components.helmfile.command` - Must be a string command
- `components.packer.command` - Must be a string command
- `logs.level` - Must be one of: Trace, Debug, Info, Warning, Off
- `settings.list_merge_strategy` - Must be one of: replace, append, merge
- `commands[].name` - Must be a string (required)

#### Stack Manifests
- Top-level structure fields (`terraform`, `helmfile`, `components`)
- Component metadata fields (`metadata.component`, `metadata.inherit`)
- Backend configuration structure

### Dynamic Fields (Accept Any Type)
These fields can accept YAML functions and therefore accept any type:

#### CLI Config (`atmos.yaml`)
- Any fields under `stores.*` - External store configurations
- Any fields under `templates.settings.gomplate.datasources`
- Custom fields added via `additionalProperties: true`

#### Stack Manifests
- `vars.*` - All variable fields (most common use of YAML functions)
- `env.*` - All environment variable fields
- `settings.*` - Settings can use templating
- Component-specific configuration under `components.terraform.<name>.vars`
- Component-specific environment under `components.terraform.<name>.env`

## Schema Flexibility

To support YAML functions and maintain flexibility, the schemas use:

1. **`additionalProperties: true`** - Allows fields not explicitly defined in the schema
2. **Removed strict `required` fields** - Only essential fields are required
3. **Union types for dynamic fields** - Fields that might have functions accept multiple types

## Validation Approach

### Two-Phase Validation

1. **Structure Validation (Schema)**
   - Validates YAML structure and field names
   - Ignores YAML function tags during parsing
   - Checks required fields exist
   - Does NOT execute functions

2. **Runtime Validation (Execution)**
   - Executes YAML functions
   - Validates resolved values
   - Checks type compatibility after resolution
   - Handles errors from function execution

### Test Files Exclusion

Some files intentionally have invalid configurations for testing purposes. These are excluded from validation:

#### Excluded Patterns
- `test-cases/validate-type-mismatch/**`
- `fixtures/scenarios/invalid-*/**`
- `fixtures/scenarios/broken-*/**`
- `fixtures/scenarios/*-negative/**`

## Validation Script

Use the `validate-all-schemas.sh` script to validate all schemas:

```bash
./validate-all-schemas.sh
```

The script will:
1. Find all configuration files
2. Skip test files with known issues
3. Validate YAML syntax
4. Report results with color-coded output

### Output Format
- ✅ **PASS**: Valid configuration
- ❌ **FAIL**: Invalid configuration
- ⏭️ **SKIP**: Test file with deliberate issues

## Best Practices

1. **Use YAML functions sparingly** - Only where dynamic values are needed
2. **Document function usage** - Comment complex function uses
3. **Test function resolution** - Verify functions resolve to expected types
4. **Keep static fields static** - Don't use functions for structural fields
5. **Validate early and often** - Run validation in CI/CD pipelines

## Common Issues and Solutions

### Issue: Field expects string but gets object from function
**Solution**: Ensure the YAML function returns the expected type. For example, `!terraform.output` might return an object that needs specific field access.

### Issue: Schema validation fails on valid config
**Solution**: Check if the field needs to be added to dynamic fields list or if `additionalProperties: true` needs to be set.

### Issue: Function not resolving during validation
**Solution**: Schema validation doesn't execute functions. This is expected behavior. Functions are resolved at runtime.

## Future Improvements

1. **Type hints for functions** - Add metadata to indicate expected return types
2. **Function validation** - Validate function syntax without execution
3. **Schema generation** - Auto-generate schemas from Go structs
4. **Visual schema explorer** - Interactive tool to explore schema structure
5. **Better error messages** - More helpful validation error descriptions
