# PRD: S3 Backend Provisioner

**Status:** Draft for Review
**Version:** 1.0
**Last Updated:** 2025-11-19
**Author:** Erik Osterman

---

## Executive Summary

The S3 Backend Provisioner automatically creates AWS S3 buckets for Terraform state storage with secure defaults. It's the reference implementation of the Backend Provisioner interface and eliminates cold-start friction for development and testing environments.

**Key Principle:** Simple, opinionated S3 buckets with AWS best practices - not production-ready infrastructure.

---

## Problem Statement

### Current Pain Points

1. **Manual Bucket Creation**: Users must create S3 buckets before running `terraform init`
2. **Inconsistent Security**: Manual bucket creation leads to varying security settings
3. **Onboarding Friction**: New developers need AWS console access or separate scripts
4. **Cold Start Delay**: Setting up new environments requires multiple manual steps

### Target Users

- **Development Teams**: Quick environment setup for testing
- **New Users**: First-time Terraform/Atmos users learning the system
- **CI/CD Pipelines**: Ephemeral environments that need automatic backend creation
- **POCs/Demos**: Rapid prototyping without infrastructure overhead

### Non-Target Users

- **Production Environments**: Should use `terraform-aws-tfstate-backend` module for:
  - Custom KMS encryption
  - Cross-region replication
  - DynamoDB state locking
  - Advanced lifecycle policies
  - Compliance requirements (HIPAA, SOC2, etc.)

---

## Goals & Non-Goals

### Goals

1. ✅ **Automatic S3 Bucket Creation**: Create bucket if doesn't exist
2. ✅ **Secure Defaults**: Encryption, versioning, public access blocking (always enabled)
3. ✅ **Idempotent Operations**: Safe to run multiple times
4. ✅ **Cross-Account Support**: Provision buckets via role assumption
5. ✅ **Zero Configuration**: No options beyond `enabled: true`
6. ✅ **Fast Implementation**: ~1 week timeline
7. ✅ **Backend Deletion**: Delete backend infrastructure with safety checks

### Non-Goals

1. ❌ **DynamoDB Tables**: Use Terraform 1.10+ native S3 locking
2. ❌ **Custom KMS Keys**: Use AWS-managed encryption
3. ❌ **Replication**: No cross-region bucket replication
4. ❌ **Lifecycle Policies**: No object expiration/transitions
5. ❌ **Access Logging**: No S3 access logs
6. ❌ **Production Features**: Not competing with terraform-aws-tfstate-backend module

---

## What Gets Created

### S3 Bucket with Hardcoded Best Practices

When `provision.backend.enabled: true` and bucket doesn't exist:

#### Always Enabled (No Configuration)

1. **Versioning**: Enabled for state file recovery
2. **Encryption**: Server-side encryption with SSE-S3 (AES-256, AWS-managed keys)
3. **Public Access**: All 4 public access settings blocked
4. **Resource Tags**:
   - `ManagedBy: Atmos`
   - `Name: <bucket-name>`

> **Note**: Bucket Key is not enabled because it only applies to SSE-KMS encryption.
> The implementation uses SSE-S3 (AES-256) for simplicity and zero additional cost.

#### NOT Created

- ❌ DynamoDB table (Terraform 1.10+ has native S3 state locking)
- ❌ Custom KMS key
- ❌ Replication configuration
- ❌ Lifecycle rules
- ❌ Access logging bucket
- ❌ Object lock/WORM
- ❌ Bucket policies (beyond public access block)

---

## Backend Deletion

### Delete Command

The `atmos terraform backend delete` command permanently removes backend infrastructure.

```shell
# Delete empty backend
atmos terraform backend delete vpc --stack dev --force

# Command will error if bucket contains objects (unless --force)
```

### Safety Mechanisms

#### Force Flag Required

The `--force` flag is **always required** for deletion to prevent accidental removal:

```shell
# This command requires --force
atmos terraform backend delete vpc --stack dev --force
```

#### Non-Empty Bucket Handling

The `--force` flag is always required. When provided, the command:

- Lists all objects and versions in bucket
- Shows count of objects and state files to be deleted
- Displays warning if `.tfstate` files are present
- Deletes all objects (including versions)
- Deletes the bucket itself
- Operation is irreversible

**Without `--force` flag:**
- Command exits with error: "the --force flag is required for deletion"
- No bucket inspection or deletion occurs

### Delete Process

When you run `atmos terraform backend delete --force`:

