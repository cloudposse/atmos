# PRD: Lazy Auth Detection for List/Describe Commands

## Executive Summary

Implement lazy authentication detection for `atmos list` and `atmos describe` commands. Currently, authentication (identity prompting) triggers before stack processing, even when stack configurations don't use any auth-requiring functions. This creates unnecessary friction for simple operations. The solution scans stack configurations for auth-requiring patterns and only triggers authentication when those patterns are found AND template/function processing is enabled.

## Problem Statement

### Current Limitation

When running list or describe commands, Atmos triggers authentication flow early in the command lifecycle:

1. `GetIdentityFromFlags()` extracts identity from CLI flags or environment
2. If a default identity is configured, `autoDetectDefaultIdentity()` prompts for selection
3. `CreateAndAuthenticateManagerWithAtmosConfig()` creates and authenticates the auth manager
4. Only then does stack processing begin

This happens **before** we know if any stack configurations actually require authentication.

### User Impact

**Scenario 1: Simple Stack Listing**
```bash
# User just wants to list stacks - no auth-requiring functions used
$ atmos list stacks
# But gets prompted for identity selection anyway (if default configured)
Select an identity:
  > developer-sandbox
    developer-prod
    platform-admin
```

**Scenario 2: Describe Component Without Auth Functions**
```bash
# Stack only has static configuration
# stacks/simple.yaml:
# components:
#   terraform:
#     vpc:
#       vars:
#         cidr_block: "10.0.0.0/16"

$ atmos describe component vpc -s simple
# Still prompts for identity selection
```

**Scenario 3: Auth Is Actually Needed**
```bash
# Stack uses auth-requiring function
# stacks/complex.yaml:
# components:
#   terraform:
#     app:
#       vars:
#         vpc_id: !terraform.state vpc prod .outputs.vpc_id

$ atmos describe component app -s complex
# Should prompt - auth is needed for !terraform.state
```

### Root Cause

The auth flow in `pkg/auth/manager_helpers.go` is invoked unconditionally:

```go
// CreateAndAuthenticateManagerWithAtmosConfig always triggers authentication
func CreateAndAuthenticateManagerWithAtmosConfig(...) (AuthManager, error) {
    // 1. Resolve identity name (may prompt)
    identityName, err := resolveIdentityName(identityName, authConfig)

    // 2. Create auth manager
    authManager, err := createAuthManagerInternal(...)

    // 3. Authenticate (always happens if identity resolved)
    if err := authManager.Authenticate(ctx, identityName); err != nil {
        return nil, err
    }

    return authManager, nil
}
```

## Goals

### Primary Goals

1. **Lazy auth detection** - Only trigger authentication when stack configs use auth-requiring functions
2. **Scoped scanning** - Scan only stacks/components being queried, not all stack files
3. **Comprehensive detection** - Detect auth patterns in YAML functions AND Go templates
4. **Backward compatible** - Explicit `--identity` flag still authenticates immediately
5. **Default identity awareness** - Even configured defaults are lazy (scan first)

### Secondary Goals

1. **Performance** - No significant overhead from pattern scanning
2. **Maintainability** - Easy to add new auth-requiring patterns
3. **Testability** - Scanner and lazy auth manager are independently testable

## Solution Overview

### Approach: Pre-scan + Lazy Auth Manager

1. **Pre-scan**: Before template/function processing, scan raw YAML for auth-requiring patterns
2. **Lazy auth manager**: Defer authentication until first access (if patterns found)
3. **Scoped scanning**: Only scan stacks/components matching query filters

### Auth-Requiring Patterns

| Pattern | Description | Requires |
|---------|-------------|----------|
| `!terraform.state` | Reads Terraform state from S3/Azure/GCS backends | AWS/Azure/GCP credentials |
| `!terraform.output` | Reads Terraform outputs from remote state | AWS/Azure/GCP credentials |
| `!store.get` / `!store` | Reads from secret stores (SSM, Key Vault, etc.) | Store-specific credentials |
| `atmos.Component` | Go template function that may reference auth-requiring components | Depends on target component |

