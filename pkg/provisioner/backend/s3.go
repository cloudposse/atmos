package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/aws"
	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/service/s3"
	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/aws_utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const errFormat = "%w: %w"

// s3Config holds S3 backend configuration.
type s3Config struct {
	bucket  string
	region  string
	roleArn string
}

func init() {
	// Register S3 backend provisioner.
	RegisterBackendProvisioner("s3", ProvisionS3Backend)
}

// ProvisionS3Backend provisions an S3 backend with opinionated, hardcoded defaults.
//
// Hardcoded features:
// - Versioning: ENABLED (always)
// - Encryption: AES-256 (AWS-managed keys, always)
// - Public Access: BLOCKED (all 4 settings, always)
// - Locking: Native S3 locking (Terraform 1.10+, no DynamoDB)
// - Tags: Standard tags (Name, ManagedBy, always)
//
// No configuration options beyond enabled: true.
// For production use, migrate to terraform-aws-tfstate-backend module.
func ProvisionS3Backend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "backend.ProvisionS3Backend")()

	// Extract and validate required configuration.
	config, err := extractS3Config(backendConfig)
	if err != nil {
		return err
	}

	_ = ui.Info(fmt.Sprintf("Provisioning S3 backend: bucket=%s region=%s", config.bucket, config.region))

	// Load AWS configuration with auth context.
	awsConfig, err := loadAWSConfigWithAuth(ctx, config.region, config.roleArn, authContext)
	if err != nil {
		return errUtils.Build(provisioner.ErrLoadAWSConfig).
			WithHint("Check AWS credentials are configured correctly").
			WithHintf("Verify AWS region '%s' is valid", config.region).
			WithHint("If using --identity flag, ensure the identity is authenticated").
			WithContext("region", config.region).
			WithContext("bucket", config.bucket).
			Err()
	}

	// Create S3 client.
	client := s3.NewFromConfig(awsConfig)

	// Check if bucket exists and create if needed.
	bucketAlreadyExisted, err := ensureBucket(ctx, client, config.bucket, config.region)
	if err != nil {
		return err
	}

	// Apply hardcoded defaults.
	// If bucket already existed, warn that settings may be overwritten.
	if err := applyS3BucketDefaults(ctx, client, config.bucket, bucketAlreadyExisted); err != nil {
		return fmt.Errorf(errFormat, provisioner.ErrApplyBucketDefaults, err)
	}

	_ = ui.Success(fmt.Sprintf("S3 backend provisioned successfully: %s", config.bucket))
	return nil
}

// extractS3Config extracts and validates required S3 configuration.
func extractS3Config(backendConfig map[string]any) (*s3Config, error) {
	// Extract bucket name.
	bucketVal, ok := backendConfig["bucket"].(string)
	if !ok || bucketVal == "" {
		return nil, fmt.Errorf("%w", provisioner.ErrBucketRequired)
	}

	// Extract region.
	regionVal, ok := backendConfig["region"].(string)
	if !ok || regionVal == "" {
		return nil, fmt.Errorf("%w", provisioner.ErrRegionRequired)
	}

	// Extract role ARN if specified (optional).
	var roleArnVal string
	if assumeRole, ok := backendConfig["assume_role"].(map[string]any); ok {
		if arn, ok := assumeRole["role_arn"].(string); ok {
			roleArnVal = arn
		}
	}

	return &s3Config{
		bucket:  bucketVal,
		region:  regionVal,
		roleArn: roleArnVal,
	}, nil
}

// ensureBucket checks if bucket exists and creates it if needed.
// Returns (true, nil) if bucket already existed, (false, nil) if bucket was created, (_, error) on failure.
func ensureBucket(ctx context.Context, client *s3.Client, bucket, region string) (bool, error) {
	exists, err := bucketExists(ctx, client, bucket)
	if err != nil {
		return false, fmt.Errorf(errFormat, provisioner.ErrCheckBucketExist, err)
	}

	if exists {
		_ = ui.Info(fmt.Sprintf("S3 bucket %s already exists, skipping creation", bucket))
		return true, nil
	}

	// Create bucket.
	if err := createBucket(ctx, client, bucket, region); err != nil {
		return false, fmt.Errorf(errFormat, provisioner.ErrCreateBucket, err)
	}
	_ = ui.Success(fmt.Sprintf("Created S3 bucket: %s", bucket))
	return false, nil
}

// loadAWSConfigWithAuth loads AWS configuration with optional role assumption.
func loadAWSConfigWithAuth(ctx context.Context, region, roleArn string, authContext *schema.AuthContext) (aws.Config, error) {
	// Extract AWS auth context if available.
	var awsAuthContext *schema.AWSAuthContext
	if authContext != nil && authContext.AWS != nil {
		awsAuthContext = authContext.AWS
	}

	// Use 1-hour duration for assumed role (default).
	assumeRoleDuration := 1 * time.Hour

	// Load AWS config with auth context and optional role assumption.
	return aws_utils.LoadAWSConfigWithAuth(ctx, region, roleArn, assumeRoleDuration, awsAuthContext)
}

