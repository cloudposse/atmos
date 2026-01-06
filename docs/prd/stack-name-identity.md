# PRD: Stack Name Identity

## Status

Implemented.

## Overview

This PRD defines how Atmos resolves stack identity - the mechanism by which users reference stacks via the `-s` argument and how Atmos determines which stack configuration to use.

## Goals

1. **Single Identity**: Each stack has exactly ONE valid identifier
2. **Zero-Config**: Newcomers can use stack filenames without any naming configuration
3. **Explicit Override**: Advanced users can set a `name` field to control the identifier
4. **Predictability**: The identifier shown in `atmos list stacks` is the only valid identifier

## Non-Goals

- Automatic migration of existing configurations
- Support for aliases or multiple valid names per stack

## Stack Name Precedence

A stack's canonical identifier is determined by the following precedence (highest to lowest):

| Priority | Source | When Used |
|----------|--------|-----------|
| 1 | `name` field in manifest | If set, this is the only valid identifier |
| 2 | `name_template` result | If template is set (and no explicit name), template result is the identifier |
| 3 | `name_pattern` result | If pattern is set (and no template/name), pattern result is the identifier |
| 4 | Filename | If nothing else is configured, filename is the identifier |

**Key principle**: There is no fallback. Only ONE identifier is valid per stack.

## Examples

### Example 1: Explicit Name Takes Priority

```yaml
# stacks/legacy-prod.yaml
name: my-explicit-stack
vars:
  environment: prod
  stage: ue1
```

With `name_template: "{{ .vars.environment }}-{{ .vars.stage }}"` in atmos.yaml:
- **Valid**: `atmos tf plan vpc -s my-explicit-stack`
- **Invalid**: `atmos tf plan vpc -s prod-ue1` (template result ignored)
- **Invalid**: `atmos tf plan vpc -s legacy-prod` (filename ignored)

### Example 2: Template Takes Priority Over Filename

```yaml
# stacks/legacy-prod.yaml
vars:
  environment: prod
  stage: ue1
```

With `name_template: "{{ .vars.environment }}-{{ .vars.stage }}"`:
- **Valid**: `atmos tf plan vpc -s prod-ue1`
- **Invalid**: `atmos tf plan vpc -s legacy-prod`

### Example 3: Pattern Takes Priority Over Filename

```yaml
# stacks/deploy/prod/us-east-1.yaml
vars:
  environment: prod
  stage: ue1
```

With `name_pattern: "{environment}-{stage}"`:
- **Valid**: `atmos tf plan vpc -s prod-ue1`
- **Invalid**: `atmos tf plan vpc -s deploy/prod/us-east-1`

### Example 4: Zero-Config (Filename as Identifier)

```yaml
# stacks/prod.yaml
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
```

With no `name`, `name_template`, or `name_pattern` configured:
- **Valid**: `atmos tf plan vpc -s prod`

This enables newcomers to start using Atmos immediately without configuring any naming strategy.

## User Experience

### Successful Usage

```bash
$ atmos terraform plan vpc -s my-explicit-stack
# Works as expected
```

### Invalid Identifier

```bash
$ atmos terraform plan vpc -s legacy-prod
Error: Could not find the component 'vpc' in the stack 'legacy-prod'.
```

### Stack Listing

The `atmos list stacks` command shows only canonical identifiers:

```bash
$ atmos list stacks
my-explicit-stack    # Not 'legacy-prod' or 'prod-ue1'
prod-ue1             # From template, not filename
dev                  # Filename (no naming config)
```

## Implementation

### Stack Resolution Logic

In `internal/exec/utils.go`, the `findComponentInStacks` function determines the canonical name:

```go
var canonicalStackName string
switch {
case stackManifestName != "":
    // Priority 1: Explicit name from manifest
    canonicalStackName = stackManifestName
case configAndStacksInfo.ContextPrefix != "" && configAndStacksInfo.ContextPrefix != stackName:
    // Priority 2/3: Generated from name_template or name_pattern
    canonicalStackName = configAndStacksInfo.ContextPrefix
default:
    // Priority 4: Filename (zero-config)
    canonicalStackName = stackName
}
stackMatches := configAndStacksInfo.Stack == canonicalStackName
```

### Filename Fallback

In `internal/exec/utils.go`, the `processStackContextPrefix` function enables filename-based identity:

```go
switch {
case atmosConfig.Stacks.NameTemplate != "":
    // Process name_template
case atmosConfig.Stacks.NamePattern != "":
    // Process name_pattern
default:
    // No naming config - use filename as identity
    configAndStacksInfo.ContextPrefix = stackName
}
```

## Testing

The following test cases verify the single identity rule:

1. **`TestProcessStacks_RejectsGeneratedNameWhenExplicitNameSet`**: Using template-generated name fails when explicit `name` is set
2. **`TestProcessStacks_RejectsFilenameWhenExplicitNameSet`**: Using filename fails when explicit `name` is set
3. **`TestProcessStacks_RejectsFilenameWhenTemplateSet`**: Using filename fails when `name_template` is configured
4. **`TestProcessStacks_AcceptsFilenameWhenNoNamingConfigured`**: Using filename works when no naming is configured
5. **`TestDescribeStacks_FilenameAsKeyWhenNoNamingConfigured`**: DescribeStacks returns filename as key when no naming is configured

## Backwards Compatibility

This is a behavioral change for users who:
1. Have `name` field set but reference stacks by generated name or filename
2. Have `name_template`/`name_pattern` set but reference stacks by filename

Users will receive clear error messages indicating the stack was not found, prompting them to use the canonical identifier.

## Success Criteria

1. Each stack has exactly ONE valid identifier
2. Using any other identifier returns "stack not found"
3. `atmos list stacks` output matches the only valid identifier
4. All commands (terraform, helmfile, describe, etc.) respect this rule
5. Newcomers can use filenames without any naming configuration

## References

- PR: #1934
- Issue: #1932