### Decision Flow

```
┌─────────────────────────────────────────────────────────────┐
│ User runs: atmos describe stacks --stack prod               │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 1. Check: Was --identity explicitly provided?               │
│    YES → Authenticate immediately (user explicitly wants)   │
│    NO  → Continue to lazy detection                         │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Load raw stack configs (FindStacksMap)                   │
│    - YAML files parsed, imports resolved                    │
│    - Templates NOT evaluated yet                            │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Scan scoped stacks for auth patterns                     │
│    - Only scan stacks matching --stack filter               │
│    - Only scan components matching --component filter       │
│    - Check for !terraform.state, !terraform.output, etc.    │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. Auth patterns found AND processYamlFunctions=true?       │
│    YES → Trigger lazy auth (resolve default, authenticate)  │
│    NO  → Continue without auth                              │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 5. Process templates and YAML functions                     │
│    - AuthContext available if auth was triggered            │
│    - !terraform.state/output use AuthContext for AWS access │
└─────────────────────────────────────────────────────────────┘
```

## Technical Implementation

### 1. Auth Pattern Scanner Package

**New package:** `pkg/auth/scanner/`

**File:** `pkg/auth/scanner/scanner.go`

```go
package scanner

import (
    "regexp"
    "sync"

    "gopkg.in/yaml.v3"
)

// Pattern strings for auth-requiring functions.
var authPatternStrings = []string{
    // YAML functions (direct)
    `!terraform\.state\s`,
    `!terraform\.output\s`,
    `!store\.get\s`,
    `!store\s+\w`,  // !store followed by store name

    // Template functions that may trigger auth internally
    `atmos\.Component`,
}

var (
    authPatterns     []*regexp.Regexp
    authPatternsOnce sync.Once
)

func initPatterns() {
    authPatternsOnce.Do(func() {
        authPatterns = make([]*regexp.Regexp, len(authPatternStrings))
        for i, p := range authPatternStrings {
            authPatterns[i] = regexp.MustCompile(p)
        }
    })
}

// RequiresAuth scans content for auth-requiring patterns.
// Returns true if any pattern is found.
func RequiresAuth(content string) bool {
    initPatterns()
    for _, pattern := range authPatterns {
        if pattern.MatchString(content) {
            return true
        }
    }
    return false
}

// ScanStackSection scans a component section for auth patterns.
// Converts the section to YAML string for pattern matching.
func ScanStackSection(section map[string]any) (bool, error) {
    yamlBytes, err := yaml.Marshal(section)
    if err != nil {
        return false, err
    }
    return RequiresAuth(string(yamlBytes)), nil
}

// ScanStacksMap scans stacks for auth patterns with optional filtering.
func ScanStacksMap(
    stacksMap map[string]any,
    filterStack string,
    filterComponents []string,
) bool {
    for stackName, stackSection := range stacksMap {
        // Apply stack filter if provided
        if filterStack != "" && !matchesFilter(stackName, filterStack) {
            continue
        }

        // Extract and scan components section
        if requiresAuth := scanComponentsSection(stackSection, filterComponents); requiresAuth {
            return true
        }
    }
    return false
}
```

### 2. Lazy Auth Manager

**New file:** `pkg/auth/lazy.go`