1. **Validate Configuration** - Load component's stack configuration
2. **Check Backend Type** - Verify supported backend type (s3, gcs, azurerm)
3. **List Objects** - Enumerate all objects and versions in bucket
4. **Detect State Files** - Count `.tfstate` files for warning message
5. **Warn User** - Display count of objects and state files to be deleted
6. **Delete Objects** - Remove all objects and versions (batch operations)
7. **Delete Bucket** - Remove the empty bucket
8. **Confirm Success** - Report completion

### Error Scenarios

- **Bucket Not Found**: Error if backend doesn't exist
- **Permission Denied**: AWS IAM permissions insufficient
- **Deletion Failure**: Partial delete (objects removed but bucket remains)
- **Force Required**: User didn't provide `--force` flag

### Best Practices

1. **Backup State Files**: Download `.tfstate` files before deletion
2. **Verify Component**: Use `describe` to confirm correct backend
3. **Check Stack**: Ensure you're targeting the right environment
4. **Document Deletion**: Record why backend was deleted
5. **Cross-Account**: Ensure role assumption permissions for delete operations

### What Gets Deleted

- ✅ S3 bucket and all objects
- ✅ All object versions (if versioning enabled)
- ✅ Terraform state files (`.tfstate`)
- ✅ Delete markers
- ❌ DynamoDB tables (not created by provisioner)
- ❌ KMS keys (not created by provisioner)
- ❌ IAM roles/policies (not created by provisioner)

---

## Configuration

### Stack Manifest Example

```yaml
# stacks/dev/us-east-1.yaml
components:
  terraform:
    vpc:
      # Component authentication
      auth:
        providers:
          aws:
            type: aws-sso
            identity: dev-admin

      # Backend configuration (standard Terraform)
      backend_type: s3
      backend:
        bucket: acme-terraform-state-dev-use1
        key: vpc/terraform.tfstate
        region: us-east-1
        encrypt: true

      # Provisioning configuration (Atmos-only)
      provision:
        backend:
          enabled: true  # Enable automatic S3 bucket creation
```

### Cross-Account Provisioning

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
        key: vpc/terraform.tfstate
        region: us-east-1

        # Assume role in target account (standard Terraform syntax)
        assume_role:
          role_arn: "arn:aws:iam::999999999999:role/TerraformStateAdmin"
          session_name: "atmos-backend-provision"

      # Enable provisioning
      provision:
        backend:
          enabled: true
```

**Flow:**
1. Authenticate as `dev-admin` in account 111111111111
2. Assume `TerraformStateAdmin` role in account 999999999999
3. Create S3 bucket in account 999999999999

### Multi-Environment Setup with Inheritance

Leverage Atmos's deep-merge system to configure provisioning at different hierarchy levels:

#### Organization-Level Defaults

Enable provisioning for all development and staging environments:

```yaml
# stacks/orgs/acme/_defaults.yaml
terraform:
  backend_type: s3
  backend:
    region: us-east-1
    encrypt: true

# stacks/orgs/acme/plat/dev/_defaults.yaml
terraform:
  backend:
    bucket: acme-terraform-state-dev  # Dev bucket
  provision:
    backend:
      enabled: true  # Auto-provision in dev

# stacks/orgs/acme/plat/staging/_defaults.yaml
terraform:
  backend:
    bucket: acme-terraform-state-staging  # Staging bucket
  provision:
    backend:
      enabled: true  # Auto-provision in staging

# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  backend:
    bucket: acme-terraform-state-prod  # Prod bucket
  provision:
    backend:
      enabled: false  # Pre-provisioned in prod (managed by Terraform)
```

#### Catalog Inheritance Pattern

Share provision configuration through component catalogs:

```yaml
# stacks/catalog/networking/vpc.yaml
components:
  terraform:
    vpc/defaults:
      backend_type: s3
      backend:
        key: vpc/terraform.tfstate
        region: us-east-1
      provision:
        backend:
          enabled: true  # Default: auto-provision

# stacks/dev/us-east-1.yaml
components:
  terraform:
    vpc-dev:
      metadata:
        inherits: [vpc/defaults]
      # Inherits provision.backend.enabled: true
      backend:
        bucket: acme-terraform-state-dev  # Dev-specific bucket

# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc-prod:
      metadata:
        inherits: [vpc/defaults]
      backend:
        bucket: acme-terraform-state-prod  # Prod-specific bucket
      provision:
        backend:
          enabled: false  # Override: disable for production
```

#### Per-Component Override

Override provisioning for specific components:

```yaml
# stacks/dev/us-east-1.yaml
components:
  terraform:
    # VPC uses auto-provisioning (inherits from environment defaults)
    vpc:
      backend:
        bucket: acme-terraform-state-dev
        key: vpc/terraform.tfstate
      # provision.backend.enabled: true (inherited)

    # EKS explicitly disables auto-provisioning
    eks:
      backend:
        bucket: acme-terraform-state-dev
        key: eks/terraform.tfstate
      provision:
        backend:
          enabled: false  # Component-level override
