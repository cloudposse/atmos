# PRD: Devcontainer Naming Convention

## Problem Statement

The original devcontainer naming scheme used hyphens as separators between all components:

```
atmos-devcontainer-{name}-{instance}
```

This creates a **parsing ambiguity** when both the devcontainer name and instance name contain hyphens. The parsing logic (split by hyphen, take last part as instance) cannot distinguish between:

- `atmos-devcontainer-my-app-test-1`
  - Could be: name=`my-app`, instance=`test-1` ✓ (intended)
  - Could be: name=`my-app-test`, instance=`1` ✗ (incorrectly parsed)

### Real-World Impact

This ambiguity led to bugs in the `GenerateNextInstance` functionality:

```go
// Test case that exposed the bug:
containers: ["atmos-devcontainer-test-dev-1", "atmos-devcontainer-test-dev-2"]
baseInstance: "dev"
expected: "dev-3" (next sequential instance)
actual: "dev-1" (failed to parse existing instances, thought none existed)
```

The parsing logic couldn't correctly extract instance numbers when the base instance name contained hyphens, causing instance name collisions and preventing proper sequential instance generation.

### Why This Matters

1. **User Experience**: Users naturally use hyphens in names (`backend-api`, `test-env`, `prod-db`)
2. **Instance Naming**: Instance names like `test-1`, `staging-2` are intuitive
3. **Automation**: CI/CD systems generate sequential instances and need reliable parsing
4. **Data Integrity**: Container naming collisions can cause data loss or confusion

## Investigation

### Docker/Podman Container Name Constraints

Docker and Podman container names support the character set: `[a-zA-Z0-9][a-zA-Z0-9_.-]*`

**Allowed characters:**
- Alphanumeric: `a-z`, `A-Z`, `0-9`
- Special: hyphen (`-`), underscore (`_`), dot (`.`)
- Must start with alphanumeric
- Maximum 63 characters (for DNS compatibility)

### Evaluated Solutions

#### Option 1: Dot (`.`) as Primary Separator ✅ SELECTED

**Format:** `atmos-devcontainer.{name}.{instance}`

**Pros:**
- Completely unambiguous: Split by `.` yields exactly 3 parts
- Visually distinct from internal hyphens
- Common convention in service naming (e.g., `app.env.instance`)
- DNS-safe and widely supported
- Allows arbitrary hyphens in both name and instance

**Cons:**
- Breaking change from hyphen-only naming
- Requires migration strategy for existing containers

**Examples:**
```
atmos-devcontainer.my-app.default
atmos-devcontainer.backend-api.test-1
atmos-devcontainer.worker-queue.staging-2
```

**Parsing logic:**
```go
remainder := strings.TrimPrefix(containerName, "atmos-devcontainer.")
parts := strings.SplitN(remainder, ".", 2)
name := parts[0]      // Can contain hyphens!
instance := parts[1]  // Can contain hyphens!
```

#### Option 2: Double Hyphen (`--`) as Separator

**Format:** `atmos-devcontainer--{name}--{instance}`

**Pros:**
- Unambiguous (single vs double hyphens)
- No new character types

**Cons:**
- Less visually clear than dots
- Unusual convention
- Harder to read: `my-app--test--1` vs `my-app.test.1`

**Rejected:** Less intuitive than dot notation

#### Option 3: Underscore (`_`) as Primary Separator

**Format:** `atmos-devcontainer_{name}_{instance}`

**Pros:**
- Unambiguous parsing
- Valid in Docker names

**Cons:**
- Users might mix underscores and hyphens in names
- Less conventional than dot notation
- Harder to read: `backend_api_test_1` vs `backend-api.test-1`

**Rejected:** Dot notation is more conventional and readable

## Solution: Dot Separator with Label-Based Identification

### Implementation

#### 1. Primary: Container Labels (Robust)

Every new devcontainer gets labels that explicitly store the parsed name and instance:

```go
Labels: {
    "com.atmos.type": "devcontainer",
    "com.atmos.devcontainer.name": "my-app",
    "com.atmos.devcontainer.instance": "test-1",
    "com.atmos.workspace": "/path/to/workspace",
    "com.atmos.created": "2025-11-10T14:00:00Z"
}
```

**Benefits:**
- **No parsing needed**: Direct label lookup
- **Future-proof**: Works regardless of naming scheme changes
- **Backward compatible**: Can identify old containers via labels
- **Metadata rich**: Can store additional context

#### 2. Fallback: Name Parsing (Migration Support)

For containers without labels (legacy or manually created), parse the name:

```go
func ParseContainerName(containerName string) (name, instance string) {
    // Try new dot format first
    if strings.HasPrefix(containerName, "atmos-devcontainer.") {
        remainder := strings.TrimPrefix(containerName, "atmos-devcontainer.")
        parts := strings.SplitN(remainder, ".", 2)
        if len(parts) == 2 {
            return parts[0], parts[1]
        }
    }

    // Fallback to old hyphen format (best-effort)
    if strings.HasPrefix(containerName, "atmos-devcontainer-") {
        remainder := strings.TrimPrefix(containerName, "atmos-devcontainer-")
        parts := strings.Split(remainder, "-")
        if len(parts) >= 2 {
            instance = parts[len(parts)-1]
            name = strings.Join(parts[:len(parts)-1], "-")
            return name, instance
        }
    }

    return "", ""
}
```

