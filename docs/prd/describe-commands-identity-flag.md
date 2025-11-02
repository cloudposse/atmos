# PRD: Add --identity Flag to Describe Commands

## Executive Summary

Add `--identity` flag support to all `atmos describe` subcommands to enable authentication when processing YAML template functions (`!terraform.state`, `!terraform.output`) that require cloud provider credentials. This ensures feature parity with `terraform` and `workflow` commands and eliminates authentication failures in describe operations.

## Problem Statement

### Current Limitation

The `atmos describe` family of commands processes stack configurations that may contain YAML template functions requiring authenticated access to cloud resources:

- `!terraform.state` - Fetches Terraform state from remote backends (S3, Azure Blob, GCS)
- `!terraform.output` - Retrieves Terraform outputs from remote state

When these functions execute without proper authentication context, they fail with timeout errors (e.g., "context deadline exceeded" when falling back to EC2 Instance Metadata Service).

### Affected Commands

All describe commands execute YAML functions (`!terraform.state`, `!terraform.output`) and Go templates by default during stack processing, unless disabled with `--process-functions=false` or `--process-templates=false` flags.

1. **`atmos describe stacks`** - Processes all stacks, may execute template functions across many components
2. **`atmos describe component <component> -s <stack>`** - Processes single component configuration
3. **`atmos describe affected`** - Processes affected components after changes
4. **`atmos describe dependents <component> -s <stack>`** - Processes component dependency tree

### User Impact

**Failure Scenario:**
```bash
# Stack configuration contains YAML function
# stacks/core.yaml:
# components:
#   terraform:
#     runs-on:
#       vars:
#         vpc_id: !terraform.output vpc.vpc_id

# Command fails with timeout
$ atmos describe component runs-on -s core-use2-auto
Error: context deadline exceeded (accessing S3 without credentials)
```

**Current Workaround:**
Users must:
1. Manually run `atmos auth login --identity <identity>` before describe commands
2. Rely on ambient AWS credentials (environment variables, shared credentials file)
3. Use EC2 instance profiles (not applicable for local development)

**Desired Behavior:**
```bash
# Runtime authentication with describe commands
$ atmos describe component runs-on -s core-use2-auto --identity core-auto/terraform
# Works - credentials obtained and used for !terraform.output function
```

## Goals

### Primary Goals

1. **Add `--identity` flag to all describe commands** - Enable runtime authentication for describe operations
2. **Support interactive identity selection** - Allow `--identity` without value for user selection
3. **Maintain consistency with terraform commands** - Use same flag behavior and shorthand (`-i`)
4. **Preserve backward compatibility** - Describe commands continue to work without `--identity` flag when ambient credentials are available

### Secondary Goals

1. **Comprehensive test coverage** - Unit and integration tests for all describe commands with identity
2. **Documentation** - Update all describe command documentation with identity flag examples
3. **Shell completion** - Enable identity auto-completion for describe commands

## Solution Overview

### Implementation Approach

Add `--identity` as a **PersistentFlag** to the `describeCmd` parent command, which will automatically inherit to all subcommands:

```go
// cmd/describe.go
func init() {
    describeCmd.PersistentFlags().StringP("query", "q", "", "...")

    // Add --identity flag to all describe commands
    describeCmd.PersistentFlags().StringP("identity", "i", "",
        "Specify the identity to authenticate to before describing. Use without value to interactively select.")

    // Enable optional flag value for interactive selection
    if identityFlag := describeCmd.PersistentFlags().Lookup("identity"); identityFlag != nil {
        identityFlag.NoOptDefVal = IdentityFlagSelectValue
    }

    // Add shell completion
    AddIdentityCompletion(describeCmd)

    RootCmd.AddCommand(describeCmd)
}
```

### Authentication Flow

