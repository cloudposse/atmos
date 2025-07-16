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
)

// GetTerraformBackendS3 returns the Terraform state file from the configured S3 backend.
func GetTerraformBackendS3(
	backendInfo *TerraformBackendInfo,
) (map[string]any, error) {
	// 5 sec timeout to read the state file from the S3 bucket.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Determine the full path to the tfstate file
	tfStateFilePath := path.Join(
		backendInfo.S3.WorkspaceKeyPrefix,
		backendInfo.Workspace,
		"terraform.tfstate",
	)

	// Load AWS config and assume the backend IAM role (using the AWS SDK).
	awsConfig, err := awsUtils.LoadAWSConfig(ctx, backendInfo.S3.Region, backendInfo.S3.RoleArn)
	if err != nil {
		return nil, err
	}

	// Create an S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	// Get the object from S3.
	output, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(backendInfo.S3.Bucket),
		Key:    aws.String(tfStateFilePath),
	})
	if err != nil {
		// Check if the error is because the object doesn't exist.
		// If the state file does not exist (the component in the stack has not been provisioned yet),
		// return a `nil` result and no error.
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil
		}
		// If any other error, return it.
		return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromS3, err)
	}

	defer output.Body.Close()

	content, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrReadS3ObjectBody, err)
	}

	data, err := ProcessTerraformStateFile(content)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror: %v", errUtils.ErrProcessTerraformStateFile, tfStateFilePath, err)
	}

	return data, nil
}
