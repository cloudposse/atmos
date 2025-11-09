# PRD: Component Path Resolution for Terraform Commands

## Overview

Enable developers to run Terraform commands using relative filesystem paths (e.g., `atmos terraform plan .`) instead of explicit component names. The system will automatically resolve the current directory to the appropriate component name based on stack configurations.

## Problem Statement

Currently, developers must know the exact component name as defined in stack configurations to run Terraform commands:

```bash
# Current requirement - developer must know component name
cd components/terraform/vpc/security-group
atmos terraform plan vpc/security-group --stack dev
```

This creates friction because:
1. **Component names may not match directory names** - A component defined as `vpc/security-group` might live in `components/terraform/network/sg/`
2. **Developers must context-switch** - Need to look up component names in stack YAML files
3. **Inconsistent with standard workflows** - Most developers expect `.` to work like `terraform plan`
4. **Error-prone** - Easy to mistype component names or use wrong component for current directory

## Proposed Solution

Allow developers to use `.` (current directory) as the component argument. Atmos will:
1. Resolve the current working directory to an absolute path
2. Parse the path to extract component type, folder prefix, and component name
3. Validate the extracted component exists in the specified stack configuration
4. Execute the command using the resolved component name

### User Experience

```bash
# New capability - path-based resolution
cd components/terraform/vpc/security-group
atmos terraform plan . --stack dev

# Resolves to:
# - Component type: terraform
# - Component name: vpc/security-group
# - Stack: dev
# - Validates component exists in dev stack configuration
# - Executes: terraform plan with proper context
```

### Benefits

- **Improved Developer Experience** - Works like native Terraform commands
- **Reduced Cognitive Load** - No need to remember component names
- **Fewer Errors** - Automatic validation prevents wrong component/stack combinations
- **Faster Workflow** - Less context switching between code and configuration files
- **Consistency** - Aligns with industry-standard CLI patterns

## Feasibility Analysis

### Current Architecture

Based on research in the Atmos codebase, the current component resolution flow is:

```
CLI Args → ProcessCommandLineArgs() → Extract ComponentFromArg →
Load Stacks → Validate Component Exists → Get Component Path → Execute
```

**Key Files:**
- `internal/exec/cli_utils.go:692` - Extracts `ComponentFromArg` from positional arguments
- `internal/exec/utils.go` - Loads stacks and validates components
- `pkg/utils/component_path_utils.go` - Converts component name to filesystem path (forward resolution)

### Implementation Feasibility: **YES, FEASIBLE**

The proposed feature is **fully feasible** because:

1. ✅ **Forward resolution already exists** - `GetComponentPath()` converts component name → path
2. ✅ **Path parsing is straightforward** - Can extract component info from standard path structure
3. ✅ **Stack validation exists** - Current code already validates components against stacks
4. ✅ **Configuration is accessible** - `AtmosConfiguration` provides all needed path settings
5. ✅ **No architectural changes needed** - Can integrate into existing resolution pipeline

### Technical Challenges

| Challenge | Severity | Mitigation |
|-----------|----------|------------|
| Path normalization across platforms | Medium | Use `filepath.Abs()`, `filepath.Clean()`, existing cross-platform utilities |
| Environment variable overrides | Medium | Check all `ATMOS_COMPONENTS_*_BASE_PATH` variables during resolution |
| Symlinks and relative paths | Low | Resolve symlinks before path parsing, normalize to absolute paths |
| Multiple components in same directory | Low | Use stack validation to disambiguate (should be configuration error anyway) |
| Component inheritance (metadata.component) | Medium | Resolve actual component name after stack validation |
| Performance (stack loading on every command) | Low | Stack loading already required for validation; caching already exists |

## Detailed Design

### Phase 1: Path Pattern Extraction

Create reverse resolution utility to extract component information from filesystem path.

**New File:** `pkg/utils/component_reverse_path_utils.go`