// bucketExists checks if an S3 bucket exists.
// Returns (false, nil) if bucket doesn't exist (404).
// Returns (false, error) for permission denied, network issues, or other errors.
func bucketExists(ctx context.Context, client *s3.Client, bucket string) (bool, error) {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Check if error is bucket not found (404).
		var notFound *types.NotFound
		var noSuchBucket *types.NoSuchBucket
		if errors.As(err, &notFound) || errors.As(err, &noSuchBucket) {
			return false, nil
		}

		// Check for HTTP status code to distinguish between different error types.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "Forbidden", "AccessDenied":
				// 403 Forbidden - permission denied.
				return false, errUtils.Build(errUtils.ErrS3BucketAccessDenied).
					WithHint("Check AWS IAM permissions for s3:ListBucket action").
					WithHintf("Verify that your credentials have access to bucket '%s'", bucket).
					WithContext("bucket", bucket).
					WithContext("operation", "HeadBucket").
					Err()
			}
		}

		// Check for HTTP-level errors using response metadata.
		var respErr interface{ HTTPStatusCode() int }
		if errors.As(err, &respErr) {
			statusCode := respErr.HTTPStatusCode()
			switch statusCode {
			case http.StatusForbidden:
				// 403 Forbidden.
				return false, errUtils.Build(errUtils.ErrS3BucketAccessDenied).
					WithHint("Check AWS IAM permissions for s3:ListBucket action").
					WithHintf("Verify that your credentials have access to bucket '%s'", bucket).
					WithContext("bucket", bucket).
					WithContext("status_code", fmt.Sprintf("%d", statusCode)).
					Err()
			case http.StatusNotFound:
				// 404 Not Found (shouldn't reach here due to type checks above, but be defensive).
				return false, nil
			}
		}

		// Network or other transient error.
		return false, errUtils.Build(provisioner.ErrCheckBucketExist).
			WithHint("Check network connectivity to AWS S3").
			WithHint("Verify AWS region is correct").
			WithHintf("Try again - this may be a transient network issue").
			WithContext("bucket", bucket).
			WithContext("error", err.Error()).
			Err()
	}

	return true, nil
}

// createBucket creates an S3 bucket.
func createBucket(ctx context.Context, client *s3.Client, bucket, region string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	// LocationConstraint is required for all regions except us-east-1.
	if region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err := client.CreateBucket(ctx, input)
	return err
}

// applyS3BucketDefaults applies hardcoded defaults to an S3 bucket.
//
// IMPORTANT: This function always overwrites existing settings with opinionated defaults:
// - Versioning: ENABLED
// - Encryption: AES-256 (replaces any existing encryption including KMS)
// - Public Access: BLOCKED (all 4 settings)
// - Tags: Replaces entire tag set with Name and ManagedBy=Atmos only
//
// If the bucket already existed (alreadyExisted=true), warnings are logged to inform the user
// that existing settings are being modified.
func applyS3BucketDefaults(ctx context.Context, client *s3.Client, bucket string, alreadyExisted bool) error {
	// Warn user if modifying pre-existing bucket settings.
	if alreadyExisted {
		_ = ui.Warning(fmt.Sprintf("Applying Atmos defaults to existing bucket '%s'", bucket))
		_ = ui.Write("  - Versioning will be ENABLED")
		_ = ui.Write("  - Encryption will be set to AES-256 (existing KMS encryption will be replaced)")
		_ = ui.Write("  - Public access will be BLOCKED (all 4 settings)")
		_ = ui.Write("  - Tags will be replaced with: Name, ManagedBy=Atmos")
	}

	// 1. Enable versioning (ALWAYS).
	if err := enableVersioning(ctx, client, bucket); err != nil {
		return fmt.Errorf(errFormat, provisioner.ErrEnableVersioning, err)
	}

	// 2. Enable encryption with AES-256 (ALWAYS).
	// NOTE: This replaces any existing encryption configuration, including KMS.
	if err := enableEncryption(ctx, client, bucket); err != nil {
		return fmt.Errorf(errFormat, provisioner.ErrEnableEncryption, err)
	}

	// 3. Block public access (ALWAYS).
	if err := blockPublicAccess(ctx, client, bucket); err != nil {
		return fmt.Errorf(errFormat, provisioner.ErrBlockPublicAccess, err)
	}

	// 4. Apply standard tags (ALWAYS).
	// NOTE: This replaces the entire tag set. Existing tags are not preserved.
	if err := applyTags(ctx, client, bucket); err != nil {
		return fmt.Errorf(errFormat, provisioner.ErrApplyTags, err)
	}

	return nil
}

// enableVersioning enables versioning on an S3 bucket.
func enableVersioning(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: types.BucketVersioningStatusEnabled,
		},
	})
	return err
}

// enableEncryption enables AES-256 encryption on an S3 bucket.
func enableEncryption(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
						SSEAlgorithm: types.ServerSideEncryptionAes256,
					},
					BucketKeyEnabled: aws.Bool(true),
				},
			},
		},
	})
	return err
}

// blockPublicAccess blocks all public access to an S3 bucket.
func blockPublicAccess(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	})
	return err
}

// applyTags applies standard tags to an S3 bucket.
func applyTags(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucket),
		Tagging: &types.Tagging{
			TagSet: []types.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(bucket),
				},
				{
					Key:   aws.String("ManagedBy"),
					Value: aws.String("Atmos"),
				},
			},
		},
	})
	return err
}
