# PRD: YAML Template Function Authentication in Terraform Commands

## Problem Statement

YAML template functions (`!terraform.state`, `!terraform.output`) fail to access authenticated AWS credentials when
using the `--identity` flag with terraform commands (`terraform plan`, `terraform apply`, etc.). This causes
authentication errors when these functions attempt to read Terraform state from S3.

### Current Behavior

When running:

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity core-identity/managers-team-access
```

If the component configuration uses YAML template functions:

```yaml
components:
  terraform:
    runs-on/cloudposse:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids
```

The command fails with authentication errors:

```
failed to read Terraform state for component vpc in stack core-ue2-auto
in YAML function: !terraform.state vpc vpc_id
failed to get object from S3: operation error S3: GetObject,
  exceeded maximum number of attempts, 3,
  get identity: get credentials: failed to refresh cached credentials,
  no EC2 IMDS role found,
  operation error ec2imds: GetMetadata, exceeded maximum number of attempts, 3,
  request send failed,
  Get "http://169.254.169.254/latest/meta-data/iam/security-credentials/":
    dial tcp 169.254.169.254:80: i/o timeout
```

### Root Cause

PR #1742 fixed authentication for `describe` commands but **missed the terraform command execution path**. The
authentication gap exists because:

1. **`ExecuteTerraform()` doesn't create AuthManager** from `info.Identity`
2. **`ProcessStacks()` doesn't accept AuthManager parameter**
3. **`ProcessComponentConfig()` doesn't accept AuthManager parameter**
4. **YAML functions evaluated during terraform execution have no AuthContext**

### Code Path Analysis

**Terraform Command Flow (BROKEN):**

```
cmd/terraform_utils.go:terraformRun()
  ↓ Parses --identity flag → info.Identity
  ↓
cmd/terraform_utils.go:126 → e.ExecuteTerraform(info)
  ↓
internal/exec/terraform.go:33 → ExecuteTerraform(info)
  ↓ No AuthManager created from info.Identity
  ↓
internal/exec/terraform.go:71 → ProcessStacks(&atmosConfig, info, ...)
  ↓ No AuthManager parameter
  ↓
internal/exec/utils.go:298 → ProcessStacks(...)
  ↓ No AuthManager parameter
  ↓
internal/exec/utils.go:343 → ProcessComponentConfig(...)
  ↓ No AuthManager parameter
  ↓
YAML functions evaluated WITHOUT AuthContext ❌
```

**Describe Command Flow (WORKING - Fixed in PR #1742):**

```
cmd/describe_component.go → describeComponentRun()
  ↓ Parses --identity flag
  ↓
internal/exec/describe_component.go → ExecuteDescribeComponent()
  ↓ Creates AuthManager from identity
  ↓
Threads AuthManager through component description pipeline
  ↓
YAML functions evaluated WITH AuthContext ✅
```

## Impact

### Severity

**High** - Blocks using `!terraform.state` and `!terraform.output` with authenticated credentials in production
multi-account AWS environments.

### Affected Use Cases

1. Multi-account AWS environments requiring role assumption
2. CI/CD pipelines using temporary credentials with `--identity` flag
3. Any terraform command that evaluates YAML template functions accessing AWS resources
4. Teams using `!terraform.state` for cross-component data lookup

### Commands Affected

All terraform execution commands when component config contains YAML template functions:

- `atmos terraform plan <component> -s <stack> --identity <identity>`
- `atmos terraform apply <component> -s <stack> --identity <identity>`
- `atmos terraform destroy <component> -s <stack> --identity <identity>`
- `atmos terraform import <component> -s <stack> --identity <identity>`
- `atmos terraform refresh <component> -s <stack> --identity <identity>`
- `atmos terraform workspace <component> -s <stack> --identity <identity>`
- `atmos terraform output <component> -s <stack> --identity <identity>`

### Commands NOT Affected

Commands that were fixed in PR #1742:

- `atmos describe component <component> -s <stack> --identity <identity>` ✅
- `atmos describe stacks --identity <identity>` ✅

## Solution

Thread AuthManager through the terraform command execution pipeline, following the same pattern established by PR #1742
for describe commands.

### Implementation Strategy

#### Phase 1: Update Function Signatures

**1. Update `ExecuteTerraform()` signature:**

```go
// Before:
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error

// After:
func ExecuteTerraform(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, authManager *auth.Manager) error
```

**2. Update `ProcessStacks()` signature:**

```go
// Before:
func ProcessStacks(
atmosConfig *schema.AtmosConfiguration,
configAndStacksInfo schema.ConfigAndStacksInfo,
checkStack bool,
processTemplates bool,
processYamlFunctions bool,
skip []string,
) (schema.ConfigAndStacksInfo, error)

// After:
func ProcessStacks(
atmosConfig *schema.AtmosConfiguration,
configAndStacksInfo schema.ConfigAndStacksInfo,
checkStack bool,
processTemplates bool,
processYamlFunctions bool,
skip []string,
authManager *auth.Manager,
) (schema.ConfigAndStacksInfo, error)
```

**3. Update `ProcessComponentConfig()` signature:**

```go
// Before:
func ProcessComponentConfig(
configAndStacksInfo *schema.ConfigAndStacksInfo,
stack string,
stacksMap map[string]any,
componentType string,
component string,
) error