```go
// ComponentInfo represents extracted component information from a path.
type ComponentInfo struct {
    ComponentType string // "terraform", "helmfile", "packer"
    FolderPrefix  string // "vpc", "networking/vpc", etc.
    ComponentName string // "security-group", "vpc/security-group", etc.
}

// ExtractComponentInfoFromPath extracts component information from an absolute filesystem path.
// Returns error if path is not within configured component directories.
func ExtractComponentInfoFromPath(
    atmosConfig schema.AtmosConfiguration,
    path string,
) (*ComponentInfo, error) {
    // 1. Normalize path (absolute, clean, resolve symlinks)
    // 2. Check environment variable overrides for base paths
    // 3. Determine component type (terraform/helmfile/packer) from path
    // 4. Extract folder prefix and component name
    // 5. Return ComponentInfo or error if path doesn't match patterns
}
```

**Algorithm:**

```
Input: /Users/dev/project/components/terraform/vpc/security-group

1. Normalize path:
   - filepath.Abs() → ensure absolute
   - filepath.Clean() → remove . and ..
   - filepath.EvalSymlinks() → resolve symlinks

2. Get base paths from atmosConfig:
   - Check ATMOS_COMPONENTS_TERRAFORM_BASE_PATH env var
   - Fall back to atmosConfig.Components.Terraform.BasePath
   - Construct full base: atmosConfig.BasePath + component base path

3. Match path against component bases:
   - Try terraform: /Users/dev/project/components/terraform
   - Try helmfile: /Users/dev/project/components/helmfile
   - Try packer: /Users/dev/project/components/packer

4. Extract relative path from matched base:
   - Full path: /Users/dev/project/components/terraform/vpc/security-group
   - Base path: /Users/dev/project/components/terraform
   - Relative: vpc/security-group

5. Parse relative path into folder prefix + component:
   - Split on filepath.Separator
   - folder prefix: vpc (or empty if no nesting)
   - component name: security-group

6. Return ComponentInfo{
     ComponentType: "terraform",
     FolderPrefix: "vpc",
     ComponentName: "security-group",
   }
```

**Error Cases:**
- Path not within any component base → Error: "Path is not within Atmos component directories"
- Path equals component base → Error: "Must specify a component directory, not the base directory"
- Cannot determine component type → Error: "Could not determine component type from path"

**Estimated Effort:** 50-100 lines, 2-4 hours

### Phase 2: Stack Validation and Resolution

Enhance component validation to support path-based lookup and tab completion.

**Modified File:** `internal/exec/cli_utils.go`

```go
// processArgsAndFlags - modify to detect "." as component argument
func processArgsAndFlags(componentType string, inputArgsAndFlags []string) (ArgsAndFlagsInfo, error) {
    // ... existing code ...

    // Around line 692 where ComponentFromArg is set:
    if len(additionalArgsAndFlags) > 1 {
        secondArg := additionalArgsAndFlags[1]

        // NEW: Check if argument is "." or a path
        if secondArg == "." || strings.Contains(secondArg, string(filepath.Separator)) {
            // Mark that this needs path resolution
            info.ComponentFromArg = secondArg
            info.NeedsPathResolution = true
        } else {
            info.ComponentFromArg = secondArg
        }
    }

    // ... rest of existing code ...
}
```

**Modified File:** `internal/exec/utils.go` (or new `internal/exec/component_resolver.go`)

