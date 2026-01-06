# PRD: Stack Name Identity

## Status

Implemented.

## Problem Statement

### Current Behavior (Incorrect)

When resolving which stack a user is referring to via the `-s` argument, Atmos currently accepts multiple identifiers for the same stack:

1. The explicit `name` field from the stack manifest
2. The generated name from `name_template` or `name_pattern` in `atmos.yaml`
3. The stack filename

For example, given:
- Stack file: `legacy-prod.yaml`
- Manifest contains: `name: my-explicit-stack`
- `atmos.yaml` has: `name_template: "{{ .vars.environment }}-{{ .vars.stage }}"`
- Stack vars produce: `prod-ue1`

Currently, ALL of the following commands work:
```bash
atmos tf plan vpc -s my-explicit-stack  # Works (explicit name)
atmos tf plan vpc -s prod-ue1           # Works (template name) - SHOULD FAIL
atmos tf plan vpc -s legacy-prod        # Works (filename) - SHOULD FAIL
```

### Why This Is Wrong

1. **Ambiguity**: Multiple valid names creates confusion about the "canonical" stack name
2. **Inconsistency**: `atmos list stacks` shows one name, but other names still work
3. **Error-prone**: Users might use deprecated/incorrect names without realizing it
4. **Principle of least surprise**: When you explicitly name something, that should BE its name

## Proposed Solution

### Single Identity Rule

A stack has exactly ONE valid identifier, determined by precedence:

| Priority | Source | When Used |
|----------|--------|-----------|
| 1 | `name` field in manifest | If set, ONLY this name is valid |
| 2 | `name_template` result | If template is set (and no explicit name), ONLY template result is valid |
| 3 | `name_pattern` result | If pattern is set (and no template/name), ONLY pattern result is valid |
| 4 | Filename | If nothing else configured, filename is the identity |

**Key principle**: There is no fallback. Each stack has exactly ONE valid identifier.

### Expected Behavior

Given the example above:
```bash
atmos tf plan vpc -s my-explicit-stack  # Works (explicit name)
atmos tf plan vpc -s prod-ue1           # FAILS - not the canonical name
atmos tf plan vpc -s legacy-prod        # FAILS - not the canonical name
```

Error message for invalid name:
```
Could not find the component 'vpc' in the stack 'prod-ue1'.

Did you mean 'my-explicit-stack'? The stack file 'legacy-prod.yaml'
defines an explicit name 'my-explicit-stack'.
```

### Scenarios

#### Scenario 1: Explicit Name Set
```yaml
# stacks/legacy-prod.yaml
name: my-explicit-stack
vars:
  environment: prod
  stage: ue1
```

With `name_template: "{{ .vars.environment }}-{{ .vars.stage }}"`:
- Valid: `my-explicit-stack`
- Invalid: `prod-ue1`, `legacy-prod`

#### Scenario 2: No Explicit Name, Template Set
```yaml
# stacks/legacy-prod.yaml
vars:
  environment: prod
  stage: ue1
```

With `name_template: "{{ .vars.environment }}-{{ .vars.stage }}"`:
- Valid: `prod-ue1`
- Invalid: `legacy-prod`

#### Scenario 3: No Name, No Template, Pattern Set
```yaml
# stacks/deploy/prod/us-east-1.yaml
vars:
  environment: prod
  stage: ue1
```

With `name_pattern: "{environment}-{stage}"`:
- Valid: `prod-ue1`
- Invalid: `deploy/prod/us-east-1`

#### Scenario 4: No Naming Configuration
```yaml
# stacks/prod-us-east-1.yaml
vars:
  environment: prod
```

With no `name`, `name_template`, or `name_pattern`:
- Valid: `prod-us-east-1`

## Implementation

### Code Changes

**File: `internal/exec/utils.go`**

Update `findComponentInStacks()` to use exclusive matching:

```go
// Current (incorrect - OR logic):
stackMatches := configAndStacksInfo.Stack == configAndStacksInfo.ContextPrefix ||
    (stackManifestName != "" && configAndStacksInfo.Stack == stackManifestName) ||
    configAndStacksInfo.Stack == stackName

// New (correct - single identity):
var validStackName string
switch {
case stackManifestName != "":
    // Priority 1: Explicit name from manifest
    validStackName = stackManifestName
case configAndStacksInfo.ContextPrefix != "" && configAndStacksInfo.ContextPrefix != stackName:
    // Priority 2/3: Generated from name_template or name_pattern
    validStackName = configAndStacksInfo.ContextPrefix
default:
    // Priority 4: Filename
    validStackName = stackName
}
stackMatches := configAndStacksInfo.Stack == validStackName
```

### Error Message Enhancement

When a stack is not found, provide helpful suggestions:

```go
// If user's stack name matches a filename but not the canonical name
if userStack == stackFilename && canonicalName != stackFilename {
    return fmt.Errorf("stack '%s' not found. Did you mean '%s'? "+
        "The stack file '%s' has canonical name '%s'",
        userStack, canonicalName, stackFilename, canonicalName)
}
```

## Backwards Compatibility

### Breaking Change

This IS a breaking change for users who:
1. Have `name` field set but use the generated name or filename
2. Have `name_template`/`name_pattern` set but use the filename

### Migration Path

Users will receive clear error messages indicating the correct name to use.

### Detection

`atmos validate stacks` could warn about non-canonical names in CI/scripts.

## Testing

### Unit Tests

1. `TestProcessStacks_RejectsGeneratedNameWhenExplicitNameSet`
   - Stack has `name: explicit`, template produces `generated`
   - Using `-s generated` should FAIL
   - Using `-s explicit` should PASS

2. `TestProcessStacks_RejectsFilenameWhenExplicitNameSet`
   - Stack has `name: explicit`, filename is `legacy-prod`
   - Using `-s legacy-prod` should FAIL
   - Using `-s explicit` should PASS

3. `TestProcessStacks_RejectsFilenameWhenTemplateSet`
   - Stack has no `name`, template produces `generated`, filename is `legacy-prod`
   - Using `-s legacy-prod` should FAIL
   - Using `-s generated` should PASS

4. `TestProcessStacks_AcceptsFilenameWhenNoNamingConfigured`
   - Stack has no `name`, no template, no pattern
   - Using `-s filename` should PASS

## Success Criteria

1. Each stack has exactly ONE valid identifier
2. Using any other identifier fails with helpful error message
3. `atmos list stacks` output matches the only valid identifier
4. All terraform/helmfile/describe commands respect this rule

## References

- Related PR: #1934 (fix: honor stack manifest 'name' field in ProcessStacks)
- Related Issue: #1932