// After:
func ProcessComponentConfig(
configAndStacksInfo *schema.ConfigAndStacksInfo,
stack string,
stacksMap map[string]any,
componentType string,
component string,
authManager *auth.Manager,
) error
```

#### Phase 2: Create AuthManager in ExecuteTerraform

```go
func ExecuteTerraform(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, authManager *auth.Manager) error {
defer perf.Track(nil, "exec.ExecuteTerraform")()

// If authManager not provided but identity specified, create it
if authManager == nil && info.Identity != "" {
var err error
authManager, err = auth.NewManager(atmosConfig, info.Identity)
if err != nil {
return fmt.Errorf("failed to create auth manager: %w", err)
}
}

// ... rest of function, passing authManager to ProcessStacks
}
```

#### Phase 3: Thread AuthManager Through Pipeline

Pass `authManager` parameter through:

1. `ExecuteTerraform()` → `ProcessStacks()`
2. `ProcessStacks()` → `ProcessComponentConfig()`
3. `ProcessComponentConfig()` → Store in `configAndStacksInfo` for YAML function evaluation

#### Phase 4: Update All Callers

**terraform_utils.go:**

```go
// Create AuthManager from info.Identity before calling ExecuteTerraform
var authManager *auth.Manager
if info.Identity != "" {
var err error
authManager, err = auth.NewManager(&atmosConfig, info.Identity)
if err != nil {
return err
}
}

err = e.ExecuteTerraform(&atmosConfig, info, authManager)
```

**All other callers of ProcessStacks and ProcessComponentConfig:**

- Update to pass `authManager` (or `nil` if not available)
- Search for all callers: `grep -r "ProcessStacks\|ProcessComponentConfig" --include="*.go"`

#### Phase 5: Enable YAML Functions to Use AuthContext

Ensure YAML function evaluation code (in `internal/exec/yaml_func_*.go`) has access to AuthContext from the AuthManager.
This was already done in PR #1742, so just verify it works with the terraform command path.

### Testing Strategy

#### Unit Tests

Create test in `internal/exec/terraform_test.go`:

```go
func TestExecuteTerraform_WithIdentityAndYAMLFunctions(t *testing.T) {
// Test that --identity flag works with !terraform.state functions
// Mock AuthManager to verify it's called correctly
// Verify YAML functions receive AuthContext
}
```

Create test in `internal/exec/utils_test.go`:

```go
func TestProcessStacks_WithAuthManager(t *testing.T) {
// Test that ProcessStacks threads AuthManager correctly
// Verify ProcessComponentConfig receives it
}

func TestProcessComponentConfig_WithAuthManager(t *testing.T) {
// Test that ProcessComponentConfig uses AuthManager
// Verify it's available during YAML function evaluation
}
```

#### Integration Tests

Create test in `tests/cli_terraform_yaml_functions_auth_test.go`:

```go
func TestTerraformPlanWithIdentityAndYAMLFunctions(t *testing.T) {
// End-to-end test with real terraform command
// Component config with !terraform.state functions
// Verify --identity flag provides credentials
// Verify YAML functions can access S3
}
```

### Files to Modify

**Core Implementation:**

- `internal/exec/terraform.go` - Add AuthManager parameter, create from identity
- `internal/exec/utils.go` - Add AuthManager to ProcessStacks and ProcessComponentConfig
- `cmd/terraform_utils.go` - Create AuthManager before calling ExecuteTerraform

**Other Terraform Commands:**

- `internal/exec/terraform_generate_backend.go` - Update ProcessStacks call
- `internal/exec/terraform_generate_backends.go` - Update ProcessStacks call
- `internal/exec/terraform_generate_varfile.go` - Update ProcessStacks call
- `internal/exec/terraform_generate_varfiles.go` - Update ProcessStacks call
- `internal/exec/terraform_generate_planfile.go` - Update ProcessStacks call
- `internal/exec/terraform_query.go` - Update ProcessStacks call
- `internal/exec/terraform_affected.go` - Update ProcessStacks call

**Stacks Processor (if used):**

- `internal/exec/stacks_processor.go` - Update DefaultStacksProcessor to accept AuthManager

**Tests:**

- `internal/exec/terraform_test.go` - Add authentication tests
- `internal/exec/utils_test.go` - Add ProcessStacks/ProcessComponentConfig tests
- `tests/cli_terraform_yaml_functions_auth_test.go` - Add integration test

### Backward Compatibility

All changes maintain backward compatibility:

- AuthManager parameter is optional (`*auth.Manager` can be `nil`)
- Existing callers without identity continue to work
- YAML functions gracefully fall back to AWS SDK default credential chain when no AuthManager

## Success Criteria

1. ✅ `atmos terraform plan <component> -s <stack> --identity <identity>` works with `!terraform.state` functions
2. ✅ `!terraform.state` and `!terraform.output` use authenticated credentials from `--identity`
3. ✅ No AWS IMDS timeout errors when using `--identity` flag
4. ✅ Multi-account role assumption works correctly
5. ✅ All existing terraform commands continue to work without regression
6. ✅ Unit and integration tests pass
7. ✅ Manual testing in `infra-live` repository confirms fix

## References

- **Bug Report:** https://github.com/cloudposse/infra-live/pull/1640
- **Related Fix (PR #1742):** https://github.com/cloudposse/atmos/pull/1742 - Fixed describe commands but missed
  terraform commands
- **Slack Discussion:** https://cloudposse.slack.com/archives/C02EC1YLTV4/p1762020638565989
- **Error Analysis:** infra-live PR #1640 demonstrates the authentication failure
- **Architecture:** Authentication system in `pkg/auth/`, YAML functions in `internal/exec/yaml_func_*.go`

## Timeline

- **Phase 1 (Investigation):** ✅ Complete - Issue identified and PRD written
- **Phase 2 (Implementation):** Update function signatures and thread AuthManager (2-3 hours)
- **Phase 3 (Testing):** Add unit and integration tests (1-2 hours)
- **Phase 4 (Verification):** Manual testing in infra-live (30 minutes)
- **Phase 5 (Documentation):** Update CLI docs if needed (30 minutes)

**Total Estimated Time:** 4-6 hours