```go
package auth

import (
    "context"
    "sync"

    "github.com/cloudposse/atmos/pkg/schema"
)

// LazyAuthManager wraps auth creation and defers until first access.
type LazyAuthManager struct {
    identityName string
    authConfig   *schema.AuthConfig
    selectValue  string
    atmosConfig  *schema.AtmosConfiguration

    once    sync.Once
    manager AuthManager
    err     error
}

// NewLazyAuthManager creates a lazy auth manager.
func NewLazyAuthManager(
    identityName string,
    authConfig *schema.AuthConfig,
    selectValue string,
    atmosConfig *schema.AtmosConfiguration,
) *LazyAuthManager {
    return &LazyAuthManager{
        identityName: identityName,
        authConfig:   authConfig,
        selectValue:  selectValue,
        atmosConfig:  atmosConfig,
    }
}

// Get returns the AuthManager, creating and authenticating on first call.
// Thread-safe via sync.Once.
func (l *LazyAuthManager) Get(ctx context.Context) (AuthManager, error) {
    l.once.Do(func() {
        l.manager, l.err = CreateAndAuthenticateManagerWithAtmosConfig(
            l.identityName,
            l.authConfig,
            l.selectValue,
            l.atmosConfig,
        )
    })
    return l.manager, l.err
}

// IsInitialized returns true if auth has been triggered.
func (l *LazyAuthManager) IsInitialized() bool {
    return l.manager != nil || l.err != nil
}
```

### 3. Update DescribeStacksArgs

**File:** `internal/exec/describe_stacks.go`

```go
type DescribeStacksArgs struct {
    Query                string
    FilterByStack        string
    Components           []string
    ComponentTypes       []string
    Sections             []string
    IgnoreMissingFiles   bool
    ProcessTemplates     bool
    ProcessYamlFunctions bool
    IncludeEmptyStacks   bool
    Skip                 []string
    Format               string
    File                 string
    AuthManager          auth.AuthManager       // Immediate auth (from explicit --identity)
    LazyAuthManager      *auth.LazyAuthManager  // NEW: Deferred auth (when no explicit --identity)
}
```

### 4. Auth Resolution in ExecuteDescribeStacks

**File:** `internal/exec/describe_stacks.go`

```go
func ExecuteDescribeStacks(
    atmosConfig *schema.AtmosConfiguration,
    filterByStack string,
    components []string,
    componentTypes []string,
    sections []string,
    ignoreMissingFiles bool,
    processTemplates bool,
    processYamlFunctions bool,
    includeEmptyStacks bool,
    skip []string,
    authManager auth.AuthManager,
    lazyAuthManager *auth.LazyAuthManager,  // NEW parameter
) (map[string]any, error) {
    defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacks")()

    // 1. Load raw stack configs (no template processing yet)
    stacksMap, rawStackConfigs, err := FindStacksMap(atmosConfig, ignoreMissingFiles)
    if err != nil {
        return nil, err
    }

    // 2. Resolve auth manager (NEW logic)
    resolvedAuthManager := authManager
    if authManager == nil && lazyAuthManager != nil && processYamlFunctions {
        // Scan for auth patterns in scoped stacks
        if scanner.ScanStacksMap(stacksMap, filterByStack, components) {
            // Auth patterns found - trigger lazy auth
            resolvedAuthManager, err = lazyAuthManager.Get(context.Background())
            if err != nil {
                return nil, err
            }
        }
    }

    // 3. Continue with template/function processing using resolvedAuthManager
    // ... rest of existing implementation
}
```

### 5. Command Layer Updates

**File:** `cmd/describe_stacks.go`

```go
func executeDescribeStacksCmd(cmd *cobra.Command, args []string) error {
    // ... existing config loading ...

    identityName := GetIdentityFromFlags(cmd, os.Args)

    var authManager auth.AuthManager
    var lazyAuthManager *auth.LazyAuthManager

    switch {
    case identityName != "" && identityName != cfg.IdentityFlagDisabledValue:
        // Explicit --identity=NAME: authenticate immediately
        authManager, err = auth.CreateAndAuthenticateManagerWithAtmosConfig(
            identityName, &atmosConfig.Auth, cfg.IdentityFlagSelectValue, &atmosConfig)
        if err != nil {
            return err
        }

    case identityName != cfg.IdentityFlagDisabledValue:
        // No explicit identity (even if default configured): use lazy auth
        lazyAuthManager = auth.NewLazyAuthManager(
            "", &atmosConfig.Auth, cfg.IdentityFlagSelectValue, &atmosConfig)

    // If --identity=false, both are nil (auth disabled)
    }

    // Pass both to execution
    describe.AuthManager = authManager
    describe.LazyAuthManager = lazyAuthManager

    return g.newDescribeStacksExec.Execute(&atmosConfig, describe)
}
```

