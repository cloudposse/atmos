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
// Safety mechanisms include requiring force=true flag, listing all objects and versions
// before deletion, detecting and counting .tfstate files, warning user about data loss,
// and deleting all objects/versions before bucket deletion.
//
// The process validates bucket configuration, checks bucket exists, lists all objects
// and versions, counts state files for warning, deletes all objects in batches
// (AWS limit: 1000 per request), and finally deletes the bucket itself.
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

	if !force {
		return errForceRequired()
	}

	config, err := extractS3Config(backendConfig)
	if err != nil {
		return err
	}

	_ = ui.Info(fmt.Sprintf("Deleting S3 backend: bucket=%s region=%s", config.bucket, config.region))

	client, err := createS3ClientForDeletion(ctx, config, authContext)
	if err != nil {
		return err
	}

	if err := validateBucketExistsForDeletion(ctx, client, config); err != nil {
		return err
	}

	if err := deleteS3BucketAndContents(ctx, client, config.bucket); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("✓ Backend deleted: bucket '%s' and all contents removed", config.bucket))
	return nil
}

// errForceRequired returns an error indicating --force flag is required.
func errForceRequired() error {
	return errUtils.Build(errUtils.ErrForceRequired).
		WithExplanation("Backend deletion requires explicit confirmation").
		WithHint("Use --force flag to confirm you want to permanently delete the backend").
		Err()
}

// createS3ClientForDeletion loads AWS config and creates an S3 client.
func createS3ClientForDeletion(ctx context.Context, config *s3Config, authContext *schema.AuthContext) (S3ClientAPI, error) {
	awsConfig, err := loadAWSConfigWithAuth(ctx, config.region, config.roleArn, authContext)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrLoadAWSConfig).
			WithCause(err).
			WithExplanation("Failed to load AWS configuration for backend deletion").
			WithContext("region", config.region).
			WithHint("Check AWS credentials and region configuration").
			Err()
	}
	return s3.NewFromConfig(awsConfig), nil
}

// validateBucketExistsForDeletion checks if the bucket exists before deletion.
func validateBucketExistsForDeletion(ctx context.Context, client S3ClientAPI, config *s3Config) error {
	exists, err := bucketExists(ctx, client, config.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return errUtils.Build(errUtils.ErrBackendNotFound).
			WithExplanation("Cannot delete backend - bucket does not exist").
			WithContext("bucket", config.bucket).
			WithContext("region", config.region).
			WithHint("Verify the bucket name in your backend configuration").
			Err()
	}
	return nil
}

// deleteS3BucketAndContents lists, warns, deletes objects, and deletes the bucket.
func deleteS3BucketAndContents(ctx context.Context, client S3ClientAPI, bucket string) error {
	objectCount, stateFileCount, err := listAllObjects(ctx, client, bucket)
	if err != nil {
		return err
	}

	if err := deleteBackendContents(ctx, client, bucket, objectCount, stateFileCount); err != nil {
		return err
	}

	return deleteBucket(ctx, client, bucket)
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
			return 0, 0, errUtils.Build(errUtils.ErrListObjects).
				WithCause(err).
				WithExplanation("Failed to list objects in bucket").
				WithContext("bucket", bucket).
				WithHint("Check IAM permissions for s3:ListBucketVersions").
				Err()
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
		return errUtils.Build(errUtils.ErrDeleteObjects).
			WithCause(err).
			WithExplanation("Failed to delete objects from bucket").
			WithContext("bucket", bucket).
			WithHint("Check IAM permissions for s3:DeleteObject and s3:DeleteObjectVersion").
			Err()
	}
	// Handle partial failures - DeleteObjects can return HTTP 200 with per-key errors.
	if resp != nil && len(resp.Errors) > 0 {
		e := resp.Errors[0]
		return errUtils.Build(errUtils.ErrDeleteObjects).
			WithExplanation("Partial failure when deleting objects").
			WithContext("bucket", bucket).
			WithContext("key", aws.ToString(e.Key)).
			WithContext("version", aws.ToString(e.VersionId)).
			WithContext("code", aws.ToString(e.Code)).
			WithContext("message", aws.ToString(e.Message)).
			WithHint("Check object-level permissions or bucket policies").
			Err()
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
			return errUtils.Build(errUtils.ErrListObjects).
				WithCause(err).
				WithExplanation("Failed to list object versions for deletion").
				WithContext("bucket", bucket).
				WithHint("Check IAM permissions for s3:ListBucketVersions").
				Err()
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
		return errUtils.Build(errUtils.ErrDeleteBucket).
			WithCause(err).
			WithExplanation("Failed to delete S3 bucket").
			WithContext("bucket", bucket).
			WithHint("Check IAM permissions for s3:DeleteBucket and ensure bucket is empty").
			Err()
	}
	return nil
}
