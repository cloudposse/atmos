# Product Requirements Document: YAML Function Authentication in Terraform Commands

**Status:** ✅ COMPLETE
**Implementation Date:** November 7, 2025

## Executive Summary

This PRD defines the expected behavior for Atmos authentication when using YAML functions (`!terraform.state`, `!terraform.output`) with Terraform commands executed under the `--identity` flag. YAML functions should be able to seamlessly access authenticated AWS credentials in all Terraform command contexts, including plan, apply, and destroy. This ensures consistent cross-component data lookups across production multi-account AWS environments.

To achieve this, the design extends the AuthManager throughout the Terraform command execution pipeline, enabling YAML functions to retrieve authenticated credentials for reading Terraform state from S3 and interacting with other cloud resources. The implementation maintains backward compatibility while ensuring full support for role assumption and authenticated data resolution in multi-account setups.

## 1. Problem Statement

### Current State

YAML functions fail authentication when evaluating during terraform command execution:

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity core-identity/managers-team-access
```

With component configuration using YAML functions:

```yaml
components:
  terraform:
    runs-on/cloudposse:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids
```

Results in authentication failures:

```
failed to read Terraform state for component vpc in stack core-ue2-auto
failed to get object from S3: operation error S3: GetObject,
  exceeded maximum number of attempts, 3,
  no EC2 IMDS role found, dial tcp 169.254.169.254:80: i/o timeout
```

### Root Cause

The terraform command execution path lacks authentication infrastructure:

1. `ExecuteTerraform()` doesn't create AuthManager from `info.Identity`
2. `ProcessStacks()` doesn't accept AuthManager parameter
3. `ProcessComponentConfig()` doesn't accept AuthManager parameter
4. YAML functions evaluate without AuthContext

### Impact

**Severity:** High - Blocks production use of `!terraform.state` and `!terraform.output` in multi-account AWS environments.

**Affected Use Cases:**
- Multi-account AWS environments requiring role assumption
- CI/CD pipelines using temporary credentials with `--identity` flag
- Cross-component data lookup in production environments
- Teams using YAML functions for infrastructure dependencies

**Commands Affected:**
- All terraform execution commands: `plan`, `apply`, `destroy`, `import`, `refresh`, `workspace`, `output`

## 2. Requirements

### 2.1 Functional Requirements

**FR-001: Thread AuthManager Through Terraform Pipeline**
- Update `ExecuteTerraform()` to create AuthManager from `info.Identity`
- Update `ProcessStacks()` to accept optional AuthManager parameter
- Update `ProcessComponentConfig()` to accept optional AuthManager parameter
- Populate AuthContext in ConfigAndStacksInfo for YAML function access

**FR-002: Shared Authentication Helper**
- Create `auth.CreateAndAuthenticateManager()` to eliminate code duplication
- Support identity name, auth config, and select value parameters
- Return authenticated AuthManager ready for use

**FR-003: Backward Compatibility**
- Commands without `--identity` flag continue to work unchanged
- AuthManager parameter is optional (nil-safe)
- Existing callers pass nil without breakage

**FR-004: Auto-Detection of Default Identity** *(Enhancement)*
- Automatically detect default identity from global/stack configuration
- Use default identity when no `--identity` flag provided
- Respect `default: true` flag in identity configuration

**FR-005: Interactive Identity Selection** *(Enhancement)*
- Prompt user to select identity when no defaults configured
- Only in TTY mode (disable in CI environments)
- Store selected identity to prevent double-prompting from hooks

**FR-006: Explicit Authentication Disable** *(Enhancement)*
- Support `--identity=off` to disable Atmos Auth
- Allow use of external mechanisms (Leapp, env vars, IMDS)

### 2.2 Non-Functional Requirements

**NFR-001: Code Quality**
- Eliminate duplicate authentication code
- Centralize authentication logic in `pkg/auth/`
- Follow existing authentication patterns from PR #1742

**NFR-002: Testing**
- Unit tests for all new functions
- Integration tests for terraform commands with YAML functions
- Manual testing in production-like multi-account setup

**NFR-003: Performance**
- No measurable performance impact for commands without authentication
- Single authentication per command execution

## 3. Solution Architecture

### 3.1 Design Overview

Thread AuthManager through the terraform execution pipeline following the pattern established by PR #1742 for `describe` commands:

```
terraform command → ExecuteTerraform() → ProcessStacks() → ProcessComponentConfig() → YAML functions
                         ↓                      ↓                    ↓                      ↓
                    Create AuthManager    Pass AuthManager    Populate AuthContext    Use AuthContext
```

### 3.2 Approach

**Phase 1: Update Function Signatures**
- Add AuthManager parameter to `ProcessStacks()` and `ProcessComponentConfig()`
- Make parameter optional (nil-safe) for backward compatibility

**Phase 2: Create Shared Helper**
- Implement `auth.CreateAndAuthenticateManager()` in `pkg/auth/manager_helpers.go`
- Centralize authentication logic to eliminate duplication
- Handle identity selection, authentication, and error cases

**Phase 3: Thread AuthManager**
- Create AuthManager in `ExecuteTerraform()` from `info.Identity`
- Pass through `ProcessStacks()` to `ProcessComponentConfig()`
- Populate AuthContext for YAML function evaluation

**Phase 4: Update All Callers**
- Update all `ProcessStacks()` callers to pass `nil` authManager
- Maintain backward compatibility for commands without authentication

### 3.3 Code Path Changes

**Before (Broken):**
```
cmd/terraform_utils.go → ExecuteTerraform() → ProcessStacks() → ProcessComponentConfig()
                                                                        ↓
                                                            YAML functions WITHOUT AuthContext ❌
