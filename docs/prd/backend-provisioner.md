# PRD: Backend Provisioner

**Status:** Draft for Review
**Version:** 1.0
**Last Updated:** 2025-11-19
**Author:** Erik Osterman

---

## Executive Summary

The Backend Provisioner is the first implementation of the Atmos Provisioner System. It automatically provisions Terraform state backends (S3, GCS, Azure Blob Storage) before Terraform initialization, eliminating cold-start friction for development and testing environments.

**Key Principle:** Backend provisioning is infrastructure plumbing - opinionated, automatic, and invisible to users except via simple configuration flags.

---

## Overview

### What is a Backend Provisioner?

The Backend Provisioner is a system hook that:
1. **Registers** for `before.terraform.init` hook event
2. **Checks** if `provision.backend.enabled: true` in component config
3. **Delegates** to backend-type-specific provisioner (S3, GCS, Azure)
4. **Provisions** minimal backend infrastructure with secure defaults

### Scope

**In Scope:**
- ✅ S3 backend provisioning (Phase 1 - see `s3-backend-provisioner.md`)
- ✅ GCS backend provisioning (Phase 2)
- ✅ Azure Blob backend provisioning (Phase 2)
- ✅ Secure defaults (encryption, versioning, public access blocking)
- ✅ Development/testing focus

**Out of Scope:**
- ❌ Production-grade features (custom KMS, replication, lifecycle policies)
- ❌ DynamoDB table provisioning (Terraform 1.10+ has native S3 locking)
- ❌ Backend migration/destruction
- ❌ Backend drift detection

---

## Architecture

### Backend Provisioner Registration

```go
// pkg/provisioner/backend/backend.go

package backend

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	// Backend provisioner registers for before.terraform.init
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "backend",
		HookEvent: hooks.BeforeTerraformInit,  // Self-declared timing
		Func:      ProvisionBackend,
	})
}
```

### Backend Provisioner Interface

```go
// BackendProvisionerFunc defines the interface for backend-specific provisioners.
type BackendProvisionerFunc func(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error
```

### Backend Registry

```go
// pkg/provisioner/backend/registry.go

package backend

import (
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Backend provisioner registry maps backend type → provisioner function
var backendProvisioners = make(map[string]BackendProvisionerFunc)
var registerBackendProvisionersOnce sync.Once

// RegisterBackendProvisioners registers all backend-specific provisioners.
func RegisterBackendProvisioners() {
	registerBackendProvisionersOnce.Do(func() {
		// Phase 1: S3 backend
		backendProvisioners["s3"] = ProvisionS3Backend

		// Phase 2: Multi-cloud backends
		// backendProvisioners["gcs"] = ProvisionGCSBackend
		// backendProvisioners["azurerm"] = ProvisionAzureBackend
	})
}

// GetBackendProvisioner retrieves a backend provisioner by type.
func GetBackendProvisioner(backendType string) (BackendProvisionerFunc, error) {
	if provisioner, ok := backendProvisioners[backendType]; ok {
		return provisioner, nil
	}
	return nil, fmt.Errorf("no provisioner for backend type: %s", backendType)
}
```

### Main Backend Provisioner Logic

```go
// pkg/provisioner/backend/backend.go

// ProvisionBackend is the main backend provisioner (delegates to backend-specific provisioners).
func ProvisionBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.backend.ProvisionBackend")()

	// 1. Check if backend provisioning is enabled
	if !isBackendProvisioningEnabled(componentSections) {
		return nil
	}

	// 2. Register backend-specific provisioners
	RegisterBackendProvisioners()

	// 3. Get backend type from component sections
	backendType := getBackendType(componentSections)
	if backendType == "" {
		return fmt.Errorf("backend_type not specified")
	}

	// 4. Get backend-specific provisioner
	backendProvisioner, err := GetBackendProvisioner(backendType)
	if err != nil {
		ui.Warning(fmt.Sprintf("No provisioner available for backend type '%s'", backendType))
		return nil  // Not an error - just skip provisioning
	}

	// 5. Execute backend-specific provisioner
	ui.Info(fmt.Sprintf("Provisioning %s backend...", backendType))
	if err := backendProvisioner(atmosConfig, componentSections, authContext); err != nil {
		return fmt.Errorf("failed to provision %s backend: %w", backendType, err)
	}

	ui.Success(fmt.Sprintf("Backend '%s' provisioned successfully", backendType))
	return nil
}

// isBackendProvisioningEnabled checks if provision.backend.enabled is true.
func isBackendProvisioningEnabled(componentSections *map[string]any) bool {
	provisionConfig, ok := (*componentSections)["provision"].(map[string]any)
	if !ok {
		return false
	}

	backendConfig, ok := provisionConfig["backend"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := backendConfig["enabled"].(bool)
	return ok && enabled
}

// getBackendType extracts backend_type from component sections.
func getBackendType(componentSections *map[string]any) string {
	backendType, ok := (*componentSections)["backend_type"].(string)
	if !ok {
		return ""
	}
	return backendType
}
```