```

**Benefits of Hierarchy:**
- **DRY**: Configure once at organization/environment level
- **Flexibility**: Override per component when needed
- **Consistency**: All dev environments auto-provision, all prod environments use pre-provisioned backends
- **Maintainability**: Change provisioning policy in one place

---

## Implementation

### Package Structure

```text
pkg/provisioner/backend/
  ├── s3.go              # S3 backend provisioner
  ├── s3_test.go         # Unit tests
  ├── s3_integration_test.go  # Integration tests
```

### Core Implementation

```go
// pkg/provisioner/backend/s3.go

package backend

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsUtils "github.com/cloudposse/atmos/internal/aws"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// S3 client cache (performance optimization)
var s3ProvisionerClientCache sync.Map

// ProvisionS3Backend provisions an S3 bucket for Terraform state.
func ProvisionS3Backend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.backend.ProvisionS3Backend")()

	// 1. Extract backend configuration
	backendConfig, ok := (*componentSections)["backend"].(map[string]any)
	if !ok {
		return fmt.Errorf("backend configuration not found")
	}

	bucket, ok := backendConfig["bucket"].(string)
	if !ok || bucket == "" {
		return fmt.Errorf("bucket name is required in backend configuration")
	}

	region, ok := backendConfig["region"].(string)
	if !ok || region == "" {
		return fmt.Errorf("region is required in backend configuration")
	}

	// 2. Get or create S3 client (with role assumption if needed)
	client, err := getCachedS3ProvisionerClient(region, &backendConfig, authContext)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	// 3. Check if bucket exists (idempotent)
	ctx := context.Background()
	exists, err := checkS3BucketExists(ctx, client, bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if exists {
		ui.Info(fmt.Sprintf("S3 bucket '%s' already exists (idempotent)", bucket))
		return nil
	}

	// 4. Create bucket with hardcoded best practices
	ui.Info(fmt.Sprintf("Creating S3 bucket '%s' with secure defaults...", bucket))
	if err := provisionS3BucketWithDefaults(ctx, client, bucket, region); err != nil {
		return fmt.Errorf("failed to provision S3 bucket: %w", err)
	}

	ui.Success(fmt.Sprintf("Successfully created S3 bucket '%s'", bucket))
	return nil
}

// getCachedS3ProvisionerClient returns a cached or new S3 client.
func getCachedS3ProvisionerClient(
	region string,
	backendConfig *map[string]any,
	authContext *schema.AuthContext,
) (*s3.Client, error) {
	defer perf.Track(nil, "provisioner.backend.getCachedS3ProvisionerClient")()

	// Extract role ARN if specified
	roleArn := GetS3BackendAssumeRoleArn(backendConfig)

	// Build deterministic cache key
	cacheKey := fmt.Sprintf("region=%s", region)
	if authContext != nil && authContext.AWS != nil {
		cacheKey += fmt.Sprintf(";profile=%s", authContext.AWS.Profile)
	}
	if roleArn != "" {
		cacheKey += fmt.Sprintf(";role=%s", roleArn)
	}

	// Check cache
	if cached, ok := s3ProvisionerClientCache.Load(cacheKey); ok {
		return cached.(*s3.Client), nil
	}

	// Create new client with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load AWS config with auth context + role assumption
	cfg, err := awsUtils.LoadAWSConfigWithAuth(
		ctx,
		region,
		roleArn,
		15*time.Minute,
		authContext.AWS,
	)
	if err != nil {
		return nil, err
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg)
	s3ProvisionerClientCache.Store(cacheKey, client)
	return client, nil
}