**File:** `cmd/list/utils.go`

```go
// createAuthForList creates auth manager(s) for list commands.
func createAuthForList(
    cmd *cobra.Command,
    atmosConfig *schema.AtmosConfiguration,
) (auth.AuthManager, *auth.LazyAuthManager, error) {
    identityName := getIdentityFromCommand(cmd)

    switch {
    case identityName != "" && identityName != cfg.IdentityFlagDisabledValue:
        // Explicit --identity: authenticate immediately
        authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
            identityName, &atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig)
        return authManager, nil, err

    case identityName != cfg.IdentityFlagDisabledValue:
        // No explicit identity: use lazy auth
        lazyAuth := auth.NewLazyAuthManager(
            "", &atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig)
        return nil, lazyAuth, nil

    default:
        // Auth disabled
        return nil, nil, nil
    }
}
```

## Behavior Matrix

| Scenario | Current Behavior | New Behavior |
|----------|------------------|--------------|
| `atmos list stacks` (no auth functions) | May prompt for identity if default configured | No prompt (no auth patterns found) |
| `atmos list stacks` (with auth functions in some stacks) | May prompt for identity | No prompt (default: `processYamlFunctions=false`) |
| `atmos list stacks --process-functions` (with auth functions) | Prompt | Prompt (auth patterns found, functions enabled) |
| `atmos describe component vpc -s simple` (no auth functions) | May prompt | No prompt |
| `atmos describe component app -s complex` (uses `!terraform.state`) | May prompt | Prompt (auth patterns found) |
| `atmos describe stacks --identity my-id` | Authenticate immediately | Authenticate immediately (unchanged) |
| `atmos describe stacks --identity=false` | No auth | No auth (unchanged) |
| Default identity in atmos.yaml, no auth functions | Auto-authenticate | No prompt (lazy - no patterns found) |
| Default identity in atmos.yaml, with auth functions | Auto-authenticate | Prompt only if patterns found |

## Testing Strategy

### Unit Tests

**File:** `pkg/auth/scanner/scanner_test.go`

```go
func TestRequiresAuth(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        expected bool
    }{
        {
            name:     "terraform.state YAML function",
            content:  "vpc_id: !terraform.state vpc prod .outputs.vpc_id",
            expected: true,
        },
        {
            name:     "terraform.output YAML function",
            content:  "output: !terraform.output component stack output_name",
            expected: true,
        },
        {
            name:     "store.get YAML function",
            content:  "secret: !store.get my-store secret-key",
            expected: true,
        },
        {
            name:     "atmos.Component in template",
            content:  `value: "{{ (atmos.Component \"vpc\" .stack).outputs.id }}"`,
            expected: true,
        },
        {
            name:     "no auth patterns - simple vars",
            content:  "vars:\n  key: value\n  count: 5",
            expected: false,
        },
        {
            name:     "terraform word not a function",
            content:  "terraform_workspace: default\nterraform_version: 1.5.0",
            expected: false,
        },
        {
            name:     "nested in complex structure",
            content:  `{{ if .enabled }}{{ atmos.Component "vpc" .stack }}{{ end }}`,
            expected: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := scanner.RequiresAuth(tt.content)
            assert.Equal(t, tt.expected, got)
        })
    }
}
```

**File:** `pkg/auth/lazy_test.go`