```go
// ResolveComponentFromPath resolves a filesystem path to a component name.
// Validates that the resolved component exists in the specified stack.
func ResolveComponentFromPath(
    atmosConfig schema.AtmosConfiguration,
    path string,
    stack string,
    componentType string,
) (string, error) {
    // 1. Convert "." to absolute path of current working directory
    absPath := path
    if path == "." {
        var err error
        absPath, err = os.Getwd()
        if err != nil {
            return "", fmt.Errorf("failed to get current directory: %w", err)
        }
    }

    // 2. Extract component info from path
    componentInfo, err := u.ExtractComponentInfoFromPath(atmosConfig, absPath)
    if err != nil {
        return "", err
    }

    // 3. Verify component type matches
    if componentInfo.ComponentType != componentType {
        return "", fmt.Errorf(
            "path resolves to %s component but command expects %s component",
            componentInfo.ComponentType,
            componentType,
        )
    }

    // 4. Construct full component name (folder prefix + component name)
    componentName := componentInfo.ComponentName
    if componentInfo.FolderPrefix != "" {
        componentName = componentInfo.FolderPrefix + "/" + componentInfo.ComponentName
    }

    // 5. Validate component exists in stack configuration
    if stack != "" {
        stacksMap, err := FindStacksMap(&atmosConfig, nil, false)
        if err != nil {
            return "", fmt.Errorf("failed to load stacks: %w", err)
        }

        stackConfig, ok := stacksMap[stack]
        if !ok {
            return "", fmt.Errorf("stack '%s' not found", stack)
        }

        components := stackConfig["components"]
        terraformComponents := components.(map[string]any)[componentType].(map[string]any)

        if _, exists := terraformComponents[componentName]; !exists {
            return "", fmt.Errorf(
                "component '%s' not found in stack '%s' (resolved from path %s)",
                componentName,
                stack,
                absPath,
            )
        }
    }

    return componentName, nil
}
```

**Integration Point:** `ProcessCommandLineArgs()` in `cmd/cmd_utils.go`

```go
// After line 128 where ComponentFromArg is assigned
if argsAndFlagsInfo.NeedsPathResolution {
    resolvedComponent, err := exec.ResolveComponentFromPath(
        atmosConfig,
        argsAndFlagsInfo.ComponentFromArg,
        configAndStacksInfo.Stack,
        componentType,
    )
    if err != nil {
        return configAndStacksInfo, fmt.Errorf(
            "failed to resolve component from path '%s': %w",
            argsAndFlagsInfo.ComponentFromArg,
            err,
        )
    }
    configAndStacksInfo.ComponentFromArg = resolvedComponent
}
```

**Modified File:** `cmd/cmd_utils.go`

Update `stackFlagCompletion()` to resolve paths before filtering stacks:

```go
// Around line 779-822 in stackFlagCompletion
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		component := args[0]

		// NEW: Check if argument is a path that needs resolution
		if component == "." || strings.Contains(component, string(filepath.Separator)) {
			// Attempt to resolve path to component name
			// Use silent error handling - if resolution fails, just list all stacks
			configAndStacksInfo := schema.ConfigAndStacksInfo{}
			atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
			if err == nil {
				// Try to resolve the path
				resolvedComponent, err := e.ResolveComponentFromPath(
					atmosConfig,
					component,
					"", // No stack context yet - we're completing the stack flag
					"terraform", // Assume terraform for completion (could enhance to detect from cmd)
				)
				if err == nil {
					component = resolvedComponent
				}
				// If resolution fails, fall through to list all stacks (graceful degradation)
			}
		}

		output, err := listStacksForComponent(component)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks.
	output, err := listStacks(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}
```

**Key Points:**
- Path resolution happens during completion to filter stacks correctly
- Silent error handling - if path resolution fails, gracefully degrades to listing all stacks
- Improves UX: `atmos terraform plan . --stack <TAB>` shows only relevant stacks
- No breaking changes - existing component name completion still works

**Estimated Effort:** 150-250 lines, 6-10 hours

### Phase 3: Tab Completion Enhancement

Enhance component completion to suggest filesystem paths in addition to component names.

**Modified File:** `cmd/cmd_utils.go`

Update `ComponentsArgCompletion()` to support directory completion:

```go
// Around line 878-900 in ComponentsArgCompletion
func ComponentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Check if user is typing a path
		if toComplete == "." || strings.Contains(toComplete, string(filepath.Separator)) {
			// Enable directory completion for paths
			return nil, cobra.ShellCompDirectiveFilterDirs
		}

		// Otherwise, suggest component names
		output, err := listComponents(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// ... rest of existing code for flag completion ...
}
```

