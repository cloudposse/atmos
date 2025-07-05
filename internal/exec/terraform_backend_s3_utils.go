package exec

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// GetTerraformBackendS3 returns the Terraform state from the configured S3 backend.
func GetTerraformBackendS3(
	backendInfo *TerraformBackendInfo,
) (map[string]any, error) {
	ctx := context.Background()

	// Determine the full path to the tfstate file
	tfStateFilePath := path.Join(
		backendInfo.S3.WorkspaceKeyPrefix,
		backendInfo.Workspace,
		"terraform.tfstate",
	)

	// Load AWS config
	awsConfig, err := loadAWSConfig(ctx, backendInfo.S3.Region, backendInfo.S3.RoleArn)
	if err != nil {
		return nil, err
	}

	// Create an S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	// Get the object from S3
	output, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(backendInfo.S3.Bucket),
		Key:    aws.String(tfStateFilePath),
	})
	if err != nil {
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