---

## Configuration Schema

### Stack Manifest Configuration

```yaml
components:
  terraform:
    vpc:
      # Backend type (standard Terraform)
      backend_type: s3

      # Backend configuration (standard Terraform)
      backend:
        bucket: my-terraform-state
        key: vpc/terraform.tfstate
        region: us-east-1
        encrypt: true

        # Optional: Role assumption (standard Terraform syntax)
        assume_role:
          role_arn: "arn:aws:iam::999999999999:role/TerraformStateAdmin"
          session_name: "atmos-backend-provision"

      # Provisioning configuration (Atmos-specific, never serialized to backend.tf.json)
      provision:
        backend:
          enabled: true  # Enable auto-provisioning for this backend
```

### Global Configuration (atmos.yaml)

```yaml
# atmos.yaml
settings:
  backends:
    auto_provision:
      enabled: true  # Global feature flag (default: false)
```

### Configuration Hierarchy

The `provision.backend` configuration leverages Atmos's deep-merge system and can be specified at **multiple levels** in the stack hierarchy. This provides maximum flexibility for different organizational patterns.

#### 1. Top-Level Terraform Defaults

Enable provisioning for all components across all environments:

```yaml
# stacks/_defaults.yaml or stacks/orgs/acme/_defaults.yaml
terraform:
  provision:
    backend:
      enabled: true  # Applies to all components
```

#### 2. Environment-Level Configuration

Override defaults per environment:

```yaml
# stacks/orgs/acme/plat/dev/_defaults.yaml
terraform:
  provision:
    backend:
      enabled: true  # Enable for all dev components

# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  provision:
    backend:
      enabled: false  # Disable for production (use pre-provisioned backends)
```

#### 3. Component-Level Configuration

Override at the component level:

```yaml
# stacks/dev.yaml
components:
  terraform:
    vpc:
      provision:
        backend:
          enabled: true  # Enable for this specific component

    eks:
      provision:
        backend:
          enabled: false  # Disable for this specific component
```

#### 4. Inheritance via metadata.inherits

Share provision configuration through catalog components:

```yaml
# stacks/catalog/vpc/defaults.yaml
components:
  terraform:
    vpc/defaults:
      provision:
        backend:
          enabled: true

# stacks/dev.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
      # Automatically inherits provision.backend.enabled: true
```

#### Deep-Merge Behavior

Atmos deep-merges `provision` blocks across all hierarchy levels:

```yaml
# 1. Top-level default
terraform:
  provision:
    backend:
      enabled: true

# 2. Component override
components:
  terraform:
    vpc:
      provision:
        backend:
          enabled: false  # Overrides top-level setting

# Result after deep-merge:
# vpc component has provision.backend.enabled: false
```

**Key Benefits:**
- **DRY Principle**: Set defaults once at high levels
- **Environment Flexibility**: Dev uses auto-provision, prod uses pre-provisioned
- **Component Control**: Override per component when needed
- **Catalog Reuse**: Share provision settings through inherits

### Configuration Filtering

**Critical:** The `provision` block is **never serialized** to `backend.tf.json`:

```go
// internal/exec/terraform_generate_backend.go

func generateBackendConfig(backendConfig map[string]any) map[string]any {
	// Remove Atmos-specific keys before serialization
	filteredConfig := make(map[string]any)
	for k, v := range backendConfig {
		if k != "provision" {  // Filter out provision block
			filteredConfig[k] = v
		}
	}
	return filteredConfig
}
```

---

## Role Assumption and Cross-Account Provisioning

### How Role Assumption Works

1. **Component identity** (from `auth.providers.aws.identity`) provides base credentials
2. **Role ARN** (from `backend.assume_role.role_arn`) specifies cross-account role
3. **Backend provisioner** assumes role using base credentials
4. **Provisioning** happens in target account with assumed role

### Configuration Example