**Completion Behavior:**

```bash
# Typing component names - suggests from stack configs
$ atmos terraform plan v<TAB>
vpc  vpc-peering  vpn

# Typing paths - suggests directories
$ atmos terraform plan ./<TAB>
./vpc/  ./networking/  ./infra/

$ atmos terraform plan components/terraform/v<TAB>
components/terraform/vpc/  components/terraform/vpn/
```

**Benefits:**
- Dual completion: both component names AND filesystem paths
- `ShellCompDirectiveFilterDirs` enables directory-only suggestions for paths
- Seamless UX - shell autocompletes directories when user types path-like input
- No cognitive load - users can choose component names OR navigate filesystem

**Estimated Effort:** 50-100 lines, 2-4 hours

### Phase 4: Performance Optimization (Optional)

Add caching to avoid repeated stack loading for path resolution.

**Approach:**
- Use existing stack caching mechanisms
- Cache path-to-component mappings with TTL
- Invalidate cache when stack files change

**Note:** Current implementation already caches stack maps, so this optimization may not be necessary unless performance testing shows issues.

**Estimated Effort:** 100-150 lines, 2-4 hours

## Error Handling

All errors must use static error definitions from `errors/errors.go`:

```go
// New errors to add to errors/errors.go
var (
    ErrPathNotInComponentDir = errors.New("path is not within Atmos component directories")
    ErrComponentTypeMismatch = errors.New("path component type does not match command")
    ErrComponentNotInStack = errors.New("component not found in stack configuration")
    ErrPathResolutionFailed = errors.New("failed to resolve component from path")
)
```

**Error Messages:**

```bash
# Path not in component directory
$ cd /tmp
$ atmos terraform plan . --stack dev
Error: failed to resolve component from path '.': path is not within Atmos component directories

# Component type mismatch
$ cd components/helmfile/app
$ atmos terraform plan . --stack dev
Error: failed to resolve component from path '.': path component type does not match command
  Path resolves to: helmfile
  Command expects: terraform

# Component not found in stack
$ cd components/terraform/vpc
$ atmos terraform plan . --stack prod
Error: failed to resolve component from path '.': component not found in stack configuration
  Component: vpc
  Stack: prod
  Hint: Run 'atmos describe component vpc --stack prod' to see available stacks

# Missing stack flag
$ cd components/terraform/vpc
$ atmos terraform plan .
Error: --stack flag is required when using path-based component resolution
  Use: atmos terraform plan . --stack <stack-name>
```

## Scope

### In Scope

✅ **All Terraform commands that accept component argument:**
- `atmos terraform plan .`
- `atmos terraform apply .`
- `atmos terraform destroy .`
- `atmos terraform output .`
- `atmos terraform workspace .`
- `atmos terraform import .`
- `atmos terraform state .`
- All other terraform subcommands

✅ **Helmfile commands:**
- `atmos helmfile diff .`
- `atmos helmfile apply .`
- All other helmfile subcommands

✅ **Packer commands:**
- `atmos packer build .`
- All other packer subcommands

✅ **Describe commands:**
- `atmos describe component . --stack dev`
- Shows component configuration from current directory

✅ **Validate commands:**
- `atmos validate component . --stack dev`
- Validates component from current directory using JSON Schema or OPA policies

✅ **Path formats:**
- `.` (current directory)
- Relative paths: `./vpc`, `../networking/vpc`
- Absolute paths: `/Users/dev/project/components/terraform/vpc`

✅ **Validation:**
- Component exists in stack configuration
- Component type matches command
- Path is within configured component directories

### Out of Scope

❌ **Automatic stack detection** - Stack flag still required (may be separate PRD)
❌ **Glob patterns** - No support for `atmos terraform plan components/*`
❌ **Multi-component commands** - No batch operations across multiple paths
❌ **IDE integration** - No special support for editor plugins (future enhancement)
❌ **Shell completion** - No autocomplete for paths (future enhancement)

