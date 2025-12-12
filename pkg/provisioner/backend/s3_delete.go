package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// DeleteS3Backend deletes an S3 backend and all its contents.
//
// Safety mechanisms:
// - Requires force=true flag (enforced at command level)
// - Lists all objects and versions before deletion
// - Detects and counts .tfstate files
// - Warns user about data loss
// - Deletes all objects/versions before bucket deletion
//
// Process:
// 1. Validate bucket configuration
// 2. Check bucket exists
// 3. List all objects and versions
// 4. Count state files for warning
// 5. Delete all objects in batches (AWS limit: 1000 per request)
// 6. Delete bucket itself
//
// This operation is irreversible. State files will be permanently lost.
func DeleteS3Backend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
	force bool,
) error {
	defer perf.Track(atmosConfig, "backend.DeleteS3Backend")()

	// Require force flag to prevent accidental deletion.
	if !force {
		return fmt.Errorf("%w: use --force flag to confirm deletion", errUtils.ErrForceRequired)
	}

	// Extract and validate required configuration.
	config, err := extractS3Config(backendConfig)
	if err != nil {
		return err
	}

	_ = ui.Info(fmt.Sprintf("Deleting S3 backend: bucket=%s region=%s", config.bucket, config.region))

	// Load AWS configuration with auth context.
	awsConfig, err := loadAWSConfigWithAuth(ctx, config.region, config.roleArn, authContext)
	if err != nil {
		return fmt.Errorf(errFormat, errUtils.ErrLoadAWSConfig, err)
	}

	// Create S3 client.
	client := s3.NewFromConfig(awsConfig)

	// Check if bucket exists before attempting deletion.
	exists, err := bucketExists(ctx, client, config.bucket)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%w: bucket '%s' does not exist", errUtils.ErrBackendNotFound, config.bucket)
	}

	// List all objects and versions to get count and detect state files.
	objectCount, stateFileCount, err := listAllObjects(ctx, client, config.bucket)
	if err != nil {
		return err
	}

	// Show warning and delete all contents.
	if err := deleteBackendContents(ctx, client, config.bucket, objectCount, stateFileCount); err != nil {
		return err
	}

	// Delete the bucket itself.
	if err := deleteBucket(ctx, client, config.bucket); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("✓ Backend deleted: bucket '%s' and all contents removed", config.bucket))
	return nil
}

// deleteBackendContents displays warnings and deletes all objects from a bucket.
func deleteBackendContents(ctx context.Context, client S3ClientAPI, bucket string, objectCount, stateFileCount int) error {
	if objectCount == 0 {
		return nil
	}

	// Show warning about what will be deleted.
	showDeletionWarning(bucket, objectCount, stateFileCount)

	// Delete all objects and versions.
	if err := deleteAllObjects(ctx, client, bucket); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("Deleted %d object(s) from bucket '%s'", objectCount, bucket))
	return nil
}

// showDeletionWarning displays a warning message about pending deletion.
func showDeletionWarning(bucket string, objectCount, stateFileCount int) {
	msg := fmt.Sprintf("⚠ Deleting backend will permanently remove %d object(s) from bucket '%s'",
		objectCount, bucket)
	if stateFileCount > 0 {
		msg += fmt.Sprintf(" (including %d Terraform state file(s))", stateFileCount)
	}
	_ = ui.Warning(msg)
	_ = ui.Warning("This action cannot be undone")
}

// listAllObjects lists all objects and versions in a bucket, returning counts.
func listAllObjects(ctx context.Context, client S3ClientAPI, bucket string) (totalObjects int, stateFiles int, err error) {
	var continuationKeyMarker *string
	var continuationVersionMarker *string

	for {
		output, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket:          aws.String(bucket),
			KeyMarker:       continuationKeyMarker,
			VersionIdMarker: continuationVersionMarker,
		})
		if err != nil {
			return 0, 0, fmt.Errorf(errFormat, errUtils.ErrListObjects, err)
		}

		// Count versions (actual objects).
		totalObjects += len(output.Versions)
		for i := range output.Versions {
			if output.Versions[i].Key != nil && strings.HasSuffix(*output.Versions[i].Key, ".tfstate") {
				stateFiles++
			}
		}

		// Count delete markers (also need to be deleted).
		totalObjects += len(output.DeleteMarkers)

		// Check if there are more pages.
		if !aws.ToBool(output.IsTruncated) {
			break
		}

		continuationKeyMarker = output.NextKeyMarker
		continuationVersionMarker = output.NextVersionIdMarker
	}

	return totalObjects, stateFiles, nil
}

// collectObjectIdentifiers builds a list of object identifiers from versions and delete markers.
func collectObjectIdentifiers(output *s3.ListObjectVersionsOutput) []types.ObjectIdentifier {
	objects := make([]types.ObjectIdentifier, 0, len(output.Versions)+len(output.DeleteMarkers))
	for i := range output.Versions {
		objects = append(objects, types.ObjectIdentifier{
			Key: output.Versions[i].Key, VersionId: output.Versions[i].VersionId,
		})
	}
	for i := range output.DeleteMarkers {
		objects = append(objects, types.ObjectIdentifier{
			Key: output.DeleteMarkers[i].Key, VersionId: output.DeleteMarkers[i].VersionId,
		})
	}
	return objects
}

// deleteBatch deletes a batch of objects and handles partial failures.
func deleteBatch(ctx context.Context, client S3ClientAPI, bucket string, objects []types.ObjectIdentifier) error {
	if len(objects) == 0 {
		return nil
	}
	resp, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
	})
	if err != nil {
		return fmt.Errorf(errFormat, errUtils.ErrDeleteObjects, err)
	}
	// Handle partial failures - DeleteObjects can return HTTP 200 with per-key errors.
	if resp != nil && len(resp.Errors) > 0 {
		e := resp.Errors[0]
		return fmt.Errorf("%w: key=%s version=%s code=%s message=%s",
			errUtils.ErrDeleteObjects, aws.ToString(e.Key), aws.ToString(e.VersionId),
			aws.ToString(e.Code), aws.ToString(e.Message))
	}
	return nil
}

// deleteAllObjects deletes all objects and versions from a bucket in batches.
func deleteAllObjects(ctx context.Context, client S3ClientAPI, bucket string) error {
	var keyMarker, versionMarker *string
	for {
		output, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket: aws.String(bucket), KeyMarker: keyMarker,
			VersionIdMarker: versionMarker, MaxKeys: aws.Int32(1000),
		})
		if err != nil {
			return fmt.Errorf(errFormat, errUtils.ErrListObjects, err)
		}
		if err := deleteBatch(ctx, client, bucket, collectObjectIdentifiers(output)); err != nil {
			return err
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		keyMarker, versionMarker = output.NextKeyMarker, output.NextVersionIdMarker
	}
	return nil
}

// deleteBucket deletes an empty S3 bucket.
func deleteBucket(ctx context.Context, client S3ClientAPI, bucket string) error {
	_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf(errFormat, errUtils.ErrDeleteBucket, err)
	}
	return nil
}
