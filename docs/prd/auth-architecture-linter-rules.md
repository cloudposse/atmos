# Auth Architecture Linter Rules

## Overview

This document describes the linter rules enforced to maintain clean separation between provider-agnostic auth core and provider-specific implementations in the Atmos auth system.

## Problem Statement

The auth system uses a plugin-like architecture where core interfaces and orchestration logic must remain provider-agnostic, while provider-specific implementations (AWS, Azure, GCP, GitHub) are isolated in dedicated packages. Without enforcement, it's easy to accidentally introduce provider-specific dependencies into core packages, breaking this architectural separation.

## Architecture Overview

### Provider-Agnostic Core

These packages define interfaces and orchestration logic that work with **any** provider:

- **`pkg/auth/types/`** - Core interfaces (`Provider`, `Identity`, `AuthManager`, `CredentialStore`, etc.)
- **`pkg/auth/manager.go`** - Auth manager orchestration
- **`pkg/auth/factory/`** - Factory for creating provider/identity instances
- **`pkg/auth/credentials/`** - Multi-backend credential storage abstraction
- **`pkg/auth/validation/`** - Config validation
- **`pkg/auth/utils/`** - General utilities (env vars, duration parsing)
- **`pkg/auth/hooks.go`** - Auth hooks
- **`pkg/auth/list/`** - Identity listing and formatting

### Provider-Specific Implementations

These packages implement provider-specific logic and **may** import provider SDKs:

- **`pkg/auth/providers/aws/`** - AWS SSO and SAML providers
- **`pkg/auth/providers/github/`** - GitHub OIDC provider
- **`pkg/auth/providers/azure/`** - Azure providers (future)
- **`pkg/auth/providers/gcp/`** - GCP providers (future)
- **`pkg/auth/identities/aws/`** - AWS identities (permission-set, assume-role, user)
- **`pkg/auth/identities/azure/`** - Azure identities (future)
- **`pkg/auth/identities/gcp/`** - GCP identities (future)
- **`pkg/auth/cloud/aws/`** - AWS-specific utilities (console URLs, credential files, etc.)

### Special Cases

- **`pkg/auth/factory/`** - Routes to provider implementations; imports provider packages but not SDKs
- **`pkg/auth/types/aws_credentials.go`** - AWS-specific credential types (exported for use across auth system)
- **`pkg/auth/types/github_oidc_credentials.go`** - GitHub-specific credential types (exported for use across auth system)

## Linter Rules

### depguard: Import Control

The `depguard` linter enforces that provider-agnostic core packages cannot import provider SDKs.

#### Configuration

```yaml
depguard:
  rules:
    provider-agnostic-auth:
      files:
        - "$all"                                        # Apply to all files
        - "!**/pkg/auth/providers/**"                   # Except providers
        - "!**/pkg/auth/identities/**"                  # Except identities
        - "!**/pkg/auth/cloud/**"                       # Except cloud utilities
        - "!**/pkg/auth/factory/**"                     # Except factory
        - "!**/pkg/auth/types/aws_credentials.go"       # Except AWS types
        - "!**/pkg/auth/types/github_oidc_credentials.go" # Except GitHub types
        - "$test"                                       # Include test files
      deny:
        # AWS: Identity and auth-related SDKs
        - pkg: "github.com/aws/aws-sdk-go-v2"
          desc: "AWS SDK imports forbidden in provider-agnostic auth code; use pkg/auth/providers/aws/, pkg/auth/identities/aws/, or pkg/auth/cloud/aws/ for AWS-specific implementations"
        # Azure: Identity and auth SDKs (Entra ID, Azure AD)
        - pkg: "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
          desc: "Azure Identity SDK imports forbidden in provider-agnostic auth code; use pkg/auth/providers/azure/ or pkg/auth/identities/azure/ for Azure-specific implementations"
        - pkg: "github.com/AzureAD"
          desc: "Azure AD SDK imports forbidden in provider-agnostic auth code; use pkg/auth/providers/azure/ or pkg/auth/identities/azure/ for Azure-specific implementations"
        # GCP: Identity and auth SDKs (IAM, not general cloud services)
        - pkg: "cloud.google.com/go/iam"
          desc: "GCP IAM SDK imports forbidden in provider-agnostic auth code; use pkg/auth/providers/gcp/ or pkg/auth/identities/gcp/ for GCP-specific implementations"
        - pkg: "google.golang.org/api/iam"
          desc: "GCP IAM API imports forbidden in provider-agnostic auth code; use pkg/auth/providers/gcp/ or pkg/auth/identities/gcp/ for GCP-specific implementations"
        - pkg: "google.golang.org/api/iamcredentials"
          desc: "GCP IAM Credentials API imports forbidden in provider-agnostic auth code; use pkg/auth/providers/gcp/ or pkg/auth/identities/gcp/ for GCP-specific implementations"
        # GitHub: Identity SDKs
        - pkg: "github.com/google/go-github"
          desc: "GitHub SDK imports forbidden in provider-agnostic auth code; use pkg/auth/providers/github/ for GitHub-specific implementations"
```

