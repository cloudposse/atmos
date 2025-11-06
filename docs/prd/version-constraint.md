# Version Constraint Validation

## Overview

This document describes the version constraint validation feature for Atmos, which allows `atmos.yaml` configurations to specify required Atmos version ranges using semantic versioning (semver) constraints. When a configuration requires a specific version range, Atmos will validate the current version against the constraint and respond according to the configured enforcement level.

## Problem Statement

### Current State

Currently, Atmos provides version checking functionality under the `version` key in `atmos.yaml`:

```yaml
version:
  check:
    enabled: true
    timeout: 5
    frequency: "24h"
```

This configuration enables checking for newer versions of Atmos available on GitHub, but it does **not** enforce any minimum or maximum version requirements for the configuration itself.

### Challenges

1. **No minimum version enforcement** - Configurations using new features cannot require a minimum Atmos version
2. **Breaking changes** - Users may run incompatible Atmos versions with configurations expecting newer features
3. **Team consistency** - No way to ensure all team members use compatible Atmos versions
4. **Feature gating** - Cannot specify version ranges for configurations using version-specific features
5. **Migration clarity** - No clear signal when upgrading is required vs. recommended

## Proposed Solution

### YAML Configuration Structure

```yaml
# atmos.yaml
version:
  # EXISTING: Check for new releases on GitHub (unchanged)
  check:
    enabled: true
    timeout: 5
    frequency: "24h"

  # NEW: Validate Atmos version against constraints
  constraint:
    require: ">=2.5.0, <3.0.0"   # Semver constraint expression
    enforcement: "fatal"          # fatal | warn | silent (default: fatal)
    message: "Custom message"     # Optional custom error message
```

### Data Structure

Extend the existing `Version` struct in `pkg/schema/version.go`:

```go
type VersionConstraint struct {
    // Require specifies the semver constraint(s) for Atmos version as a single string.
    // Multiple constraints are comma-separated and treated as logical AND.
    // Uses hashicorp/go-version library (already in go.mod): https://github.com/hashicorp/go-version
    //
    // Why string instead of []string:
    //   - Consistent with Terraform constraint syntax
    //   - Simpler YAML (no list nesting)
    //   - Native to hashicorp/go-version (parses comma-separated directly)
    //   - Single atomic expression
    //
    // Examples:
    //   ">=1.2.3"                    - Minimum version
    //   "<2.0.0"                     - Maximum version (exclude)
    //   ">=1.2.3, <2.0.0"            - Range (AND logic)
    //   ">=2.5.0, !=2.7.0, <3.0.0"   - Complex (multiple AND constraints)
    //   "~>1.2"                      - Pessimistic constraint (>=1.2.0, <1.3.0)
    //   "~>1.2.3"                    - Pessimistic constraint (>=1.2.3, <1.3.0)
    //   "1.2.3"                      - Exact version
    Require string `yaml:"require,omitempty" mapstructure:"require" json:"require,omitempty"`

    // Enforcement specifies the behavior when version constraint is not satisfied.
    // Values:
    //   "fatal" - Exit immediately with error code 1 (default)
    //   "warn"  - Log warning but continue execution
    //   "silent" - Skip validation entirely (for debugging)
    Enforcement string `yaml:"enforcement,omitempty" mapstructure:"enforcement" json:"enforcement,omitempty"`

    // Message provides a custom message to display when constraint fails.
    // If empty, a default message is shown.
    Message string `yaml:"message,omitempty" mapstructure:"message" json:"message,omitempty"`
}

type Version struct {
    Check      VersionCheck      `yaml:"check,omitempty" mapstructure:"check" json:"check,omitempty"`
    Constraint VersionConstraint `yaml:"constraint,omitempty" mapstructure:"constraint" json:"constraint,omitempty"`
}
```

## Configuration Examples

### Example 1: Minimum Version (Fatal)
```yaml
version:
  constraint:
    require: ">=2.5.0"
    enforcement: "fatal"
```

