//go:build mage

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// s3DeployClient is the subset of the AWS SDK v2 S3 client used by this target.
type s3DeployClient interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

var loadS3DeployClient = func(ctx context.Context) (s3DeployClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: load AWS configuration: %w", errS3DeployAWSOperation, err)
	}
	return s3.NewFromConfig(cfg), nil
}

func (d *s3Deployer) loadManifest(ctx context.Context, location s3DeployLocation) (*s3DeployManifest, error) {
	output, err := d.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(location.Bucket),
		Key:    aws.String(location.objectKey(s3DeployManifestName)),
	})
	if err != nil {
		if isS3ManifestNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: read manifest: %w", errS3DeployAWSOperation, err)
	}
	if output == nil || output.Body == nil {
		return nil, errS3DeployInvalidManifest
	}
	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("mage: read S3 deploy manifest: %w", err)
	}
	var manifest s3DeployManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: %w", errS3DeployInvalidManifest, err)
	}
	if manifest.Version != s3DeployManifestVersion {
		return nil, fmt.Errorf("%w: %d", errS3DeployUnsupportedState, manifest.Version)
	}
	if manifest.Files == nil {
		return nil, errS3DeployInvalidManifest
	}
	return &manifest, nil
}

func isS3ManifestNotFound(err error) bool {
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == s3ManifestNotFoundErrorCode
}

func (d *s3Deployer) bootstrap(ctx context.Context, state *s3DeployState) error {
	fmt.Println("No remote manifest found; performing the one-time full metadata upload.")
	files := make([]string, 0, len(state.manifest.Files))
	for path := range state.manifest.Files {
		files = append(files, path)
	}
	sort.Strings(files)
	if err := d.uploadChanged(ctx, state.localDir, state.location, files, state.manifest); err != nil {
		return err
	}

	stale, err := d.listStaleObjects(ctx, state.location, &state.manifest, state.protected)
	if err != nil {
		return err
	}
	return d.deleteRemoved(ctx, state.location, stale)
}

func (d *s3Deployer) uploadChanged(
	ctx context.Context,
	localDir string,
	location s3DeployLocation,
	changed []string,
	manifest s3DeployManifest,
) error {
	for _, relative := range changed {
		metadata, ok := manifest.Files[relative]
		if !ok {
			return fmt.Errorf(s3ErrorWithValueFormat, errS3DeployMissingMetadata, relative)
		}
		path := filepath.Join(localDir, filepath.FromSlash(relative))
		file, err := os.Open(path) // #nosec G304 -- path is constrained to the deployment directory.
		if err != nil {
			return fmt.Errorf("mage: open S3 deploy source: %w", err)
		}
		_, putErr := d.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(location.Bucket),
			Key:           aws.String(location.objectKey(relative)),
			Body:          file,
			ContentLength: aws.Int64(metadata.Size),
			ContentType:   aws.String(metadata.ContentType),
		})
		closeErr := file.Close()
		if putErr != nil {
			return fmt.Errorf("%w: upload %s: %w", errS3DeployAWSOperation, relative, putErr)
		}
		if closeErr != nil {
			return fmt.Errorf("mage: close S3 deploy source: %w", closeErr)
		}
	}
	return nil
}

func (d *s3Deployer) listStaleObjects(
	ctx context.Context,
	location s3DeployLocation,
	manifest *s3DeployManifest,
	protected []s3ProtectedPattern,
) ([]string, error) {
	stale := make([]string, 0)
	var continuationToken *string
	for {
		output, err := d.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(location.Bucket),
			Prefix:            aws.String(location.listPrefix()),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: list bootstrap objects: %w", errS3DeployAWSOperation, err)
		}
		if output == nil {
			return nil, fmt.Errorf("%w: list bootstrap objects returned no response", errS3DeployAWSOperation)
		}
		stale = append(stale, collectStaleS3Objects(location, output.Contents, manifest, protected)...)
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if output.NextContinuationToken == nil {
			return nil, fmt.Errorf("%w: truncated object listing omitted its continuation token", errS3DeployAWSOperation)
		}
		continuationToken = output.NextContinuationToken
	}
	sort.Strings(stale)
	return stale, nil
}

func collectStaleS3Objects(
	location s3DeployLocation,
	objects []s3types.Object,
	manifest *s3DeployManifest,
	protected []s3ProtectedPattern,
) []string {
	stale := make([]string, 0)
	for _, object := range objects {
		relative, ok := location.relativeKey(aws.ToString(object.Key))
		if !ok || relative == "" || relative == s3DeployManifestName {
			continue
		}
		if _, managed := manifest.Files[relative]; managed {
			continue
		}
		if matchesS3ProtectedPath(relative, protected) {
			manifest.Files[relative] = s3DeployFile{
				Size:      aws.ToInt64(object.Size),
				Protected: true,
			}
			continue
		}
		stale = append(stale, relative)
	}
	return stale
}

func (d *s3Deployer) deleteRemoved(ctx context.Context, location s3DeployLocation, deleted []string) error {
	for offset := 0; offset < len(deleted); offset += s3DeleteBatchSize {
		end := min(offset+s3DeleteBatchSize, len(deleted))
		objects := make([]s3types.ObjectIdentifier, 0, end-offset)
		for _, relative := range deleted[offset:end] {
			objects = append(objects, s3types.ObjectIdentifier{Key: aws.String(location.objectKey(relative))})
		}
		output, err := d.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(location.Bucket),
			Delete: &s3types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("%w: delete objects: %w", errS3DeployAWSOperation, err)
		}
		if output != nil && len(output.Errors) > 0 {
			failure := output.Errors[0]
			return fmt.Errorf(
				"%w: key=%s code=%s message=%s",
				errS3DeployPartialDelete,
				aws.ToString(failure.Key),
				aws.ToString(failure.Code),
				aws.ToString(failure.Message),
			)
		}
	}
	return nil
}

func (d *s3Deployer) uploadManifest(ctx context.Context, location s3DeployLocation, manifest s3DeployManifest) error {
	data, err := marshalS3DeployManifest(manifest)
	if err != nil {
		return err
	}
	_, err = d.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(location.Bucket),
		Key:           aws.String(location.objectKey(s3DeployManifestName)),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(s3ManifestContentType),
	})
	if err != nil {
		return fmt.Errorf("%w: upload manifest: %w", errS3DeployAWSOperation, err)
	}
	return nil
}
