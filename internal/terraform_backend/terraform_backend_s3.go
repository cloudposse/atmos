package terraform_backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awsUtils "github.com/cloudposse/atmos/internal/aws_utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxRetryCount = 2

// GetS3BackendAssumeRoleArn returns the s3 backend role ARN from the S3 backend config.
// https://developer.hashicorp.com/terraform/language/backend/s3#assume-role-configuration
func GetS3BackendAssumeRoleArn(backend *map[string]any) string {
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

// ReadTerraformBackendS3 reads the Terraform state file from the configured S3 backend.
// If the state file does not exist in the bucket, the function returns `nil`.
func ReadTerraformBackendS3(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
) ([]byte, error) {
	backend := GetComponentBackend(componentSections)

	region := GetBackendAttribute(&backend, "region")
	roleArn := GetS3BackendAssumeRoleArn(&backend)

	// 30 sec timeout to configure an AWS client (and assume a role if provided).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load AWS config and assume the backend IAM role (using the AWS SDK).
	awsConfig, err := awsUtils.LoadAWSConfig(ctx, region, roleArn)
	if err != nil {
		return nil, err
	}

	// Create an S3 client.
	s3Client := s3.NewFromConfig(awsConfig)

	return ReadTerraformBackendS3Internal(s3Client, componentSections, &backend)
}

// ReadTerraformBackendS3Internal accepts an S3 client and reads the Terraform state file from the configured S3 backend.
func ReadTerraformBackendS3Internal(
	s3Client S3API,
	componentSections *map[string]any,
	backend *map[string]any,
) ([]byte, error) {
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
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
				return nil, nil
			}
			lastErr = err
			if attempt < maxRetryCount {
				time.Sleep(time.Second * 2) // backoff
				continue
			}
			return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromS3, lastErr)
		}

		defer output.Body.Close()
		content, err := io.ReadAll(output.Body)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errUtils.ErrReadS3ObjectBody, err)
		}
		return content, nil
	}

	return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromS3, lastErr)
}