**Behavior:**
- Current version `2.6.0` → Pass, execute normally
- Current version `2.4.0` → Exit with error:
  ```
  ✗ Atmos version constraint not satisfied
    Required: >=2.5.0
    Current:  2.4.0

  This configuration requires Atmos version >=2.5.0.
  Please upgrade: https://atmos.tools/install
  ```

### Example 2: Version Range with Custom Message
```yaml
version:
  constraint:
    require: ">=2.5.0, <3.0.0"
    enforcement: "warn"
    message: "This stack configuration is tested with Atmos 2.x. Atmos 3.x may introduce breaking changes."
```

**Behavior:**
- Current version `2.6.0` → Pass, execute normally
- Current version `3.0.0` → Show warning and continue:
  ```
  ⚠ Atmos version constraint not satisfied
    Required: >=2.5.0, <3.0.0
    Current:  3.0.0

  This stack configuration is tested with Atmos 2.x. Atmos 3.x may introduce breaking changes.
  ```

### Example 3: Pessimistic Constraint (Terraform-style)
```yaml
version:
  constraint:
    require: "~>2.5"  # Equivalent to ">=2.5.0, <2.6.0"
    enforcement: "fatal"
```

### Example 4: Multiple Constraints
```yaml
version:
  constraint:
    require: ">=2.5.0, !=2.7.0, <3.0.0"  # Allow 2.5.x and 2.6.x, skip broken 2.7.0
    enforcement: "fatal"
```

### Example 5: Silent Mode (Debugging)
```yaml
version:
  constraint:
    require: ">=2.5.0"
    enforcement: "silent"  # Skip validation, useful for testing
```

### Example 6: Team Consistency
```yaml
version:
  constraint:
    require: ">=2.5.0, <3.0.0"
    enforcement: "fatal"
    message: |
      Our team uses Atmos 2.x for this project.

      To install/upgrade Atmos:
      - macOS: brew install atmos
      - Linux: Download from https://github.com/cloudposse/atmos/releases

      Questions? Contact #infrastructure-support
```

### Example 7: Combined with Version Checking
```yaml
version:
  # Check for new Atmos releases periodically
  check:
    enabled: true
    timeout: 5
    frequency: "24h"

  # Require minimum version for this configuration
  constraint:
    require: ">=2.5.0"
    enforcement: "fatal"
```

## Constraint Syntax Reference

Using `hashicorp/go-version` constraint syntax (same as Terraform):

| Constraint | Meaning | Example |
|------------|---------|---------|
| `>=1.2.3` | Greater than or equal | `>=2.5.0` |
| `<=1.2.3` | Less than or equal | `<=3.0.0` |
| `>1.2.3` | Greater than (exclusive) | `>2.4.0` |
| `<1.2.3` | Less than (exclusive) | `<3.0.0` |
| `1.2.3` | Exact version | `2.5.0` |
| `!=1.2.3` | Not equal | `!=2.7.0` |
| `~>1.2` | Pessimistic (~> 1.2 = >=1.2.0, <1.3.0) | `~>2.5` |
| `~>1.2.3` | Pessimistic (~> 1.2.3 = >=1.2.3, <1.3.0) | `~>2.5.0` |
| Multiple | Comma-separated AND | `>=2.5.0, <3.0.0, !=2.7.0` |

Full syntax: https://github.com/hashicorp/go-version

## Enforcement Levels

| Level | Behavior | Use Case |
|-------|----------|----------|
| `fatal` (default) | Exit with error code 1 | Production configs requiring specific versions |
| `warn` | Show warning, continue execution | Migration periods, soft requirements |
| `silent` | Skip validation entirely | Debugging, testing with different versions |

## Environment Variable Override

Allow runtime override for debugging and CI/CD:

```bash
# Override enforcement level
ATMOS_VERSION_ENFORCEMENT=warn atmos terraform plan

# Disable constraint checking entirely
ATMOS_VERSION_ENFORCEMENT=silent atmos terraform plan
```

**Precedence:**
1. `ATMOS_VERSION_ENFORCEMENT` environment variable (highest)
2. `version.constraint.enforcement` in `atmos.yaml`
3. Default value `"fatal"` (lowest)

## Implementation Plan

### Phase 1: Core Validation