```
┌─────────────────────────────────────────────────────────────┐
│ User runs: atmos describe component vpc -s core --identity │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 1. Parse --identity flag from command line                 │
│    - Get identity name from flag                            │
│    - Handle interactive selection if flag has no value      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Create AuthManager (if identity provided)               │
│    - Create stackInfo with empty AuthContext                │
│    - Initialize AuthManager with auth config                │
│    - Call AuthManager.Authenticate(identityName)            │
│    - AuthContext now populated with AWS credentials         │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Pass AuthManager to execution function                  │
│    - ExecuteDescribeStacks(params)                          │
│    - ExecuteDescribeComponent(params)                       │
│    - ExecuteDescribeAffected(params)                        │
│    - ExecuteDescribeDependents(params)                      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. AuthContext propagates to YAML functions                │
│    - configAndStacksInfo.AuthContext = authManager.GetStackInfo().AuthContext │
│    - !terraform.state uses AuthContext for AWS SDK config   │
│    - !terraform.output uses AuthContext for S3 access       │
└─────────────────────────────────────────────────────────────┘
```

## Technical Implementation

### 1. Parent Command Flag Addition

**File:** `cmd/describe.go`

Add `--identity` as PersistentFlag to inherit to all subcommands.

### 2. Update Describe Subcommands

Each describe subcommand needs to:

#### `atmos describe stacks`
**File:** `cmd/describe_stacks.go`

```go
func executeDescribeStacksCmd(cmd *cobra.Command, args []string) error {
    // ... existing code ...

    // Get identity from flag
    identityName := GetIdentityFromFlags(cmd, os.Args)

    var authManager auth.AuthManager
    if identityName != "" {
        authStackInfo := &schema.ConfigAndStacksInfo{
            AuthContext: &schema.AuthContext{},
        }
        credStore := credentials.NewCredentialStore()
        validator := validation.NewValidator()
        authManager, err = auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo)
        if err != nil {
            return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
        }

        // Handle interactive selection
        forceSelect := identityName == IdentityFlagSelectValue
        if identityName == "" || forceSelect {
            identityName, err = getDefaultIdentity(cmd, authManager, forceSelect)
            if err != nil {
                return err
            }
        }

        // Authenticate
        if err := authManager.Authenticate(context.Background(), identityName); err != nil {
            return err
        }
    }

    // Pass to execution
    stacks, err := e.ExecuteDescribeStacks(&e.ExecuteDescribeStacksParams{
        // ... existing params ...
        AuthManager: authManager,
    })
    // ... rest of function ...
}
```

#### `atmos describe component`
**File:** `cmd/describe_component.go`

Similar pattern - extract identity, create AuthManager, authenticate, pass to `ExecuteDescribeComponent` (already supports AuthManager parameter).

#### `atmos describe affected`
**File:** `cmd/describe_affected.go`

Add identity handling and pass AuthManager to `ExecuteDescribeAffected`.

#### `atmos describe dependents`
**File:** `cmd/describe_dependents.go`

Add identity handling and pass AuthManager to `ExecuteDescribeDependents`.

### 3. Update Internal Execution Functions

Some execution functions may need AuthManager parameter added:

#### `ExecuteDescribeStacks`
**File:** `internal/exec/describe_stacks.go`

Add AuthManager to function signature or create parameter struct (similar to `ExecuteDescribeComponentParams`).

#### `ExecuteDescribeAffected`
**File:** `internal/exec/describe_affected.go`

Add AuthManager parameter and propagate to underlying component processing.

#### `ExecuteDescribeDependents`
**File:** `internal/exec/describe_dependents.go`

Add AuthManager parameter and propagate to component processing.

### 4. AuthContext Propagation

Ensure all execution paths extract AuthContext from AuthManager:

```go
if params.AuthManager != nil {
    managerStackInfo := params.AuthManager.GetStackInfo()
    if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
        configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
        log.Debug("Populated AuthContext from AuthManager for template functions")
    }
}
```

## Testing Strategy

### Unit Tests

#### Test Identity Flag Parsing
- Test `--identity <name>` with explicit identity
- Test `--identity` without value (interactive selection)
- Test `-i <name>` shorthand
- Test `-i` shorthand interactive
- Test missing identity flag (nil AuthManager)