### Migration Strategy

#### Phase 1: Dual Support (Current Release)
- New containers use dot separator + labels
- Old containers still work via fallback parsing
- All lifecycle operations check labels first, fall back to parsing

#### Phase 2: Deprecation Warning (Next Release)
- Log warnings when operating on containers without labels
- Provide `atmos devcontainer migrate` command to relabel containers

#### Phase 3: Labels Required (Future Release)
- Remove fallback parsing
- Require all containers to have proper labels

### Updated Naming Convention

**Format:** `atmos-devcontainer.{name}.{instance}`

**Rules:**
1. **Prefix:** Always `atmos-devcontainer`
2. **Separator:** Dot (`.`) between prefix, name, and instance
3. **Name:** Alphanumeric, hyphens, underscores; must start with alphanumeric
4. **Instance:** Same rules as name; defaults to `default` if not specified
5. **Total length:** Maximum 63 characters (DNS-safe)

**Examples:**
```bash
# Simple names
atmos-devcontainer.myapp.default
atmos-devcontainer.api.alice

# Hyphenated names (now unambiguous!)
atmos-devcontainer.backend-api.default
atmos-devcontainer.worker-queue.prod-1

# Hyphenated instance names
atmos-devcontainer.frontend.test-1
atmos-devcontainer.database.staging-2
```

## Implementation Details

### Files Modified

1. **`pkg/devcontainer/naming.go`**
   - Update `GenerateContainerName` to use dot separator
   - Update `ParseContainerName` to handle both dot and hyphen formats
   - Update package documentation

2. **`pkg/devcontainer/naming_test.go`**
   - Update all test cases for dot separator
   - Add tests for parsing ambiguity resolution

3. **`pkg/devcontainer/lifecycle_instance_test.go`**
   - **Remove "known bug" test cases** (lines 44-87, 212-248)
   - Add correct test cases for instance generation with hyphenated names
   - Test that `GenerateNextInstance` correctly identifies existing instances

4. **All lifecycle operation tests**
   - Update container name references from hyphens to dots
   - Ensure tests use correct naming convention

### Container Creation

All new containers will include labels:

```go
config.Labels = map[string]string{
    LabelType:                  "devcontainer",
    LabelDevcontainerName:      name,
    LabelDevcontainerInstance:  instance,
    LabelWorkspace:             workspacePath,
    LabelCreated:               time.Now().Format(time.RFC3339),
}
```

### Container Discovery

Container lookup now prioritizes labels:

```go
func findContainer(name, instance string) (*container.Info, error) {
    // Primary: Find by labels
    containers, err := runtime.List(ctx, map[string]string{
        LabelDevcontainerName:     name,
        LabelDevcontainerInstance: instance,
    })
    if len(containers) > 0 {
        return &containers[0], nil
    }

    // Fallback: Find by name pattern (for migration)
    expectedName := GenerateContainerName(name, instance)
    containers, err = runtime.List(ctx, nil)
    for _, c := range containers {
        if c.Name == expectedName {
            return &c, nil
        }
    }

    return nil, ErrContainerNotFound
}
```

## Testing Strategy

### Unit Tests

1. **Naming Tests** (`naming_test.go`)
   - Generate names with hyphens in both name and instance
   - Parse dot-separated names correctly
   - Parse legacy hyphen-separated names (fallback)
   - Validate character constraints

2. **Instance Tests** (`lifecycle_instance_test.go`)
   - Generate next instance with hyphenated base names
   - Handle existing instances with hyphens correctly
   - Test sequential instance generation (dev-1, dev-2, dev-3)

3. **Lifecycle Tests**
   - All operations work with dot-separated names
   - Label-based identification works correctly

### Integration Tests

1. Create containers with hyphenated names and instances
2. Verify they can be listed, stopped, removed
3. Test sequential instance generation
4. Verify backward compatibility with legacy containers

## Success Criteria

✅ **Unambiguous Parsing**: Any combination of hyphens in name/instance parses correctly
✅ **No Collisions**: Sequential instance generation never conflicts
✅ **Backward Compatible**: Old containers still discoverable via fallback
✅ **Clean Syntax**: Intuitive naming that users expect
✅ **Tests Pass**: All existing and new tests validate correct behavior
✅ **Documentation**: Clear examples and migration guide

## Future Enhancements

1. **`atmos devcontainer migrate`**: Relabel old containers with proper labels
2. **`atmos devcontainer doctor`**: Check for containers without proper labels
3. **Deprecation Timeline**: Communicate sunset of hyphen-only support

## References

- Docker container naming: `[a-zA-Z0-9][a-zA-Z0-9_.-]*`
- DNS-safe names: Maximum 63 characters
- Container labels: Docker/Podman metadata API
- Issue: CodeRabbit review identified tests validating incorrect behavior