## Tab Completion Details

### Current Completion Implementation

Atmos uses **Cobra's built-in completion system** with custom enhancement functions:

**Key Files:**
- `cmd/completion.go` - Generates shell-specific completion scripts (Bash/Zsh/Fish/PowerShell)
- `cmd/cmd_utils.go` - Custom completion functions (`ComponentsArgCompletion`, `stackFlagCompletion`)
- `pkg/list/list_components.go` - Component listing and filtering logic
- `pkg/list/list_stacks.go` - Stack listing and filtering logic

**Current Behavior:**

```bash
# Component completion - suggests component names from stack configs
$ atmos terraform plan <TAB>
vpc  myapp  networking/vpc

# Stack completion - filters by component
$ atmos terraform plan vpc --stack <TAB>
dev  staging  prod
```

### Enhanced Completion with Path Resolution

**Component Argument Completion:**

```bash
# When typing component names - suggests from configurations
$ atmos terraform plan v<TAB>
vpc  vpc-peering  vpn

# When typing paths - enables directory completion
$ atmos terraform plan ./<TAB>
./vpc/  ./networking/  ./security/

$ atmos terraform plan components/terraform/<TAB>
components/terraform/vpc/  components/terraform/networking/
```

**Stack Flag Completion with Paths:**

```bash
# Path gets resolved to component name, then stacks are filtered
$ cd components/terraform/vpc
$ atmos terraform plan . --stack <TAB>
dev  staging  prod  # Only stacks containing 'vpc' component

# If resolution fails, gracefully degrades to all stacks
$ atmos terraform plan ./invalid --stack <TAB>
dev  staging  prod  qa  test  # All stacks (no filtering)
```

### Implementation Details

**Shell Completion Directives:**

Cobra provides several directives that control shell behavior:

- `cobra.ShellCompDirectiveNoFileComp` - No file completion (current default)
- `cobra.ShellCompDirectiveFilterDirs` - Only complete directories
- `cobra.ShellCompDirectiveFilterFileExt` - Filter by file extension

**Enhancement Strategy:**

1. **Detect path input** - Check if `toComplete` starts with `.` or contains path separators
2. **Switch completion mode** - Return `ShellCompDirectiveFilterDirs` for paths
3. **Fallback to component names** - If not a path, suggest component names from configs

**Benefits:**

- ✅ **Zero learning curve** - Works like standard shell completion
- ✅ **Flexible input** - Users can choose component names OR filesystem paths
- ✅ **Context-aware** - Stack completion filters correctly for paths
- ✅ **Graceful degradation** - Falls back to showing all items if resolution fails
- ✅ **No breaking changes** - Existing completion behavior preserved

### Completion Test Cases

Add to `tests/test-cases/tab-completions.yaml`:

```yaml
- name: completion for path should enable directory suggestions
  enabled: true
  snapshot: true
  description: "Test that directory completion is enabled for path input"
  workdir: "fixtures/scenarios/completions"
  command: "atmos"
  args:
    - "__completeNoDesc"
    - "terraform"
    - "plan"
    - "./"
  expect:
    # ShellCompDirectiveFilterDirs = 16
    stdout:
      - ":16"  # Cobra completion directive for directory filtering
    exit_code: 0

- name: completion for stacks with path should filter by resolved component
  enabled: true
  snapshot: true
  description: "Test that stack completion resolves path and filters stacks"
  workdir: "fixtures/scenarios/completions/components/terraform/vpc"
  command: "atmos"
  args:
    - "__completeNoDesc"
    - "terraform"
    - "plan"
    - "."
    - "--stack"
  expect:
    stdout:
      - "dev"
      - "prod"
    exit_code: 0
```

## Testing Strategy

### Unit Tests

**File:** `pkg/utils/component_reverse_path_utils_test.go`