#### Test AuthManager Integration
- Test AuthManager creation when identity provided
- Test authentication is called with correct identity
- Test AuthContext propagation to execution functions
- Test nil AuthManager when no identity provided

### Integration Tests

#### `describe_stacks_identity_test.go`
```go
func TestDescribeStacksWithIdentity(t *testing.T) {
    // Test that describe stacks with --identity flag
    // properly authenticates and can process YAML functions
}

func TestDescribeStacksWithoutIdentity(t *testing.T) {
    // Test backward compatibility - works without identity
}

func TestDescribeStacksInteractiveIdentity(t *testing.T) {
    // Test interactive identity selection
}
```

#### `describe_component_identity_test.go`
```go
func TestDescribeComponentWithIdentity(t *testing.T) {
    // Test component description with identity
}
```

#### `describe_affected_identity_test.go`
```go
func TestDescribeAffectedWithIdentity(t *testing.T) {
    // Test affected components with identity
}
```

#### `describe_dependents_identity_test.go`
```go
func TestDescribeDependentsWithIdentity(t *testing.T) {
    // Test dependents with identity
}
```

### Test Fixtures

Reuse existing test fixture:
- `tests/fixtures/scenarios/authmanager-propagation/`

Create additional fixture if needed:
- `tests/fixtures/scenarios/describe-with-identity/`
- Include stacks with `!terraform.state` and `!terraform.output` functions
- Include multiple components with dependencies

### CLI Tests

Add CLI tests in `tests/cli_test.go`:

```bash
# Test describe stacks with identity
atmos describe stacks --identity test-identity

# Test describe component with identity
atmos describe component vpc -s test-stack --identity test-identity

# Test describe affected with identity
atmos describe affected --identity test-identity

# Test describe dependents with identity
atmos describe dependents vpc -s test-stack --identity test-identity
```

## Documentation Updates

### Command Documentation

Update the following Docusaurus pages:

1. **`website/docs/cli/commands/describe/atmos_describe_stacks.md`**
   - Add `--identity` flag to flags table
   - Add usage examples with identity
   - Document interactive selection

2. **`website/docs/cli/commands/describe/atmos_describe_component.md`**
   - Add `--identity` flag documentation
   - Add examples

3. **`website/docs/cli/commands/describe/atmos_describe_affected.md`**
   - Add `--identity` flag documentation
   - Add examples with identity for CI/CD scenarios

4. **`website/docs/cli/commands/describe/atmos_describe_dependents.md`**
   - Add `--identity` flag documentation
   - Add examples

### Usage Examples

Add to each command's documentation:

```markdown
## Using with Authentication

When your stack configurations use YAML template functions that access remote state (e.g., `!terraform.state`, `!terraform.output`), you may need to authenticate:

### Explicit Identity
```bash
atmos describe stacks --identity core-auto/terraform
```

### Interactive Identity Selection
```bash
atmos describe stacks --identity
# Prompts to select from configured identities
```

### Short Form
```bash
atmos describe component vpc -s core-use2-auto -i core-auto/terraform
```
```

## Migration Guide

### For End Users

**No breaking changes** - existing commands continue to work:

```bash
# Still works (uses ambient credentials)
atmos describe stacks

# New capability (runtime authentication)
atmos describe stacks --identity my-identity
```

### For CI/CD Pipelines

Recommended pattern for CI/CD:

```bash
# Before (required ambient AWS credentials)
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
atmos describe affected

# After (use Atmos identity management)
atmos describe affected --identity ci-automation
```

### For Developers

No code changes required unless customizing describe command behavior.

## Performance Considerations

### Authentication Overhead

- AuthManager creation: ~10ms
- Authentication call: ~100-500ms (depends on provider: AWS SSO, Vault, OIDC)
- Credential caching: Subsequent operations reuse cached credentials

### Optimization