// checkS3BucketExists checks if an S3 bucket exists.
// Returns:
//   - (true, nil) if bucket exists and is accessible
//   - (false, nil) if bucket does not exist (404/NotFound)
//   - (false, error) if access denied (403) or other errors occur
func checkS3BucketExists(ctx context.Context, client *s3.Client, bucket string) (bool, error) {
	defer perf.Track(nil, "provisioner.backend.checkS3BucketExists")()

	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil {
		// Check for specific error types to distinguish between "not found" and "access denied".
		var notFound *s3types.NotFound
		var noSuchBucket *s3types.NoSuchBucket
		if errors.As(err, &notFound) || errors.As(err, &noSuchBucket) {
			// Bucket genuinely doesn't exist - safe to proceed with creation.
			return false, nil
		}
		// For AccessDenied (403) or other errors, return the error.
		// This prevents attempting to create a bucket we can't access.
		return false, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	return true, nil
}

// provisionS3BucketWithDefaults creates an S3 bucket with hardcoded best practices.
func provisionS3BucketWithDefaults(
	ctx context.Context,
	client *s3.Client,
	bucket, region string,
) error {
	defer perf.Track(nil, "provisioner.backend.provisionS3BucketWithDefaults")()

	// 1. Create bucket
	createInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	// For regions other than us-east-1, must specify location constraint
	if region != "us-east-1" {
		createInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(region),
		}
	}

	if _, err := client.CreateBucket(ctx, createInput); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Wait for bucket to be available (eventual consistency)
	time.Sleep(2 * time.Second)

	// 2. Enable versioning (ALWAYS)
	ui.Info("Enabling bucket versioning...")
	if _, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	}); err != nil {
		return fmt.Errorf("failed to enable versioning: %w", err)
	}

	// 3. Enable encryption (ALWAYS - AES-256)
	ui.Info("Enabling bucket encryption (AES-256)...")
	if _, err := client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
			Rules: []s3types.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
						SSEAlgorithm: s3types.ServerSideEncryptionAes256,
					},
					BucketKeyEnabled: aws.Bool(true),
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to enable encryption: %w", err)
	}

	// 4. Block public access (ALWAYS)
	ui.Info("Blocking public access...")
	if _, err := client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}); err != nil {
		return fmt.Errorf("failed to block public access: %w", err)
	}

	// 5. Apply standard tags (ALWAYS)
	ui.Info("Applying resource tags...")
	if _, err := client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucket),
		Tagging: &s3types.Tagging{
			TagSet: []s3types.Tag{
				{Key: aws.String("ManagedBy"), Value: aws.String("Atmos")},
				{Key: aws.String("CreatedAt"), Value: aws.String(time.Now().Format(time.RFC3339))},
				{Key: aws.String("Purpose"), Value: aws.String("TerraformState")},
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to apply tags: %w", err)
	}

	return nil
}

// GetS3BackendAssumeRoleArn extracts role ARN from backend config (standard Terraform syntax).
func GetS3BackendAssumeRoleArn(backend *map[string]any) string {
	// Try assume_role block first (standard Terraform)
	if assumeRoleSection, ok := (*backend)["assume_role"].(map[string]any); ok {
		if roleArn, ok := assumeRoleSection["role_arn"].(string); ok && roleArn != "" {
			return roleArn
		}
	}

	// Fallback to top-level role_arn (legacy)
	if roleArn, ok := (*backend)["role_arn"].(string); ok && roleArn != "" {
		return roleArn
	}

	return ""
}
```

---

## Testing Strategy

### Unit Tests

**File:** `pkg/provisioner/backend/s3_test.go`

```go
func TestProvisionS3Backend_NewBucket(t *testing.T) {
	// Test: Bucket doesn't exist → create bucket with all settings
}

func TestProvisionS3Backend_ExistingBucket(t *testing.T) {
	// Test: Bucket exists → return nil (idempotent)
}

func TestProvisionS3Backend_InvalidConfig(t *testing.T) {
	// Test: Missing bucket/region → return error
}

func TestProvisionS3Backend_RoleAssumption(t *testing.T) {
	// Test: Role ARN specified → assume role and create bucket
}

func TestCheckS3BucketExists(t *testing.T) {
	// Test: HeadBucket returns 200 → true
	// Test: HeadBucket returns 404 → false
}

func TestProvisionS3BucketWithDefaults(t *testing.T) {
	// Test: All bucket settings applied correctly
	// Test: Versioning enabled
	// Test: Encryption enabled
	// Test: Public access blocked
	// Test: Tags applied
}

func TestGetCachedS3ProvisionerClient(t *testing.T) {
	// Test: Client cached and reused
	// Test: Different cache key per region/profile/role
}

func TestGetS3BackendAssumeRoleArn(t *testing.T) {
	// Test: Extract from assume_role.role_arn
	// Test: Fallback to top-level role_arn
	// Test: Return empty string if not specified
}
```

**Mocking Strategy:**
- Use `go.uber.org/mock/mockgen` for AWS SDK interfaces
- Mock S3 client for unit tests
- Table-driven tests for configuration variants

### Integration Tests

**File:** `pkg/provisioner/backend/s3_integration_test.go`

```go
func TestS3BackendProvisioning_Localstack(t *testing.T) {
	// Requires: Docker with localstack
	tests.RequireLocalstack(t)

	// Create S3 bucket via provisioner
	// Verify bucket exists
	// Verify versioning enabled
	// Verify encryption enabled
	// Verify public access blocked
	// Verify tags applied
}

