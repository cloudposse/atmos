package backend

import (
	"context"
	"fmt"
	"strings"

	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/aws"
	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/service/s3"
	//nolint:depguard
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
//
//revive:disable:cyclomatic
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

//revive:enable:cyclomatic

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

// deleteAllObjects deletes all objects and versions from a bucket in batches.
func deleteAllObjects(ctx context.Context, client S3ClientAPI, bucket string) error {
	var continuationKeyMarker *string
	var continuationVersionMarker *string

	for {
		// List objects and versions to delete.
		output, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket:          aws.String(bucket),
			KeyMarker:       continuationKeyMarker,
			VersionIdMarker: continuationVersionMarker,
			MaxKeys:         aws.Int32(1000), // AWS limit for batch delete.
		})
		if err != nil {
			return fmt.Errorf(errFormat, errUtils.ErrListObjects, err)
		}

		// Build list of objects to delete (versions + delete markers).
		var objectsToDelete []types.ObjectIdentifier

		for i := range output.Versions {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key:       output.Versions[i].Key,
				VersionId: output.Versions[i].VersionId,
			})
		}

		for i := range output.DeleteMarkers {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key:       output.DeleteMarkers[i].Key,
				VersionId: output.DeleteMarkers[i].VersionId,
			})
		}

		// Delete this batch if there are objects.
		if len(objectsToDelete) > 0 {
			_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &types.Delete{
					Objects: objectsToDelete,
					Quiet:   aws.Bool(true), // Don't return deleted objects in response.
				},
			})
			if err != nil {
				return fmt.Errorf(errFormat, errUtils.ErrDeleteObjects, err)
			}
		}

		// Check if there are more pages.
		if !aws.ToBool(output.IsTruncated) {
			break
		}

		continuationKeyMarker = output.NextKeyMarker
		continuationVersionMarker = output.NextVersionIdMarker
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
