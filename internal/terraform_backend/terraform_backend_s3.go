package terraform_backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awsUtils "github.com/cloudposse/atmos/internal/aws_utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// maxRetryCount defines the max attempts to read a state file from an S3 bucket.
const maxRetryCount = 2

// GetS3BackendAssumeRoleArn returns the s3 backend role ARN from the S3 backend config.
// https://developer.hashicorp.com/terraform/language/backend/s3#assume-role-configuration
func GetS3BackendAssumeRoleArn(backend *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetS3BackendAssumeRoleArn")()

	var roleArn string
	roleArnAttribute := "role_arn"

	// Check `assume_role.role_arn`.
	if assumeRoleSection, ok := (*backend)["assume_role"].(map[string]any); ok {
		if len(assumeRoleSection) > 0 {
			roleArn = GetBackendAttribute(&assumeRoleSection, roleArnAttribute)
		}
	}
	// If `assume_role.role_arn` is not set, fallback to `role_arn`.
	if roleArn == "" {
		roleArn = GetBackendAttribute(backend, roleArnAttribute)
	}
	return roleArn
}

// S3API defines an interface for interacting with S3, including retrieving objects with context and configuration options.
type S3API interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// s3ClientCache caches the S3 clients based on a deterministic cache key.
// It's a map[string]S3API.
var s3ClientCache sync.Map

func getCachedS3Client(backend *map[string]any) (S3API, error) {
	region := GetBackendAttribute(backend, "region")
	roleArn := GetS3BackendAssumeRoleArn(backend)

	// Build a deterministic cache key
	cacheKey := fmt.Sprintf("region=%s;role_arn=%s", region, roleArn)

	// Check the cache
	if cached, ok := s3ClientCache.Load(cacheKey); ok {
		return cached.(S3API), nil
	}

	// Build the S3 client if not cached
	// 30 sec timeout to configure an AWS client (and assume a role if provided).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The minimum `assume role` duration allowed by AWS is 15 minutes
	cfg, err := awsUtils.LoadAWSConfig(ctx, region, roleArn, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg)
	s3ClientCache.Store(cacheKey, s3Client)
	return s3Client, nil
}

// ReadTerraformBackendS3 reads the Terraform state file from the configured S3 backend.
// If the state file does not exist in the bucket, the function returns `nil`.
func ReadTerraformBackendS3(
	_ *schema.AtmosConfiguration,
	componentSections *map[string]any,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendS3")()

	backend := GetComponentBackend(componentSections)

	s3Client, err := getCachedS3Client(&backend)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendS3Internal(s3Client, componentSections, &backend)
}

// ReadTerraformBackendS3Internal accepts an S3 client and reads the Terraform state file from the configured S3 backend.
func ReadTerraformBackendS3Internal(
	s3Client S3API,
	componentSections *map[string]any,
	backend *map[string]any,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendS3Internal")()

	// Path to the tfstate file in the s3 bucket.
	tfStateFilePath := path.Join(
		GetBackendAttribute(backend, "workspace_key_prefix"),
		GetTerraformWorkspace(componentSections),
		GetBackendAttribute(backend, "key"),
	)

	bucket := GetBackendAttribute(backend, "bucket")

	var lastErr error
	for attempt := 0; attempt <= maxRetryCount; attempt++ {
		// 30 sec timeout to read the state file from the S3 bucket.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(tfStateFilePath),
		})
		if err != nil {
			// Check if the error is because the object doesn't exist.
			// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error.
			var nsk *types.NoSuchKey
			if errors.As(err, &nsk) {
				log.Debug("Terraform state file doesn't exist in the S3 bucket; returning 'null'", "file", tfStateFilePath, "bucket", bucket)
				return nil, nil
			}

			lastErr = err
			if attempt < maxRetryCount {
				log.Debug("Failed to read Terraform state file from the S3 bucket", "attempt", attempt+1, "file", tfStateFilePath, "bucket", bucket, "error", err)
				time.Sleep(time.Second * 2) // backoff
				continue
			}
			return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromS3, lastErr)
		}

		content, err := io.ReadAll(output.Body)
		_ = output.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errUtils.ErrReadS3ObjectBody, err)
		}
		return content, nil
	}

	return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromS3, lastErr)
}