func TestS3BackendProvisioning_RealAWS(t *testing.T) {
	// Requires: Real AWS account with credentials
	tests.RequireAWSAccess(t)

	// Create unique bucket name
	bucket := fmt.Sprintf("atmos-test-%s", randomString())

	// Provision bucket
	// Verify bucket created with all settings
	// Cleanup: delete bucket
}

func TestS3BackendProvisioning_Idempotent(t *testing.T) {
	// Create bucket first
	// Run provisioner again
	// Verify no error (idempotent)
}
```

**Test Infrastructure:**
- Docker Compose with localstack for local testing
- Real AWS account for integration tests (optional)
- Cleanup helpers to delete test buckets

### Manual Testing Checklist

- [ ] Fresh AWS account (verify bucket created)
- [ ] Existing bucket (verify idempotent, no errors)
- [ ] Cross-region bucket creation (us-east-1, us-west-2, eu-west-1)
- [ ] Cross-account provisioning (role assumption)
- [ ] Permission denied (verify clear error message)
- [ ] Invalid bucket name (verify error handling)
- [ ] Bucket name conflict (globally unique names)
- [ ] Integration with `atmos terraform init`
- [ ] Integration with `atmos terraform apply`

---

## Security

### Required IAM Permissions

**Minimal permissions for S3 backend provisioning:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3BackendProvisioning",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:HeadBucket",
        "s3:PutBucketVersioning",
        "s3:GetBucketVersioning",
        "s3:PutBucketEncryption",
        "s3:GetBucketEncryption",
        "s3:PutBucketPublicAccessBlock",
        "s3:GetBucketPublicAccessBlock",
        "s3:PutBucketTagging",
        "s3:GetBucketTagging"
      ],
      "Resource": "arn:aws:s3:::*-terraform-state-*"
    }
  ]
}
```

**Cross-account role trust policy:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::111111111111:role/DevAdminRole"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "atmos-backend-provision"
        }
      }
    }
  ]
}
```

### Security Best Practices (Hardcoded)

Every auto-provisioned S3 bucket includes:

1. **Encryption at Rest**: Server-side encryption with AES-256
2. **Versioning**: Enabled for state file recovery
3. **Public Access**: All 4 settings blocked
4. **Bucket Key**: Enabled for cost reduction
5. **Resource Tags**: Tracking and attribution

**What's NOT Included (Use terraform-aws-tfstate-backend for Production):**
- ❌ Custom KMS keys
- ❌ Access logging
- ❌ Object lock (WORM)
- ❌ MFA delete
- ❌ Bucket policies (beyond public access)
- ❌ Lifecycle rules
- ❌ Replication

---

## Error Handling

### Common Errors and Solutions

#### 1. Bucket Name Already Taken

**Error:**
```text
failed to provision S3 bucket: BucketAlreadyExists: The requested bucket name is not available
```

**Cause:** S3 bucket names are globally unique across all AWS accounts.

**Solution:**
```yaml
# Use more specific bucket name
backend:
  bucket: acme-terraform-state-dev-12345678  # Add account ID or random suffix
```

#### 2. Permission Denied

**Error:**
```text
failed to create S3 client: operation error HeadBucket: AccessDenied
```

**Cause:** IAM identity lacks required S3 permissions.

**Solution:**
- Attach IAM policy with required permissions (see Security section)
- Verify identity: `aws sts get-caller-identity`
- Check CloudTrail for specific permission denied

#### 3. Invalid Region

**Error:**
```text
failed to create bucket: InvalidLocationConstraint
```

**Cause:** Region specified doesn't exist or is invalid.

**Solution:**
```yaml
backend:
  region: us-east-1  # Use valid AWS region
```

#### 4. Cross-Account Role Assumption Failed

**Error:**
```text
failed to create S3 client: operation error STS: AssumeRole, AccessDenied
```

**Cause:** Trust policy doesn't allow source identity to assume role.

**Solution:**
- Verify trust policy allows source account/role
- Check external ID if required
- Verify role ARN is correct

---

## Migration Guide

### Enabling S3 Backend Provisioning

**Step 1: Enable global feature flag (optional)**

```yaml
# atmos.yaml
settings:
  backends:
    auto_provision:
      enabled: true
```

**Step 2: Enable per-component**

```yaml
# stacks/dev.yaml
components:
  terraform:
    vpc:
      backend:
        bucket: acme-terraform-state-dev
        key: vpc/terraform.tfstate
        region: us-east-1

      provision:  # ADD THIS
        backend:
          enabled: true