```go
func TestExtractComponentInfoFromPath(t *testing.T) {
    tests := []struct {
        name          string
        atmosConfig   schema.AtmosConfiguration
        path          string
        want          *ComponentInfo
        wantErr       bool
        errorContains string
    }{
        {
            name: "terraform component with folder prefix",
            atmosConfig: schema.AtmosConfiguration{
                BasePath: "/project",
                Components: schema.Components{
                    Terraform: schema.Terraform{
                        BasePath: "components/terraform",
                    },
                },
            },
            path: "/project/components/terraform/vpc/security-group",
            want: &ComponentInfo{
                ComponentType: "terraform",
                FolderPrefix:  "vpc",
                ComponentName: "security-group",
            },
            wantErr: false,
        },
        {
            name: "path not in component directory",
            // ... test configuration ...
            path:          "/tmp/random",
            wantErr:       true,
            errorContains: "not within Atmos component directories",
        },
        // ... more test cases ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ExtractComponentInfoFromPath(tt.atmosConfig, tt.path)
            // ... assertions ...
        })
    }
}
```

**File:** `internal/exec/component_resolver_test.go`

```go
func TestResolveComponentFromPath(t *testing.T) {
    // Test cases:
    // - Successful resolution with valid component
    // - Component type mismatch
    // - Component not in stack
    // - Invalid path
    // - Environment variable overrides
    // - Symlink resolution
}
```

### Integration Tests

**File:** `tests/test-cases/path-resolution/`

Create test fixtures with:
- Stack configurations defining components
- Component directory structure
- Test cases for various path formats

**Test Scenarios:**
1. `atmos terraform plan . --stack dev` from component directory
2. Path resolution with environment variable overrides
3. Error handling for invalid paths
4. Error handling for missing components
5. Cross-platform path handling (Windows vs Unix)
6. Tab completion with path arguments
7. Stack filtering with path-based component resolution

**Golden Snapshots:**
- Success messages showing resolved component
- Error messages for various failure modes

### Manual Testing Checklist

- [ ] Test on macOS with Unix paths
- [ ] Test on Windows with Windows paths
- [ ] Test with symlinked component directories
- [ ] Test with absolute and relative paths
- [ ] Test with environment variable overrides
- [ ] Test error messages are clear and actionable
- [ ] Test performance with large stack configurations
- [ ] Test tab completion for component paths (Bash/Zsh/Fish)
- [ ] Test stack flag completion with `.` argument
- [ ] Verify directory-only suggestions when typing paths
- [ ] Test completion graceful degradation on resolution failures

## Documentation

### User-Facing Documentation

**File:** `website/docs/cli/commands/terraform/terraform.mdx`

Add section:

```markdown
## Using Path-Based Component Resolution

Atmos supports using filesystem paths instead of component names for convenience:

```bash
# Navigate to component directory
cd components/terraform/vpc/security-group

# Use . to reference current directory
atmos terraform plan . --stack dev
```

This automatically resolves the path to the component name configured in your stack.

**Requirements:**
- Must be inside a component directory under configured base path
- Must specify `--stack` flag
- Component must exist in the specified stack configuration

**Supported path formats:**
- `.` - Current directory
- `./component` - Relative path from current directory
- `../other-component` - Relative path to sibling directory
- `/absolute/path/to/component` - Absolute path

**Tab completion:**
Atmos provides intelligent tab completion for both component names and filesystem paths:

```bash
# Component name completion (from stack configs)
$ atmos terraform plan v<TAB>
vpc  vpc-peering  vpn

# Directory completion (when typing paths)
$ atmos terraform plan ./<TAB>
./vpc/  ./networking/  ./security/

# Stack completion with path resolution
$ atmos terraform plan . --stack <TAB>
dev  prod  # Only stacks containing the component in current directory
```

The completion system automatically:
- Detects when you're typing a path vs. a component name
- Enables directory-only suggestions for paths
- Resolves paths to component names for stack filtering
- Falls back gracefully if resolution fails

**Error handling:**
If the path cannot be resolved, Atmos will provide a clear error message explaining:
- Whether the path is within component directories
- Which component type was detected
- Whether the component exists in the stack
```