**File Changes:**

1. **`pkg/schema/version.go`** - Add `VersionConstraint` struct
   ```go
   type VersionConstraint struct {
       Require     string `yaml:"require,omitempty" mapstructure:"require" json:"require,omitempty"`
       Enforcement string `yaml:"enforcement,omitempty" mapstructure:"enforcement" json:"enforcement,omitempty"`
       Message     string `yaml:"message,omitempty" mapstructure:"message" json:"message,omitempty"`
   }

   type Version struct {
       Check      VersionCheck      `yaml:"check,omitempty" mapstructure:"check" json:"check,omitempty"`
       Constraint VersionConstraint `yaml:"constraint,omitempty" mapstructure:"constraint" json:"constraint,omitempty"`
   }
   ```

2. **`pkg/version/constraint.go`** (new file) - Validation logic
   ```go
   package version

   import (
       "fmt"

       goversion "github.com/hashicorp/go-version"
   )

   // ValidateConstraint checks if the current Atmos version satisfies the constraint.
   // Returns (satisfied bool, error).
   func ValidateConstraint(constraintStr string) (bool, error) {
       if constraintStr == "" {
           return true, nil // No constraint = always pass
       }

       current, err := goversion.NewVersion(Version)
       if err != nil {
           return false, fmt.Errorf("invalid current version %q: %w", Version, err)
       }

       constraints, err := goversion.NewConstraint(constraintStr)
       if err != nil {
           return false, fmt.Errorf("invalid version constraint %q: %w", constraintStr, err)
       }

       return constraints.Check(current), nil
   }
   ```

3. **`pkg/version/constraint_test.go`** (new file) - Comprehensive tests
   ```go
   package version

   import (
       "testing"
   )

   func TestValidateConstraint(t *testing.T) {
       // Save original version
       originalVersion := Version
       defer func() { Version = originalVersion }()

       tests := []struct {
           name           string
           currentVersion string
           constraint     string
           expectPass     bool
           expectError    bool
       }{
           {
               name:           "empty constraint always passes",
               currentVersion: "1.0.0",
               constraint:     "",
               expectPass:     true,
               expectError:    false,
           },
           {
               name:           "minimum version satisfied",
               currentVersion: "2.5.0",
               constraint:     ">=2.0.0",
               expectPass:     true,
               expectError:    false,
           },
           {
               name:           "minimum version not satisfied",
               currentVersion: "1.9.0",
               constraint:     ">=2.0.0",
               expectPass:     false,
               expectError:    false,
           },
           {
               name:           "range satisfied",
               currentVersion: "2.5.0",
               constraint:     ">=2.0.0, <3.0.0",
               expectPass:     true,
               expectError:    false,
           },
           {
               name:           "range not satisfied (too new)",
               currentVersion: "3.0.0",
               constraint:     ">=2.0.0, <3.0.0",
               expectPass:     false,
               expectError:    false,
           },
           {
               name:           "pessimistic constraint satisfied",
               currentVersion: "2.5.3",
               constraint:     "~>2.5",
               expectPass:     true,
               expectError:    false,
           },
           {
               name:           "pessimistic constraint not satisfied",
               currentVersion: "2.6.0",
               constraint:     "~>2.5",
               expectPass:     false,
               expectError:    false,
           },
           {
               name:           "exact version match",
               currentVersion: "2.5.0",
               constraint:     "2.5.0",
               expectPass:     true,
               expectError:    false,
           },
           {
               name:           "invalid constraint syntax",
               currentVersion: "2.5.0",
               constraint:     "invalid>>2.0",
               expectPass:     false,
               expectError:    true,
           },
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               Version = tt.currentVersion

               pass, err := ValidateConstraint(tt.constraint)

               if tt.expectError && err == nil {
                   t.Errorf("expected error but got none")
               }
               if !tt.expectError && err != nil {
                   t.Errorf("unexpected error: %v", err)
               }
               if pass != tt.expectPass {
                   t.Errorf("expected pass=%v, got pass=%v", tt.expectPass, pass)
               }
           })
       }
   }
   ```

