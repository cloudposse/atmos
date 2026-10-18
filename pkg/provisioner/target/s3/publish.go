// Package s3 publishes files to AWS S3 without deleting remote objects.
package s3

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/s3upload"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	bucketField                = "bucket"
	prefixField                = "prefix"
	maxPublishObjectSize int64 = 5 * 1024 * 1024 * 1024
)

type publisher struct{}

func init() { target.Register("aws/s3", &publisher{}) }

// Deliver leaves CloudFormation's specialized template packaging with its producer.
func (p *publisher) Deliver(context.Context, *target.DeliverInput) error {
	defer perf.Track(nil, "s3.publisher.Deliver")()
	return fmt.Errorf("%w: aws/s3 accepts file publishing", errUtils.ErrPublishTarget)
}

// ValidatePublish checks configuration and object sizes without contacting AWS.
func (p *publisher) ValidatePublish(in *target.PublishInput) error {
	defer perf.Track(in.AtmosConfig, "target.s3.ValidatePublish")()
	if err := validatePublishSettings(in.TargetConfig); err != nil {
		return err
	}
	bucket := field(in, bucketField)
	if bucket == "" || strings.ContainsAny(bucket, "/\\:@?# ") || field(in, "region") == "" {
		return fmt.Errorf("%w: aws/s3 requires bucket and region", errUtils.ErrPublishTarget)
	}
	if prefix := field(in, prefixField); prefix != "" {
		if err := target.ValidatePublishPath(prefix); err != nil {
			return err
		}
	}
	return validatePublishFiles(in.Files)
}

func validatePublishFiles(files []target.PublishFile) error {
	for _, file := range files {
		if file.Size < 0 || file.Size > maxPublishObjectSize {
			return fmt.Errorf("%w: %s must have a size between zero and 5 GiB", errUtils.ErrPublishSource, file.Name)
		}
		if err := target.ValidatePublishPath(file.Name); err != nil {
			return err
		}
	}
	return nil
}

func validatePublishSettings(settings map[string]any) error {
	for key, value := range settings {
		switch key {
		case "kind", bucketField, prefixField, "region", "content_type", "cache_control":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%w: %s must be a string", errUtils.ErrPublishTarget, key)
			}
		case "auth":
		default:
			return fmt.Errorf("%w: unsupported aws/s3 setting %q", errUtils.ErrPublishTarget, key)
		}
	}
	return nil
}

// Publish uploads changed files using destination-specific credentials.
func (p *publisher) Publish(ctx context.Context, in *target.PublishInput) (*target.PublishResult, error) {
	defer perf.Track(in.AtmosConfig, "target.s3.Publish")()
	if err := p.ValidatePublish(in); err != nil {
		return nil, err
	}
	var awsAuth *schema.AWSAuthContext
	if in.AuthContext != nil {
		awsAuth = in.AuthContext.AWS
	}
	client, err := s3upload.NewClient(ctx, field(in, "region"), awsAuth, in.Env)
	if err != nil {
		return nil, err
	}
	result := &target.PublishResult{Metadata: map[string]any{bucketField: field(in, bucketField), "key": field(in, prefixField)}}
	for _, file := range in.Files {
		changed, uri, err := publishFile(ctx, client, in, file)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrPublishFailed, file.Name, err)
		}
		result.Locations = append(result.Locations, uri)
		if changed {
			result.Changed++
		} else {
			result.Unchanged++
		}
	}
	result.Metadata["uploaded"] = result.Changed
	if len(in.Files) == 1 {
		result.Metadata["key"] = filepath.ToSlash(filepath.Join(field(in, prefixField), in.Files[0].Name))
	}
	return result, nil
}

func publishFile(ctx context.Context, client s3upload.Client, in *target.PublishInput, file target.PublishFile) (bool, string, error) {
	reader, err := file.Open()
	if err != nil {
		return false, "", err
	}
	defer reader.Close()
	uri := (&url.URL{Scheme: "s3", Host: field(in, bucketField), Path: "/" + filepath.ToSlash(filepath.Join(field(in, prefixField), file.Name))}).String()
	changed, err := s3upload.UploadReader(ctx, client, reader, file.Size, s3upload.Options{
		Source: file.Name, Destination: uri, ContentType: field(in, "content_type"), CacheControl: field(in, "cache_control"),
	})
	return changed, uri, err
}

func field(in *target.PublishInput, key string) string {
	value, _ := in.TargetConfig[key].(string)
	return value
}