```yaml
components:
  terraform:
    vpc:
      # Source account identity
      auth:
        providers:
          aws:
            type: aws-sso
            identity: dev-admin  # Credentials in account 111111111111

      # Target account backend
      backend_type: s3
      backend:
        bucket: prod-terraform-state
        region: us-east-1

        # Assume role in target account
        assume_role:
          role_arn: "arn:aws:iam::999999999999:role/TerraformStateAdmin"
          session_name: "atmos-backend-provision"

      # Enable provisioning
      provision:
        backend:
          enabled: true
```

**Flow:**
1. Auth system authenticates as `dev-admin` (account 111111111111)
2. Backend provisioner extracts `role_arn` from backend config
3. Provisioner assumes role in target account (999999999999)
4. S3 bucket created in target account

### Implementation Pattern

```go
// Backend provisioners follow this pattern for role assumption

func ProvisionS3Backend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	// Extract backend config
	backendConfig := (*componentSections)["backend"].(map[string]any)
	region := backendConfig["region"].(string)

	// Get role ARN from backend config (if specified)
	roleArn := GetS3BackendAssumeRoleArn(&backendConfig)

	// Load AWS config with auth context + role assumption
	cfg, err := awsUtils.LoadAWSConfigWithAuth(
		ctx,
		region,
		roleArn,           // From backend.assume_role.role_arn
		15*time.Minute,
		authContext.AWS,   // From component's auth.identity
	)

	// Create client and provision
	client := s3.NewFromConfig(cfg)
	return provisionBucket(client, bucket)
}

// GetS3BackendAssumeRoleArn extracts role ARN from backend config (standard Terraform syntax)
func GetS3BackendAssumeRoleArn(backend *map[string]any) string {
	// Try assume_role block first (standard Terraform)
	if assumeRoleSection, ok := (*backend)["assume_role"].(map[string]any); ok {
		if roleArn, ok := assumeRoleSection["role_arn"].(string); ok {
			return roleArn
		}
	}

	// Fallback to top-level role_arn (legacy)
	if roleArn, ok := (*backend)["role_arn"].(string); ok {
		return roleArn
	}

	return ""
}
```

---

## Backend-Specific Provisioner Requirements

### Interface Contract

All backend provisioners MUST:

1. **Check if backend exists** (idempotent operation)
2. **Create backend with secure defaults** (if doesn't exist)
3. **Return nil** (no error) if backend already exists
4. **Use AuthContext** for authentication
5. **Support role assumption** from backend config
6. **Implement client caching** for performance
7. **Retry with exponential backoff** for transient failures
8. **Log provisioning actions** with ui package

### Hardcoded Best Practices (No Configuration)

All backends MUST create resources with:

- ✅ **Encryption** - Always enabled (provider-managed keys)
- ✅ **Versioning** - Always enabled (recovery from accidental deletions)
- ✅ **Public Access** - Always blocked (security)
- ✅ **Resource Tags/Labels**:
  - `ManagedBy: Atmos`
  - `CreatedAt: <ISO8601 timestamp>`
  - `Purpose: TerraformState`

### Provider-Specific Defaults

#### AWS S3
- Encryption: AES-256 or AWS-managed KMS
- Versioning: Enabled
- Public access: All 4 settings blocked
- State locking: Terraform 1.10+ native S3 locking (no DynamoDB)

#### GCP GCS
- Encryption: Google-managed encryption keys
- Versioning: Enabled
- Access: Uniform bucket-level access
- State locking: Native GCS locking

#### Azure Blob Storage
- Encryption: Microsoft-managed keys
- Versioning: Blob versioning enabled
- HTTPS: Required
- State locking: Native blob lease locking

---

## Implementation Guide

### Step 1: Implement Backend Provisioner

```go
// pkg/provisioner/backend/mybackend.go

package backend

func ProvisionMyBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.backend.ProvisionMyBackend")()

	// 1. Extract backend configuration
	backendConfig := (*componentSections)["backend"].(map[string]any)
	containerName := backendConfig["container"].(string)

	// 2. Get authenticated client (with role assumption if needed)
	client, err := getMyBackendClient(backendConfig, authContext)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// 3. Check if backend exists (idempotent)
	exists, err := checkMyBackendExists(client, containerName)
	if err != nil {
		return fmt.Errorf("failed to check backend existence: %w", err)
	}

	if exists {
		ui.Info(fmt.Sprintf("Backend '%s' already exists", containerName))
		return nil
	}

	// 4. Create backend with hardcoded best practices
	ui.Info(fmt.Sprintf("Creating backend '%s'...", containerName))
	if err := createMyBackend(client, containerName); err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	ui.Success(fmt.Sprintf("Backend '%s' created successfully", containerName))
	return nil
}
```

### Step 2: Register Backend Provisioner

```go
// pkg/provisioner/backend/registry.go

func RegisterBackendProvisioners() {
	registerBackendProvisionersOnce.Do(func() {
		backendProvisioners["s3"] = ProvisionS3Backend
		backendProvisioners["mybackend"] = ProvisionMyBackend  // Add new backend
	})
}
```

### Step 3: Test Backend Provisioner

```go
// pkg/provisioner/backend/mybackend_test.go

func TestProvisionMyBackend_NewBackend(t *testing.T) {
	// Mock client
	mockClient := &MockMyBackendClient{}

	// Configure mock expectations
	mockClient.EXPECT().
		CheckExists("my-container").
		Return(false, nil)

	mockClient.EXPECT().
		Create("my-container", gomock.Any()).
		Return(nil)

	// Execute provisioner
	err := ProvisionMyBackend(atmosConfig, componentSections, authContext)

	// Verify
	assert.NoError(t, err)
}

func TestProvisionMyBackend_ExistingBackend(t *testing.T) {
	// Mock client
	mockClient := &MockMyBackendClient{}

	// Backend already exists
	mockClient.EXPECT().
		CheckExists("my-container").
		Return(true, nil)

	// Should NOT call Create
	mockClient.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Times(0)

	// Execute provisioner
	err := ProvisionMyBackend(atmosConfig, componentSections, authContext)

	// Verify idempotent behavior
	assert.NoError(t, err)
}
```

---

## Error Handling

### Error Definitions

```go
// pkg/provisioner/backend/errors.go

package backend

import "errors"

var (
	// Backend provisioning errors
	ErrBackendProvision     = errors.New("backend provisioning failed")
	ErrBackendCheck         = errors.New("backend existence check failed")
	ErrBackendConfig        = errors.New("invalid backend configuration")
	ErrBackendTypeUnsupported = errors.New("backend type not supported for provisioning")
)
```

### Error Examples

```go
// Configuration error
if bucket == "" {
	return fmt.Errorf("%w: bucket name is required", ErrBackendConfig)
}

// Provisioning error with context
return errUtils.Build(ErrBackendProvision).
	WithHint("Verify AWS credentials have s3:CreateBucket permission").
	WithContext("backend_type", "s3").
	WithContext("bucket", bucket).
	WithContext("region", region).
	WithExitCode(2).
	Err()

// Permission error
return errUtils.Build(ErrBackendProvision).
	WithHint("Required permissions: s3:CreateBucket, s3:PutBucketVersioning, s3:PutBucketEncryption").
	WithHintf("Check IAM policy for identity: %s", authContext.AWS.Profile).
	WithContext("bucket", bucket).
	WithExitCode(2).
	Err()
```

---

## Testing Strategy

### Unit Tests (per backend type)

```go
func TestProvisionS3Backend_NewBucket(t *testing.T)
func TestProvisionS3Backend_ExistingBucket(t *testing.T)
func TestProvisionS3Backend_InvalidConfig(t *testing.T)
func TestProvisionS3Backend_PermissionDenied(t *testing.T)
func TestProvisionS3Backend_RoleAssumption(t *testing.T)
```

### Integration Tests

```go
// tests/backend_provisioning_test.go

func TestBackendProvisioning_S3_FreshAccount(t *testing.T) {
	// Requires: localstack or real AWS account
	tests.RequireAWSAccess(t)

	// Execute provisioning
	err := ProvisionBackend(atmosConfig, componentSections, authContext)

	// Verify bucket created with correct settings
	assert.NoError(t, err)
	assertBucketExists(t, "my-test-bucket")
	assertVersioningEnabled(t, "my-test-bucket")
	assertEncryptionEnabled(t, "my-test-bucket")
	assertPublicAccessBlocked(t, "my-test-bucket")
}

func TestBackendProvisioning_S3_Idempotent(t *testing.T) {
	// Create bucket manually first
	createBucket(t, "my-test-bucket")

	// Execute provisioning
	err := ProvisionBackend(atmosConfig, componentSections, authContext)

	// Should not error - idempotent
	assert.NoError(t, err)
}
```

---

## Security Considerations

### IAM Permissions

Backend provisioners require specific permissions. Document these clearly:

**AWS S3 Backend:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:HeadBucket",
        "s3:PutBucketVersioning",
        "s3:PutBucketEncryption",
        "s3:PutBucketPublicAccessBlock",
        "s3:PutBucketTagging"
      ],
      "Resource": "arn:aws:s3:::*-terraform-state-*"
    }
  ]
}
```

**GCP GCS Backend:**
```yaml
roles:
  - roles/storage.admin  # For bucket creation