```

**Step 3: Generate backend**

```bash
atmos terraform generate backend vpc --stack dev
```

**What happens:**
1. Atmos checks if bucket exists
2. Bucket doesn't exist → creates with secure defaults
3. Generates `backend.tf.json`
4. Ready for `terraform init`

### Upgrading to Production Backend

**Scenario:** Moving from auto-provisioned dev bucket to production-grade backend.

**Step 1: Provision production backend via Terraform module**

```yaml
# stacks/prod.yaml
components:
  terraform:
    # Provision production backend first
    prod-backend:
      component: terraform-aws-tfstate-backend
      backend_type: local  # Bootstrap with local state
      backend:
        path: ./local-state/backend.tfstate

      vars:
        bucket: acme-terraform-state-prod
        dynamodb_table: terraform-locks-prod
        s3_replication_enabled: true
        s3_replica_bucket_arn: "arn:aws:s3:::acme-terraform-state-prod-dr"
        enable_point_in_time_recovery: true
        sse_algorithm: "aws:kms"
        kms_master_key_id: "arn:aws:kms:us-east-1:123456789012:key/..."
```

**Step 2: Apply backend infrastructure**

```bash
atmos terraform apply prod-backend --stack prod
```

**Step 3: Update component to use production backend**

```yaml
# stacks/prod.yaml
components:
  terraform:
    vpc:
      backend:
        bucket: acme-terraform-state-prod      # New production bucket
        key: vpc/terraform.tfstate
        dynamodb_table: terraform-locks-prod
        kms_key_id: "arn:aws:kms:us-east-1:123456789012:key/..."
        # Remove provision block - backend already exists
```

**Step 4: Migrate state**

```bash
# Re-initialize with new backend
atmos terraform init vpc --stack prod -migrate-state

# Verify migration
atmos terraform state list vpc --stack prod
```

**Step 5: (Optional) Delete old auto-provisioned bucket**

```bash
# Only after confirming migration successful
aws s3 rb s3://acme-dev-state --force
```

---

## Performance Benchmarks

### Target Metrics

- **Bucket existence check**: <2 seconds
- **Bucket creation**: <30 seconds (including all settings)
- **Total provisioning time**: <1 minute
- **Cache hit rate**: >80% for repeated operations

### Optimization Strategies

1. **Client Caching**: Reuse S3 clients across operations
2. **Concurrent Settings**: Apply bucket settings in parallel (future optimization)
3. **Retry with Backoff**: Handle transient AWS API failures
4. **Context Timeouts**: Prevent hanging on slow API calls

---

## FAQ

### Q: Why not support DynamoDB table provisioning?

**A:** Terraform 1.10+ includes native S3 state locking, eliminating the need for DynamoDB. For users requiring DynamoDB (Terraform <1.10 or advanced features like point-in-time recovery), use the `terraform-aws-tfstate-backend` module.

### Q: Can I use custom KMS keys?

**A:** Not with auto-provisioning. Auto-provisioning uses AWS-managed encryption keys for simplicity. For custom KMS keys, use the `terraform-aws-tfstate-backend` module.

### Q: Is auto-provisioned bucket suitable for production?

**A:** No. Auto-provisioning is designed for development and testing. Production backends should use the `terraform-aws-tfstate-backend` Terraform module for advanced features like replication, custom KMS, lifecycle policies, and compliance controls.

### Q: What happens if bucket name is already taken?

**A:** S3 bucket names are globally unique across all AWS accounts. If the name is taken, you'll receive a clear error message. Use a more specific name (e.g., add account ID or random suffix).

### Q: Can I migrate from auto-provisioned to production backend?

**A:** Yes. Provision your production backend using the `terraform-aws-tfstate-backend` module, update your stack manifest, then run `terraform init -migrate-state`. See Migration Guide for detailed steps.

### Q: Does this work with Terraform Cloud?

**A:** The `cloud` backend type doesn't require provisioning (Terraform Cloud manages storage). Auto-provisioning only applies to self-managed backends (S3, GCS, Azure).

### Q: What permissions are required?

**A:** Minimal permissions: `s3:CreateBucket`, `s3:HeadBucket`, `s3:PutBucketVersioning`, `s3:PutBucketEncryption`, `s3:PutBucketPublicAccessBlock`, `s3:PutBucketTagging`. See Security section for complete IAM policy.

### Q: Can I provision buckets in different AWS accounts?

**A:** Yes, using role assumption. Configure `backend.assume_role.role_arn` to specify the target account role. The provisioner will assume the role and create the bucket in the target account.

### Q: What if I already have a bucket?

**A:** The provisioner is idempotent - if the bucket already exists, it returns without error. It will NOT modify existing bucket settings.

---

## CLI Usage

### Automatic Provisioning (Recommended)

Backend provisioned automatically when running Terraform commands:

```bash
# Backend provisioned automatically if provision.backend.enabled: true
atmos terraform apply vpc --stack dev