**Note:** Rules focus on auth/identity SDKs only. General cloud service SDKs (BigQuery, Pub/Sub, S3, etc.) are not restricted as they're unrelated to authentication.

#### What It Catches

**Forbidden** (will fail lint):

```go
// In pkg/auth/types/interfaces.go
import "github.com/aws/aws-sdk-go-v2/aws" // ❌ FORBIDDEN - AWS SDK

// In pkg/auth/manager.go
import "github.com/Azure/azure-sdk-for-go/sdk/azidentity" // ❌ FORBIDDEN - Azure Identity SDK

// In pkg/auth/credentials/store.go
import "cloud.google.com/go/iam" // ❌ FORBIDDEN - GCP IAM SDK

// In pkg/auth/utils/env.go
import "github.com/AzureAD/microsoft-authentication-library-for-go" // ❌ FORBIDDEN - Azure AD SDK
```

**Allowed** (will pass lint):

```go
// In pkg/auth/providers/aws/sso.go
import "github.com/aws/aws-sdk-go-v2/service/sso" // ✅ ALLOWED

// In pkg/auth/identities/aws/permission_set.go
import "github.com/aws/aws-sdk-go-v2/service/sts" // ✅ ALLOWED

// In pkg/auth/cloud/aws/console.go
import "github.com/aws/aws-sdk-go-v2/aws" // ✅ ALLOWED

// In pkg/auth/types/aws_credentials.go
// No imports needed, just type definitions ✅ ALLOWED
```

### Testing the Rules

To verify the rules work:

```bash
# Test that core packages reject AWS SDK
echo 'package types
import "github.com/aws/aws-sdk-go-v2/aws"
func Test() {}' > pkg/auth/types/test.go

./custom-gcl run pkg/auth/types/test.go
# Expected: depguard error

# Test that provider packages allow AWS SDK
./custom-gcl run pkg/auth/providers/aws/sso.go
# Expected: no depguard errors (other errors OK)

# Clean up
rm pkg/auth/types/test.go
```

## Benefits

1. **Architectural Enforcement** - Prevents accidental coupling between core and providers
2. **Clear Separation** - Makes it obvious where provider-specific code belongs
3. **Extensibility** - Easy to add new providers without modifying core
4. **Testability** - Core can be tested with mocks without pulling in provider SDKs
5. **Maintainability** - Clear boundaries reduce cognitive load

## Adding New Providers

When adding a new provider (e.g., Okta, Auth0):

1. Create `pkg/auth/providers/okta/` for provider implementation
2. Create `pkg/auth/identities/okta/` if needed for identities
3. **No changes needed to depguard rules** - exclusions already cover `providers/` and `identities/`
4. Implement `types.Provider` and `types.Identity` interfaces
5. Register in `pkg/auth/factory/factory.go`.

The linter will automatically:
- ✅ Allow Okta SDK imports in `pkg/auth/providers/okta/`
- ❌ Reject Okta SDK imports in core packages

## Exceptions

### Factory Package

`pkg/auth/factory/` is excluded because it must import provider packages to instantiate them. However, it should **not** import provider SDKs directly - only the provider package interfaces.

**Correct:**

```go
// pkg/auth/factory/factory.go
import (
    awsProviders "github.com/cloudposse/atmos/pkg/auth/providers/aws"  // ✅ OK
    "github.com/cloudposse/atmos/pkg/auth/types"                        // ✅ OK
)

func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    switch config.Kind {
    case "aws/iam-identity-center":
        return awsProviders.NewSSOProvider(config) // ✅ OK - delegates to provider package
    }
}
```

**Incorrect:**

```go
// pkg/auth/factory/factory.go
import (
    "github.com/aws/aws-sdk-go-v2/service/sso" // ❌ FORBIDDEN - SDK import in factory
)
```

### Type Definition Files

`pkg/auth/types/aws_credentials.go` and `github_oidc_credentials.go` are excluded because they define provider-specific credential types that need to be exported and used across the auth system. These files should only contain type definitions, no SDK imports.

## Future Enhancements

1. **String Literal Detection** - Use forbidigo to detect hardcoded provider-specific strings (e.g., "aws", "iam", "sts") in core packages
2. **Interface Verification** - Ensure all providers implement required interfaces
3. **Documentation Enforcement** - Require godoc on all exported types in `types/` package

## References

- Command Registry Pattern: `docs/prd/command-registry-pattern.md`
- Error Handling Linter Rules: `docs/prd/error-handling-linter-rules.md`
- Testing Strategy: `docs/prd/testing-strategy.md`
- Interface-Driven Design: `CLAUDE.md` - "Architectural Patterns"