```

**Azure Blob Backend:**
```yaml
permissions:
  - Microsoft.Storage/storageAccounts/write
  - Microsoft.Storage/storageAccounts/blobServices/containers/write
```

### Security Defaults

All backends MUST:
- ✅ Enable encryption at rest
- ✅ Block public access
- ✅ Enable versioning
- ✅ Use provider-managed keys (not custom KMS for simplicity)
- ✅ Apply resource tags for tracking

---

## Performance Optimization

### Client Caching

```go
// Per-backend client cache
var s3ClientCache sync.Map

func getCachedS3Client(region string, authContext *schema.AuthContext) (*s3.Client, error) {
	// Build deterministic cache key
	cacheKey := fmt.Sprintf("region=%s;profile=%s", region, authContext.AWS.Profile)

	// Check cache
	if cached, ok := s3ClientCache.Load(cacheKey); ok {
		return cached.(*s3.Client), nil
	}

	// Create client with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := LoadAWSConfigWithAuth(ctx, region, authContext)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg)
	s3ClientCache.Store(cacheKey, client)
	return client, nil
}
```

### Retry Logic

```go
const maxRetries = 3

func provisionWithRetry(fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			backoff := time.Duration(attempt+1) * 2 * time.Second
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("provisioning failed after %d attempts: %w", maxRetries, lastErr)
}
```

---

## Documentation Requirements

Each backend provisioner MUST document:

1. **Backend type** - What backend does it provision?
2. **Resources created** - What infrastructure is created?
3. **Hardcoded defaults** - What security settings are applied?
4. **Required permissions** - What IAM/RBAC permissions needed?
5. **Configuration example** - How to enable provisioning?
6. **Limitations** - What's NOT supported (custom KMS, etc.)?
7. **Migration path** - How to upgrade to production backend?

---

## CLI Commands

### Backend Provisioning Command

```bash
# Provision backend explicitly
atmos provision backend <component> --stack <stack>

# Examples
atmos provision backend vpc --stack dev
atmos provision backend eks --stack prod
```

**When to use:**
- Separate provisioning from Terraform execution (CI/CD pipelines)
- Troubleshoot provisioning issues
- Pre-provision backends for multiple components

**Automatic provisioning (via hooks):**
```bash
# Backend provisioned automatically if provision.backend.enabled: true
atmos terraform apply vpc --stack dev
```

### Error Handling in CLI

**Provisioning failure stops execution:**
```bash
$ atmos provision backend vpc --stack dev
Error: provisioner 'backend' failed: backend provisioning failed:
failed to create bucket: AccessDenied

Hint: Verify AWS credentials have s3:CreateBucket permission
Context: bucket=acme-state-dev, region=us-east-1

Exit code: 3
```

**Terraform won't run if provisioning fails:**
```bash
$ atmos terraform apply vpc --stack dev
Running backend provisioner...
Error: Provisioning failed - cannot proceed with terraform
provisioner 'backend' failed: backend provisioning failed

Exit code: 2
```

---

## Error Handling and Propagation

### Error Handling Requirements

**All backend provisioners MUST:**

1. **Return errors (never panic)**
   ```go
   func ProvisionS3Backend(...) error {
       if err := createBucket(); err != nil {
           return fmt.Errorf("failed to create bucket: %w", err)
       }
       return nil
   }
   ```

2. **Return nil for idempotent operations**
   ```go
   if bucketExists {
       ui.Info("Bucket already exists (idempotent)")
       return nil  // Not an error
   }
   ```

3. **Use error builder for detailed errors**
   ```go
   return errUtils.Build(errUtils.ErrBackendProvision).
       WithHint("Verify AWS credentials have s3:CreateBucket permission").
       WithContext("bucket", bucket).
       WithExitCode(3).
       Err()
   ```

4. **Fail fast on critical errors**
   ```go
   if bucket == "" {
       return fmt.Errorf("%w: bucket name required", errUtils.ErrInvalidConfig)
   }
   ```

### Error Propagation Flow

```
Backend Provisioner (ProvisionS3Backend)
  ↓ returns error
Backend Provisioner Wrapper (ProvisionBackend)
  ↓ wraps and returns