**File:** `website/docs/core-concepts/components.mdx`

Add section explaining path-based resolution as an alternative to component names.

### Internal Documentation

**File:** `docs/prd/component-path-resolution.md` (this document)

**File:** `pkg/utils/component_reverse_path_utils.go` (inline documentation)

Comprehensive package and function comments following godoc conventions.

## Implementation Phases

### Phase 1: Core Path Resolution (Required)
- **Deliverables:**
  - `pkg/utils/component_reverse_path_utils.go` with path extraction
  - Unit tests with >80% coverage
  - Error definitions in `errors/errors.go`
- **Effort:** 6-12 hours
- **Risk:** Low - isolated utility function

### Phase 2: Stack Validation Integration (Required)
- **Deliverables:**
  - `internal/exec/component_resolver.go` with stack validation
  - Integration into `ProcessCommandLineArgs()`
  - Enhanced `stackFlagCompletion()` for path-aware stack filtering
  - Unit tests with mocks
- **Effort:** 10-18 hours
- **Risk:** Medium - touches core command processing

### Phase 3: Tab Completion Enhancement (Required)
- **Deliverables:**
  - Enhanced `ComponentsArgCompletion()` for directory suggestions
  - Integration tests for completion with paths
  - Documentation for completion behavior
- **Effort:** 4-8 hours
- **Risk:** Low - isolated completion enhancement

### Phase 4: Testing & Documentation (Required)
- **Deliverables:**
  - Integration tests with golden snapshots
  - User-facing documentation
  - Blog post for release notes
- **Effort:** 8-12 hours
- **Risk:** Low - standard testing practices

### Phase 5: Performance Optimization (Optional)
- **Deliverables:**
  - Component cache implementation
  - Performance benchmarks
- **Effort:** 4-8 hours
- **Risk:** Low - optional enhancement

**Total Estimated Effort:** 30-56 hours

## Success Metrics

- ✅ All Terraform/Helmfile/Packer commands support `.` as component argument
- ✅ Path resolution works on macOS, Linux, and Windows
- ✅ Error messages provide actionable guidance
- ✅ No performance regression (command execution time < +50ms)
- ✅ Unit test coverage >80%
- ✅ Integration tests cover common workflows
- ✅ Documentation clearly explains feature and requirements

## Security Considerations

- **Path Traversal Prevention:** All paths normalized and validated against configured base paths
- **Symlink Handling:** Resolve symlinks before validation to prevent escaping component directories
- **Input Validation:** Reject paths containing suspicious patterns (e.g., `..` after normalization)
- **Error Messages:** Don't leak filesystem structure in error messages to unauthorized users

## Backwards Compatibility

✅ **Fully Backwards Compatible**

- Existing component name syntax continues to work unchanged
- New path syntax only activated when argument contains `.` or path separators
- No breaking changes to existing workflows
- No changes to stack configuration format

## Alternatives Considered

### Alternative 1: Auto-detect stack from directory

**Idea:** Also auto-detect which stack to use based on directory structure or metadata files.

**Decision:** Out of scope for this PRD. Requires separate design for:
- Stack selection algorithm
- Handling multiple stacks with same component
- Configuration format for directory-to-stack mapping

**Future Consideration:** Separate PRD for automatic stack detection.

### Alternative 2: Support glob patterns

**Idea:** Support `atmos terraform plan components/vpc/*` to operate on multiple components.

**Decision:** Out of scope. Complexity:
- Multi-component execution model
- Error handling when some components fail
- Output formatting for multiple results
- Stack validation for multiple components

**Future Consideration:** Separate PRD for batch operations.

### Alternative 3: Configuration file in component directory

**Idea:** Add `.atmos.yaml` in component directories specifying component name and stacks.

**Decision:** Rejected. Violates Atmos philosophy:
- Components should be stack-agnostic
- Stack configuration is source of truth
- Adds maintenance burden (must keep in sync)