# Execution flow:
# 1. Auth setup (TerraformPreHook)
# 2. Backend provisioning (if enabled) ← Automatic
# 3. Terraform init
# 4. Terraform apply
```

### Manual Provisioning

Explicitly provision backend before Terraform execution:

```bash
# Provision S3 backend explicitly
atmos terraform backend create vpc --stack dev

# Then run Terraform
atmos terraform apply vpc --stack dev
```

**When to use manual provisioning:**
- CI/CD pipelines with separate provisioning stages
- Troubleshooting provisioning issues
- Batch provisioning for multiple components
- Pre-provisioning before large-scale deployments

### CI/CD Integration Examples

#### GitHub Actions

```yaml
name: Deploy Infrastructure

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/GitHubActions
          aws-region: us-east-1

      - name: Provision Backend
        run: |
          atmos terraform backend create vpc --stack dev
          atmos terraform backend create eks --stack dev
          atmos terraform backend create rds --stack dev
        # If any provisioning fails, workflow stops here

      - name: Deploy Infrastructure
        run: |
          atmos terraform apply vpc --stack dev
          atmos terraform apply eks --stack dev
          atmos terraform apply rds --stack dev
        # Only runs if provisioning succeeded
```

#### GitLab CI

```yaml
stages:
  - provision
  - deploy

provision_backend:
  stage: provision
  script:
    - atmos terraform backend create vpc --stack dev
  # Pipeline fails if exit code != 0

deploy_infrastructure:
  stage: deploy
  script:
    - atmos terraform apply vpc --stack dev
  # Only runs if provision stage succeeded
```

### Error Handling in CLI

**Provisioning failure stops execution:**

```bash
$ atmos terraform backend create vpc --stack dev

Running backend provisioner...
Creating S3 bucket 'acme-terraform-state-dev'...
Error: backend provisioning failed: failed to create bucket:
operation error S3: CreateBucket, https response error StatusCode: 403, AccessDenied

Hint: Verify AWS credentials have s3:CreateBucket permission
Required IAM permissions: s3:CreateBucket, s3:PutBucketVersioning, s3:PutBucketEncryption
Context:
  bucket: acme-terraform-state-dev
  region: us-east-1
  identity: dev-admin

Exit code: 3
```

**Terraform blocked if provisioning fails:**

```bash
$ atmos terraform apply vpc --stack dev

Authenticating...
Running backend provisioner...
Error: Provisioning failed - cannot proceed with terraform
provisioner 'backend' failed: backend provisioning failed

Exit code: 2
```

**Success output:**

```bash
$ atmos terraform backend create vpc --stack dev

Running backend provisioner...
Creating S3 bucket 'acme-terraform-state-dev' with secure defaults...
Enabling bucket versioning...
Enabling bucket encryption (AES-256)...
Blocking public access...
Applying resource tags...
✓ Successfully created S3 bucket 'acme-terraform-state-dev'

Exit code: 0
```

**Idempotent operation:**

```bash
$ atmos terraform backend create vpc --stack dev

Running backend provisioner...
S3 bucket 'acme-terraform-state-dev' already exists (idempotent)
✓ Backend provisioning completed

Exit code: 0
```

---

## Error Categories and Exit Codes

### Error Categories

#### 1. Configuration Errors (Exit Code 2)

**Missing bucket name:**
```text
Error: backend.bucket is required in backend configuration

Hint: Add bucket name to stack manifest
Example:
  backend:
    bucket: my-terraform-state
    key: vpc/terraform.tfstate
    region: us-east-1

Exit code: 2
```

**Missing region:**
```text
Error: backend.region is required in backend configuration

Hint: Specify AWS region for S3 bucket
Example:
  backend:
    region: us-east-1

Exit code: 2
```

#### 2. Permission Errors (Exit Code 3)

**IAM permission denied:**
```text
Error: failed to create bucket: AccessDenied

Hint: Verify AWS credentials have s3:CreateBucket permission
Required IAM permissions:
  - s3:CreateBucket
  - s3:HeadBucket
  - s3:PutBucketVersioning
  - s3:PutBucketEncryption
  - s3:PutBucketPublicAccessBlock
  - s3:PutBucketTagging

Check IAM policy for identity: dev-admin
Context:
  bucket: acme-terraform-state-dev
  region: us-east-1