4. **`cmd/cmd_utils.go`** - Add validation call in `initConfig()`
   ```go
   // Add after config is loaded, before any command execution
   func initConfig() error {
       // ... existing config loading code ...

       // Validate version constraint
       if err := validateVersionConstraint(atmosConfig); err != nil {
           return err
       }

       return nil
   }

   func validateVersionConstraint(cfg *schema.AtmosConfiguration) error {
       constraint := cfg.Version.Constraint

       // Skip if no constraint specified
       if constraint.Require == "" {
           return nil
       }

       // Check environment variable override
       enforcement := constraint.Enforcement
       if envEnforcement := os.Getenv("ATMOS_VERSION_ENFORCEMENT"); envEnforcement != "" {
           enforcement = envEnforcement
       }

       // Default enforcement is "fatal"
       if enforcement == "" {
           enforcement = "fatal"
       }

       // Skip validation if silent
       if enforcement == "silent" {
           return nil
       }

       // Validate constraint
       satisfied, err := version.ValidateConstraint(constraint.Require)
       if err != nil {
           // Invalid constraint syntax is always fatal
           return fmt.Errorf("invalid version constraint in configuration: %w", err)
       }

       if !satisfied {
           // Build error message
           msg := constraint.Message
           if msg == "" {
               msg = fmt.Sprintf(
                   "This configuration requires Atmos version %s.\nPlease upgrade: https://atmos.tools/install",
                   constraint.Require,
               )
           }

           fullMsg := fmt.Sprintf(
               "Atmos version constraint not satisfied\n  Required: %s\n  Current:  %s\n\n%s",
               constraint.Require,
               version.Version,
               msg,
           )

           if enforcement == "fatal" {
               return fmt.Errorf(fullMsg)
           } else if enforcement == "warn" {
               ui.Warning(fullMsg)
           }
       }

       return nil
   }
   ```

5. **`pkg/datafetcher/schema/atmos/1.0.json`** - Update JSON schema
   ```json
   {
     "version": {
       "type": "object",
       "properties": {
         "check": { ... },
         "constraint": {
           "type": "object",
           "properties": {
             "require": {
               "type": "string",
               "description": "Semver constraint for required Atmos version (e.g., '>=2.5.0', '>=2.0.0, <3.0.0')"
             },
             "enforcement": {
               "type": "string",
               "enum": ["fatal", "warn", "silent"],
               "default": "fatal",
               "description": "Enforcement level: 'fatal' exits with error, 'warn' shows warning, 'silent' skips validation"
             },
             "message": {
               "type": "string",
               "description": "Custom message to display when constraint fails"
             }
           }
         }
       }
     }
   }
   ```

### Phase 2: Documentation

1. **`website/docs/cli/configuration.mdx`** - Add version constraint documentation
2. **`website/docs/cli/versioning.mdx`** - Update with constraint examples

## Error Messages

**Fatal Error (enforcement: fatal):**
```
✗ Atmos version constraint not satisfied
  Required: >=2.5.0
  Current:  2.4.0

This configuration requires Atmos version >=2.5.0.
Please upgrade: https://atmos.tools/install

Error: version constraint validation failed
```

**Warning (enforcement: warn):**
```
⚠ Atmos version constraint not satisfied
  Required: >=2.5.0, <3.0.0
  Current:  3.0.0

This stack configuration is tested with Atmos 2.x. Atmos 3.x may introduce breaking changes.

Continuing execution...
```

**Invalid Constraint Syntax (always fatal):**
```
✗ Invalid version constraint in configuration
  Constraint: "invalid>>2.0"

Error: Malformed constraint: invalid>>2.0
Please use valid semver constraint syntax: https://github.com/hashicorp/go-version
```

## Validation Flow

```
1. Atmos CLI starts (any command)
   ↓
2. Load atmos.yaml configuration
   ↓
3. Parse version.constraint section
   ↓
4. Validate constraint (if specified)
   ├─ Empty? → Skip validation
   ├─ Invalid syntax? → Fatal error (cannot be bypassed)
   └─ Version mismatch?
      ├─ enforcement: "fatal" → Exit with error code 1
      ├─ enforcement: "warn" → Show warning, continue
      └─ enforcement: "silent" → Skip validation
   ↓
5. Continue with command execution
```