## Open Questions

1. **Should we support relative paths outside current directory?**
   - Example: `cd /tmp && atmos terraform plan ~/project/components/terraform/vpc`
   - Decision: YES - normalize to absolute path and validate

2. **What happens with component inheritance (metadata.component)?**
   - Example: Component `vpc-dev` inherits from `vpc`
   - Decision: Resolve to the component name in stack configuration (e.g., `vpc-dev`)

3. **Should --stack flag be required?**
   - Current design requires it for validation
   - Could allow omitting if component exists in only one stack
   - Decision: REQUIRE --stack flag (simpler, more explicit, consistent)

4. **Should we cache path-to-component mappings?**
   - Decision: DEFER to Phase 4 - only if performance testing shows need

## References

- **Research Document:** `/Users/erik/conductor/atmos/COMPONENT_RESOLUTION_RESEARCH.md`
- **Command Registry Pattern:** `docs/prd/command-registry-pattern.md`
- **Error Handling Strategy:** `docs/prd/error-handling-strategy.md`
- **Testing Strategy:** `docs/prd/testing-strategy.md`
- **Component Path Utils:** `pkg/utils/component_path_utils.go`
- **CLI Utils:** `internal/exec/cli_utils.go`
- **Process Stacks:** `internal/exec/utils.go`
- **Completion Utils:** `cmd/cmd_utils.go` (lines 779-900)
- **List Components:** `pkg/list/list_components.go`
- **List Stacks:** `pkg/list/list_stacks.go`
- **Completion Command:** `cmd/completion.go`
- **Completion Tests:** `tests/test-cases/tab-completions.yaml`
- **Completion Docs:** `website/docs/cli/commands/completion.mdx`

## Approval

- [ ] Architecture Review - Validates against CLAUDE.md patterns
- [ ] Security Review - Path traversal and input validation
- [ ] Performance Review - No significant regression
- [ ] Documentation Review - Clear user guidance
- [ ] Testing Review - Adequate coverage and golden snapshots

## Implementation Checklist

### Code Changes
- [ ] Create `pkg/utils/component_reverse_path_utils.go`
- [ ] Add `ExtractComponentInfoFromPath()` function
- [ ] Add error definitions to `errors/errors.go`
- [ ] Create `internal/exec/component_resolver.go`
- [ ] Add `ResolveComponentFromPath()` function
- [ ] Modify `internal/exec/cli_utils.go` to detect path arguments
- [ ] Integrate path resolution into `ProcessCommandLineArgs()`
- [ ] Enhance `cmd/cmd_utils.go::stackFlagCompletion()` for path-aware filtering
- [ ] Enhance `cmd/cmd_utils.go::ComponentsArgCompletion()` for directory suggestions
- [ ] Add `defer perf.Track()` to public functions

### Testing
- [ ] Unit tests for `ExtractComponentInfoFromPath()`
- [ ] Unit tests for `ResolveComponentFromPath()`
- [ ] Unit tests for enhanced completion functions
- [ ] Integration tests in `tests/test-cases/`
- [ ] Tab completion tests in `tests/test-cases/tab-completions.yaml`
- [ ] Golden snapshots for success cases
- [ ] Golden snapshots for error cases
- [ ] Golden snapshots for completion outputs
- [ ] Cross-platform testing (macOS/Linux/Windows)
- [ ] Shell-specific completion testing (Bash/Zsh/Fish)
- [ ] Performance benchmarks
- [ ] Verify >80% code coverage

### Documentation
- [ ] Update `website/docs/cli/commands/terraform/terraform.mdx`
- [ ] Update `website/docs/core-concepts/components.mdx`
- [ ] Add inline godoc comments
- [ ] Create blog post for release notes
- [ ] Update CHANGELOG.md

### Release
- [ ] PR description following template
- [ ] Blog post with `feature` tag
- [ ] Update version in appropriate files
- [ ] Test in staging environment
- [ ] Monitor for issues post-release