```go
func TestLazyAuthManager(t *testing.T) {
    t.Run("defers authentication until Get called", func(t *testing.T) {
        // Create lazy auth manager
        lazy := auth.NewLazyAuthManager(...)

        // Not initialized yet
        assert.False(t, lazy.IsInitialized())

        // Call Get
        manager, err := lazy.Get(context.Background())

        // Now initialized
        assert.True(t, lazy.IsInitialized())
        assert.NoError(t, err)
        assert.NotNil(t, manager)
    })

    t.Run("thread-safe concurrent access", func(t *testing.T) {
        lazy := auth.NewLazyAuthManager(...)

        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                manager, _ := lazy.Get(context.Background())
                assert.NotNil(t, manager)
            }()
        }
        wg.Wait()
    })
}
```

### Integration Tests

**Fixtures:** `tests/fixtures/scenarios/lazy-auth/`

**`stacks/no-auth.yaml`**
```yaml
components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
        name: "main-vpc"
```

**`stacks/with-terraform-state.yaml`**
```yaml
components:
  terraform:
    app:
      vars:
        vpc_id: !terraform.state vpc prod .outputs.vpc_id
```

**`stacks/with-template-auth.yaml`**
```yaml
components:
  terraform:
    app:
      vars:
        vpc_config: "{{ (atmos.Component \"vpc\" .stack).outputs }}"
```

**Test Cases:**

```go
func TestLazyAuthIntegration(t *testing.T) {
    t.Run("no auth triggered for simple stacks", func(t *testing.T) {
        // Run describe stacks on no-auth.yaml
        // Verify auth manager is never created
    })

    t.Run("auth triggered for terraform.state", func(t *testing.T) {
        // Run describe stacks on with-terraform-state.yaml with --process-functions
        // Verify auth is triggered before template processing
    })

    t.Run("explicit --identity skips scanning", func(t *testing.T) {
        // Run with --identity=my-identity
        // Verify immediate auth without scanning
    })

    t.Run("scoped scan respects --stack filter", func(t *testing.T) {
        // Run with --stack=no-auth when other stacks have auth patterns
        // Verify no auth triggered
    })
}
```

## Performance Considerations

### Scanning Overhead

- **Pattern matching**: O(n) where n = YAML content size
- **Compiled regex**: Patterns compiled once (sync.Once)
- **Early exit**: Returns on first pattern match
- **Scoped scanning**: Only scans stacks/components matching filters

### Expected Impact

| Operation | Current | With Lazy Auth |
|-----------|---------|----------------|
| `list stacks` (no auth) | ~100ms (may include auth prompt wait) | ~100ms (no prompt) |
| `describe stacks` (no auth) | ~200ms (may include auth prompt wait) | ~210ms (scanning overhead) |
| `describe stacks` (with auth) | ~500ms (includes auth) | ~510ms (scanning + auth) |

### Memory

- Scanner patterns: ~1KB (compiled regex)
- Lazy auth manager: ~100 bytes per instance
- No additional stack loading (uses existing FindStacksMap result)

## Migration Guide

### For End Users

**No breaking changes** - existing commands work identically:

```bash
# Still works (explicit identity)
atmos describe stacks --identity my-identity

# Still works (auth disabled)
atmos describe stacks --identity=false

# NEW: No unnecessary prompts
atmos list stacks  # No prompt if stacks don't use auth functions
```

### Behavior Change

**Previous:** Default identity auto-selected/prompted before stack processing
**New:** Default identity only selected/prompted when auth patterns detected

This is a **user experience improvement**, not a breaking change.

### For CI/CD Pipelines

Recommended pattern unchanged:

```bash
# Explicit identity for CI (still recommended)
atmos describe stacks --identity ci-automation

# Or with ATMOS_IDENTITY
export ATMOS_IDENTITY=ci-automation
atmos describe stacks
```

## Security Considerations

### No Credential Exposure

- Scanner operates on raw YAML strings
- No credentials accessed during scanning
- Auth only triggered when truly needed

### Pattern Detection Accuracy

- **False positives**: Rare - patterns are specific (e.g., `!terraform.state\s`)
- **False negatives**: Could miss auth patterns in:
  - Dynamically generated template content
  - Heavily obfuscated configurations