- Credentials cached in AuthManager for duration of command
- Multiple YAML function calls reuse same AuthContext
- No additional authentication per component

## Security Considerations

### Credential Handling

- Credentials never logged or printed to stdout
- AuthContext passed by reference (not copied)
- Credentials cleared when AuthManager is garbage collected

### Identity Validation

- Identity name validated against configured identities in `atmos.yaml`
- Invalid identity names fail early with clear error message
- Interactive selection only shows valid, configured identities

### Audit Trail

- Authentication events logged at Debug level
- Successful authentication logged with identity name (not credentials)
- Failed authentication logged with error reason

## Success Criteria

### Functional Requirements

- ✅ `--identity` flag added to all describe commands
- ✅ Interactive identity selection works (`--identity` without value)
- ✅ Shorthand `-i` supported
- ✅ AuthManager created and authenticated when identity provided
- ✅ AuthContext propagates to YAML template functions
- ✅ Backward compatible (works without `--identity`)

### Testing Requirements

- ✅ Unit tests for all describe commands with identity flag
- ✅ Integration tests with mock AuthManager
- ✅ CLI tests with actual identity usage
- ✅ Test coverage >80% for new code

### Documentation Requirements

- ✅ All describe command docs updated with `--identity` flag
- ✅ Usage examples added for each command
- ✅ Migration guide provided

## Implementation Checklist

### Phase 1: Core Implementation
- [ ] Add `--identity` PersistentFlag to `cmd/describe.go`
- [ ] Update `cmd/describe_stacks.go` with identity handling
- [ ] Update `cmd/describe_component.go` with identity handling
- [ ] Update `cmd/describe_affected.go` with identity handling
- [ ] Update `cmd/describe_dependents.go` with identity handling

### Phase 2: Internal Functions
- [ ] Add AuthManager parameter to `ExecuteDescribeStacks`
- [ ] Add AuthManager parameter to `ExecuteDescribeAffected`
- [ ] Add AuthManager parameter to `ExecuteDescribeDependents`
- [ ] Ensure AuthContext propagation in all execution paths

### Phase 3: Testing
- [ ] Create `describe_stacks_identity_test.go`
- [ ] Create `describe_component_identity_test.go`
- [ ] Create `describe_affected_identity_test.go`
- [ ] Create `describe_dependents_identity_test.go`
- [ ] Add CLI tests for all describe commands with identity
- [ ] Verify test coverage >80%

### Phase 4: Documentation
- [ ] Update `atmos_describe_stacks.md`
- [ ] Update `atmos_describe_component.md`
- [ ] Update `atmos_describe_affected.md`
- [ ] Update `atmos_describe_dependents.md`
- [ ] Add usage examples to each doc
- [ ] Build and verify documentation locally

### Phase 5: Validation
- [ ] Run full test suite
- [ ] Run linter (golangci-lint)
- [ ] Manual testing with real AWS credentials
- [ ] Verify backward compatibility (commands work without identity)
- [ ] Code review

## Future Enhancements

### Environment Variable Support

Consider adding `ATMOS_IDENTITY` environment variable support for describe commands (similar to auth commands):

```go
if err := viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY"); err != nil {
    log.Trace("Failed to bind identity environment variables", "error", err)
}
```

### Default Identity

Consider supporting default identity in `atmos.yaml`:

```yaml
auth:
  default_identity: core-auto/terraform
```

### Identity Caching

Consider caching authenticated identity across multiple describe command invocations within a session.

## References

- **Related PRD:** `docs/prd/terraform-template-functions-auth-context.md` - AuthContext propagation fix
- **Implementation:** `cmd/auth.go` - Auth command identity flag implementation
- **Implementation:** `cmd/terraform_commands.go` - Terraform command identity flag implementation
- **Implementation:** `internal/exec/describe_component.go` - ExecuteDescribeComponent with AuthManager

## Changelog

| Date | Version | Changes | Author |
|------|---------|---------|--------|
| 2025-11-02 | 1.0 | Initial PRD created | Claude Code |