Exit code: 3
```

**Cross-account role assumption failed:**
```text
Error: failed to create S3 client: operation error STS: AssumeRole, AccessDenied

Hint: Verify trust policy allows source identity to assume role
Required:
  - Trust policy in target account must allow source account
  - Source identity must have sts:AssumeRole permission

Context:
  source_identity: dev-admin
  target_role: arn:aws:iam::999999999999:role/TerraformStateAdmin

Exit code: 3
```

#### 3. Resource Conflicts (Exit Code 4)

**Bucket name already taken:**
```text
Error: failed to create bucket: BucketAlreadyExists

Hint: S3 bucket names are globally unique across all AWS accounts
Try a different bucket name, for example:
  - acme-terraform-state-dev-123456789012 (add account ID)
  - acme-terraform-state-dev-us-east-1 (add region)
  - acme-terraform-state-dev-a1b2c3 (add random suffix)

Context:
  bucket: acme-terraform-state-dev
  region: us-east-1

Exit code: 4
```

#### 4. Network Errors (Exit Code 5)

**Connection timeout:**
```text
Error: failed to create bucket: RequestTimeout

Hint: Check network connectivity to AWS API endpoints
Possible causes:
  - Network firewall blocking AWS API access
  - VPN/proxy configuration issues
  - AWS service outage in region

Context:
  bucket: acme-terraform-state-dev
  region: us-east-1
  endpoint: s3.us-east-1.amazonaws.com

Exit code: 5
```

### Error Recovery Strategies

**Permission Issues:**
1. Check IAM policy attached to identity
2. Verify trust policy for cross-account roles
3. Check CloudTrail for specific denied actions
4. Attach required permissions (see Security section)

**Bucket Name Conflicts:**
1. Use more specific naming (add account ID or region)
2. Add random suffix for uniqueness
3. Check existing buckets: `aws s3 ls`

**Network Issues:**
1. Verify AWS CLI connectivity: `aws s3 ls`
2. Check firewall/proxy settings
3. Try different region
4. Check AWS service health dashboard

### Exit Code Summary

| Exit Code | Category | Action |
|-----------|----------|--------|
| 0 | Success | Continue to Terraform execution |
| 1 | General error | Check error message for details |
| 2 | Configuration | Fix stack manifest configuration |
| 3 | Permission | Grant required IAM permissions |
| 4 | Resource conflict | Change resource name |
| 5 | Network | Check network connectivity |

---

## Timeline

### Week 1: Implementation

- **Day 1-2**: Core provisioner implementation
  - `ProvisionS3Backend()` function
  - `checkS3BucketExists()` helper
  - `provisionS3BucketWithDefaults()` helper
  - Client caching logic

- **Day 3-4**: Unit tests
  - Mock S3 client
  - Test bucket creation
  - Test idempotency
  - Test error handling
  - Test role assumption

- **Day 5**: Integration tests
  - Localstack setup
  - Real AWS tests (optional)
  - Cleanup helpers

- **Weekend**: Documentation
  - User guide
  - Migration guide
  - FAQ
  - Examples

### Success Criteria

- ✅ All unit tests passing (>90% coverage)
- ✅ Integration tests passing (localstack)
- ✅ Manual testing complete
- ✅ Documentation published
- ✅ PR reviewed and approved

---

## Related Documents

- **[Provisioner System](./provisioner-system.md)** - Generic provisioner infrastructure
- **[Backend Provisioner](./backend-provisioner.md)** - Backend provisioner interface

---

## Appendix: Example Usage

### Development Workflow

```bash
# 1. Configure stack with auto-provision
vim stacks/dev.yaml

# 2. Generate backend (creates bucket if needed)
atmos terraform generate backend vpc --stack dev

# 3. Initialize Terraform (backend exists)
atmos terraform init vpc --stack dev

# 4. Apply infrastructure
atmos terraform apply vpc --stack dev
```

### Multi-Environment Setup

```yaml
# Base configuration with auto-provision
# stacks/catalog/terraform/base.yaml
components:
  terraform:
    base:
      provision:
        backend:
          enabled: true  # All dev/test environments inherit

# Development environment
# stacks/dev.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [base]
      backend:
        bucket: acme-dev-state
        # provision.backend.enabled: true inherited

# Production environment (no auto-provision)
# stacks/prod.yaml
components:
  terraform:
    vpc:
      backend:
        bucket: acme-prod-state  # Provisioned via terraform-aws-tfstate-backend
        # No provision block
```

---

**End of PRD**

**Status:** Ready for Implementation
**Estimated Timeline:** 1 week
**Next Steps:** Begin implementation of core provisioner functions