- **Mitigation**: Use `--identity` flag when auth is definitively needed

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/auth/scanner/scanner.go` | NEW - Pattern detection |
| `pkg/auth/scanner/scanner_test.go` | NEW - Unit tests |
| `pkg/auth/lazy.go` | NEW - Lazy auth manager |
| `pkg/auth/lazy_test.go` | NEW - Unit tests |
| `internal/exec/describe_stacks.go` | Add LazyAuthManager field, auth resolution logic |
| `internal/exec/describe_component.go` | Similar changes |
| `cmd/describe_stacks.go` | Use lazy auth when no explicit --identity |
| `cmd/describe_component.go` | Use lazy auth when no explicit --identity |
| `cmd/list/utils.go` | Use lazy auth for list commands |
| `cmd/list/stacks.go` | Wire up lazy auth |
| `cmd/list/components.go` | Wire up lazy auth |
| `cmd/list/instances.go` | Wire up lazy auth |

## Success Criteria

### Functional Requirements

- [ ] Scanner detects all auth-requiring patterns
- [ ] Lazy auth manager defers authentication until Get() called
- [ ] No auth prompt when stacks don't use auth functions
- [ ] Auth prompt when auth functions detected and processing enabled
- [ ] Explicit `--identity` still authenticates immediately
- [ ] Scoped scanning respects `--stack` and `--component` filters
- [ ] Backward compatible with all existing workflows

### Testing Requirements

- [ ] Unit tests for scanner (all patterns)
- [ ] Unit tests for lazy auth manager (thread safety)
- [ ] Integration tests for describe commands
- [ ] Integration tests for list commands
- [ ] Test coverage >80% for new code

### Performance Requirements

- [ ] Scanning overhead <20ms for typical stack configurations
- [ ] No additional file I/O (uses existing FindStacksMap result)

## Implementation Checklist

### Phase 1: Core Scanner (2-3 hours)
- [ ] Create `pkg/auth/scanner/scanner.go`
- [ ] Create `pkg/auth/scanner/scanner_test.go`
- [ ] Implement pattern detection for all auth functions
- [ ] Add comprehensive unit tests

### Phase 2: Lazy Auth Manager (1-2 hours)
- [ ] Create `pkg/auth/lazy.go`
- [ ] Create `pkg/auth/lazy_test.go`
- [ ] Implement thread-safe lazy initialization
- [ ] Add unit tests including concurrency tests

### Phase 3: Describe Commands (3-4 hours)
- [ ] Update `internal/exec/describe_stacks.go`
- [ ] Update `internal/exec/describe_component.go`
- [ ] Update `cmd/describe_stacks.go`
- [ ] Update `cmd/describe_component.go`
- [ ] Add integration tests

### Phase 4: List Commands (2-3 hours)
- [ ] Update `cmd/list/utils.go`
- [ ] Update `cmd/list/stacks.go`
- [ ] Update `cmd/list/components.go`
- [ ] Update `cmd/list/instances.go`
- [ ] Add integration tests

### Phase 5: Validation (2-3 hours)
- [ ] Run full test suite
- [ ] Run linter (golangci-lint)
- [ ] Manual testing with real configurations
- [ ] Verify backward compatibility
- [ ] Code review

## References

- **Related PRD:** `docs/prd/describe-commands-identity-flag.md` - Identity flag for describe commands
- **Related PRD:** `docs/prd/auth-default-settings.md` - Auth defaults configuration
- **Related PRD:** `docs/prd/terraform-template-functions-auth-context.md` - AuthContext propagation
- **Implementation:** `pkg/auth/manager_helpers.go` - Current auth manager creation
- **Implementation:** `internal/exec/describe_stacks.go` - Describe stacks execution
- **Implementation:** `cmd/list/utils.go` - List command utilities

## Changelog

| Date | Version | Changes | Author |
|------|---------|---------|--------|
| 2025-01-13 | 1.0 | Initial PRD created | Claude Code |