Hook System (ExecuteProvisionerHooks)
  ↓ propagates immediately (fail fast)
Terraform Execution (ExecuteTerraform)
  ↓ stops before terraform init
Main (main.go)
  ↓ exits with error code
CI/CD Pipeline
  ↓ fails build
```

### Exit Codes

| Exit Code | Error Type | Example |
|-----------|------------|---------|
| 0 | Success | Backend created or already exists |
| 1 | General error | Unexpected AWS SDK error |
| 2 | Configuration error | Missing bucket name in config |
| 3 | Permission error | IAM s3:CreateBucket denied |
| 4 | Resource conflict | Bucket name globally taken |
| 5 | Network error | Connection timeout to AWS API |

### Error Examples by Backend Type

#### S3 Backend Errors

**Configuration Error:**
```go
if bucket == "" {
    return fmt.Errorf("%w: backend.bucket is required", errUtils.ErrBackendConfig)
}
```

**Permission Error:**
```go
if isAccessDenied(err) {
    return errUtils.Build(errUtils.ErrBackendProvision).
        WithHint("Required IAM permissions: s3:CreateBucket, s3:PutBucketVersioning").
        WithHintf("Check policy for identity: %s", authContext.AWS.Profile).
        WithContext("bucket", bucket).
        WithExitCode(3).
        Err()
}
```

**Resource Conflict:**
```go
if isBucketNameTaken(err) {
    return errUtils.Build(errUtils.ErrBackendProvision).
        WithHint("S3 bucket names are globally unique across all AWS accounts").
        WithHintf("Try a different name: %s-%s", bucket, accountID).
        WithContext("bucket", bucket).
        WithExitCode(4).
        Err()
}
```

#### GCS Backend Errors (Future)

**Permission Error:**
```go
return errUtils.Build(errUtils.ErrBackendProvision).
    WithHint("Required GCP permissions: storage.buckets.create").
    WithContext("bucket", bucket).
    WithContext("project", project).
    WithExitCode(3).
    Err()
```

#### Azure Backend Errors (Future)

**Permission Error:**
```go
return errUtils.Build(errUtils.ErrBackendProvision).
    WithHint("Required Azure permissions: Microsoft.Storage/storageAccounts/write").
    WithContext("storage_account", storageAccount).
    WithExitCode(3).
    Err()
```

### Testing Error Handling

**Unit Tests:**
```go
func TestProvisionS3Backend_ConfigurationError(t *testing.T) {
    componentSections := map[string]any{
        "backend": map[string]any{
            // Missing bucket name
            "region": "us-east-1",
        },
    }

    err := ProvisionS3Backend(atmosConfig, &componentSections, authContext)

    assert.Error(t, err)
    assert.ErrorIs(t, err, errUtils.ErrBackendConfig)
    assert.Contains(t, err.Error(), "bucket")
}

func TestProvisionS3Backend_PermissionDenied(t *testing.T) {
    mockClient := &MockS3Client{}
    mockClient.EXPECT().
        CreateBucket(gomock.Any()).
        Return(nil, awserr.New("AccessDenied", "Permission denied", nil))

    err := ProvisionS3Backend(...)

    assert.Error(t, err)
    assert.ErrorIs(t, err, errUtils.ErrBackendProvision)
    exitCode := errUtils.GetExitCode(err)
    assert.Equal(t, 3, exitCode)
}
```

---

## Related Documents

- **[Provisioner System](./provisioner-system.md)** - Generic provisioner infrastructure
- **[S3 Backend Provisioner](./s3-backend-provisioner.md)** - S3 implementation (reference)

---

## Appendix: Backend Type Registry

| Backend Type | Status | Provisioner | Phase |
|--------------|--------|-------------|-------|
| `s3` | ✅ Implemented | `ProvisionS3Backend` | Phase 1 |
| `gcs` | 🔄 Planned | `ProvisionGCSBackend` | Phase 2 |
| `azurerm` | 🔄 Planned | `ProvisionAzureBackend` | Phase 2 |
| `local` | ❌ Not applicable | N/A | - |
| `cloud` | ❌ Not applicable | N/A (Terraform Cloud manages storage) | - |
| `remote` | ❌ Deprecated | N/A | - |

---

**End of PRD**

**Status:** Ready for Review
**Next Steps:**
1. Review backend provisioner interface
2. Implement S3 backend provisioner (see `s3-backend-provisioner.md`)
3. Test with localstack/real AWS account
4. Add GCS and Azure provisioners (Phase 2)