## Testing Strategy

1. **Unit tests** - `pkg/version/constraint_test.go` covers all constraint types
2. **Integration tests** - CLI tests with various configurations
3. **Error message tests** - Snapshot tests for all error/warning formats
4. **Environment variable tests** - Override behavior verification
5. **Edge cases** - Empty constraints, invalid syntax, malformed versions

Target: >80% coverage

## Backward Compatibility

- **Fully backward compatible** - No breaking changes
- Existing `atmos.yaml` files without `version.constraint` work unchanged
- Empty or missing `version.constraint.require` is treated as "no constraint"
- Default enforcement is "fatal" for safety, but can be explicitly set to "warn"

## Security Considerations

- Version validation occurs **after** config loading but **before** any execution
- Invalid constraint syntax is always fatal (cannot be bypassed)
- Environment variable override requires explicit action (not accidental)
- No remote version fetching required (offline-safe)

## Performance Considerations

- Version parsing is cached (hashicorp/go-version is efficient)
- Validation adds <1ms overhead
- No network calls required
- Early exit on fatal constraint violation

## Dependencies

### SemVer Library: `hashicorp/go-version`

**Already in Atmos dependencies** (`go.mod` v1.7.0) - currently used by `pkg/downloader/get_git.go` for git version checking.

**Why this library:**
- ✅ **Zero new dependencies** - Already in `go.mod`
- ✅ **Terraform-compatible syntax** - Users already familiar with constraint format
- ✅ **Battle-tested** - Used by HashiCorp in production tools
- ✅ **Complete feature set** - All operators: `>=`, `<=`, `>`, `<`, `=`, `!=`, `~>`
- ✅ **Efficient** - Fast parsing and comparison
- ✅ **Well-documented** - Clear API and examples

**Current usage in Atmos:**
```go
// pkg/downloader/get_git.go:330
want, err := version.NewVersion(min)
have, err := version.NewVersion(v)
if have.LessThan(want) {
    return fmt.Errorf("git version %s is too old", v)
}
```

**Proposed usage for constraints:**
```go
// pkg/version/constraint.go
current, err := goversion.NewVersion(Version)
constraints, err := goversion.NewConstraint(">=2.5.0, <3.0.0")
satisfied := constraints.Check(current)
```

### Constraint Format: String (Not List)

**Design decision: Use single string field with comma-separated constraints**

```yaml
# ✅ CHOSEN: String (comma-separated)
require: ">=2.5.0, <3.0.0, !=2.7.0"

# ❌ NOT CHOSEN: List of strings
# require:
#   - ">=2.5.0"
#   - "<3.0.0"
#   - "!=2.7.0"
```

**Rationale:**
1. **Terraform consistency** - Same syntax users already know
2. **Library native** - `hashicorp/go-version` parses comma-separated strings directly
3. **Simpler YAML** - No list nesting, cleaner configuration
4. **Atomic expression** - Single logical AND statement
5. **Less verbose** - Easier to read and write

**Library support:**
```go
// Single call handles all comma-separated constraints (AND logic)
constraints, err := version.NewConstraint(">=2.5.0, <3.0.0, !=2.7.0")
// Parses three constraints: >=2.5.0 AND <3.0.0 AND !=2.7.0
```

## Success Metrics

- ✅ Zero breaking changes for existing configurations
- ✅ >80% test coverage for constraint validation
- ✅ <5ms overhead for version validation
- ✅ Clear, actionable error messages
- ✅ Comprehensive documentation with examples
- ✅ User adoption in Cloud Posse reference architectures

## References

- [hashicorp/go-version](https://github.com/hashicorp/go-version) - Constraint syntax
- [Semantic Versioning](https://semver.org/) - Version format specification
- [Terraform Version Constraints](https://www.terraform.io/language/expressions/version-constraints) - Similar constraint syntax
- Existing Atmos version handling: `pkg/version/version.go`, `internal/exec/version.go`