```

**After (Fixed):**
```
cmd/terraform_utils.go → ExecuteTerraform() → ProcessStacks() → ProcessComponentConfig()
         ↓ (--identity)         ↓                   ↓                    ↓
    Parse identity      Create & Auth       Pass AuthManager    Populate AuthContext
                          AuthManager                                    ↓
                                                            YAML functions WITH AuthContext ✅
```

## 4. Implementation Details

### 4.1 Files Modified

**New Files:**
- `pkg/auth/manager_helpers.go` - Shared authentication helper (+74 lines)
- `pkg/auth/manager_helpers_test.go` - Unit tests

**Core Implementation:**
- `internal/exec/terraform.go` - Create and thread AuthManager
- `internal/exec/utils.go` - Add AuthManager parameter to ProcessStacks and ProcessComponentConfig
- `cmd/identity_flag.go` - Refactor to use shared helper

**ProcessStacks Callers Updated:**
- `internal/exec/aws_eks_update_kubeconfig.go`
- `internal/exec/helmfile.go`
- `internal/exec/helmfile_generate_varfile.go`
- `internal/exec/packer.go`
- `internal/exec/terraform_generate_backend.go`
- `internal/exec/terraform_generate_planfile.go`
- `internal/exec/terraform_generate_varfile.go`
- `internal/exec/describe_component.go`

### 4.2 Critical Implementation Details

**Shared Authentication Helper:**

```go
func CreateAndAuthenticateManager(
	identityName string,
	authConfig *schema.AuthConfig,
	selectValue string,
) (AuthManager, error) {
	// Handle explicit disable
	if identityName == cfg.IdentityFlagDisabledValue {
		return nil, nil
	}

	// Auto-detect default identity if not provided
	if identityName == "" {
		if authConfig == nil || len(authConfig.Identities) == 0 {
			return nil, nil // Backward compatible
		}
		identityName = autoDetectDefaultIdentity(authConfig)
		if identityName == "" {
			return nil, nil // No default found
		}
	}

	// Create AuthManager
	authManager, err := NewAuthManager(authConfig, ...)
	if err != nil {
		return nil, err
	}

	// CRITICAL: Authenticate to populate AuthContext
	_, err = authManager.Authenticate(context.Background(), identityName)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}
```

**Key Insight Discovered During Implementation:**

Creating AuthManager without calling `.Authenticate()` leaves AuthContext empty, causing AWS SDK to fall back to IMDS (resulting in timeout errors). The `.Authenticate()` call is **critical** for populating credentials.

### 4.3 Testing

**Unit Tests:**
- `pkg/auth/manager_helpers_test.go` - 19 test cases covering all scenarios

**Integration Tests:**
- `tests/test-cases/auth-merge.yaml` - End-to-end CLI testing

**Manual Testing:**
- Verified in infra-live repository with real multi-account AWS setup
- Confirmed YAML functions access S3 state using authenticated credentials
- No IMDS timeout errors observed

## 5. Success Criteria

All success criteria achieved:

1. ✅ `atmos terraform plan <component> -s <stack> --identity <identity>` works with `!terraform.state` functions
2. ✅ `!terraform.state` and `!terraform.output` use authenticated credentials from `--identity` flag
3. ✅ No AWS IMDS timeout errors when using `--identity` flag
4. ✅ Multi-account role assumption works correctly
5. ✅ All existing terraform commands continue to work without regression
6. ✅ Unit and integration tests pass
7. ✅ Manual testing in production-like environment confirms fix

**Additional Achievements:**
- ✅ Code refactoring eliminates ~90 lines of duplicate code
- ✅ Comprehensive audit found no other instances of "create but not authenticate" bug
- ✅ Architectural improvements (no reverse dependencies)

## 6. References

- **Bug Report:** https://github.com/cloudposse/infra-live/pull/1640
- **Related Fix (PR #1742):** https://github.com/cloudposse/atmos/pull/1742 - Fixed describe commands
- **Authentication Architecture:** `pkg/auth/`, YAML functions in `internal/exec/yaml_func_*.go`

## 7. Implementation Timeline

- **Phase 1 (Investigation):** ✅ Complete - Issue identified and PRD written
- **Phase 2 (Implementation):** ✅ Complete - Function signatures updated, AuthManager threaded
- **Phase 3 (Bug Discovery):** ✅ Complete - Found and fixed missing authentication call
- **Phase 4 (Refactoring):** ✅ Complete - Consolidated duplicate helper functions
- **Phase 5 (Audit):** ✅ Complete - Verified no other instances of the bug
- **Phase 6 (Testing):** ✅ Complete - Manual testing in infra-live confirmed fix
- **Phase 7 (Verification):** ✅ Complete - Build, tests, and code quality checks pass

## 8. Enhancements (November 10, 2025)

Following initial implementation, enhanced with flexible identity resolution:

**8.1 Auto-Detection of Default Identity**
- Automatically detects default identity from global/stack configuration
- Eliminates need to specify `--identity` on every command

**8.2 Interactive Identity Selection**
- Prompts user once when no defaults configured (TTY mode only)
- Stores selected identity to prevent double-prompting from hooks

**8.3 Identity Storage for Hooks**
- After authentication, stores authenticated identity in `info.Identity`
- Prevents hooks like `TerraformPreHook` from prompting again

**8.4 CI/CD Mode Support**
- Auto-detects CI environments via `telemetry.IsCI()`
- Gracefully falls back to no authentication in non-interactive mode

**8.5 Explicit Auth Disable**
- Support for `--identity=off` to use external mechanisms
- Enables use of Leapp, environment variables, or IMDS

**Benefits:**
- Improved UX - No `--identity` flag needed when default configured
- Single authentication per command - No repeated prompts
- CI/CD friendly - Auto-detects non-interactive environments
- Flexible - Works with both Atmos Auth and external mechanisms
