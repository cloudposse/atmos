package step

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/s3upload"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// S3Handler uploads artifacts with the step or component's AWS credentials.
type S3Handler struct {
	BaseHandler
	newClient func(context.Context, string, *Variables) (s3upload.Client, error)
}

func init() {
	Register(&S3Handler{BaseHandler: NewBaseHandler("aws/s3", CategoryCommand, false), newClient: newS3UploadClient})
}

// Validate checks the supported action and required paths.
func (h *S3Handler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.S3Handler.Validate")()
	if step.Action != "" && step.Action != "upload" {
		return fmt.Errorf("%w: aws/s3 action must be upload", errUtils.ErrS3Upload)
	}
	source, ok := step.Source.(string)
	if !ok || strings.TrimSpace(source) == "" {
		return errUtils.ErrS3UploadSource
	}
	return h.ValidateRequired(step, "destination", step.Destination)
}

// Execute resolves paths and credentials without changing the process environment.
func (h *S3Handler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.S3Handler.Execute")()
	if err := h.Validate(step); err != nil {
		return nil, err
	}
	source, err := h.ResolveInWorkingDirectory(step, vars, step.Source.(string), "source")
	if err != nil {
		return nil, err
	}
	fields := []string{step.Destination, step.Region, step.ContentType, step.CacheControl}
	for i := range fields {
		fields[i], err = vars.Resolve(fields[i])
		if err != nil {
			return nil, err
		}
	}
	destination, region, contentType, cacheControl := fields[0], fields[1], fields[2], fields[3]
	bucket, key, err := s3upload.ParseDestination(destination)
	if err != nil {
		return nil, err
	}
	if step.DryRun {
		return NewStepResult(destination).WithSkipped(), nil
	}
	client, err := h.newClient(ctx, region, vars)
	if err != nil {
		return nil, err
	}
	result, err := s3upload.Upload(ctx, client, s3upload.Options{
		Source: source, Destination: destination, ContentType: contentType, CacheControl: cacheControl,
	})
	if err != nil {
		return nil, err
	}
	return s3StepResult(destination, bucket, key, result)
}

func s3StepResult(destination, bucket, key string, result s3upload.Result) (*StepResult, error) {
	value := destination
	if len(result.URIs) == 1 {
		value = result.URIs[0]
		_, resolvedKey, err := s3upload.ParseDestination(value)
		if err != nil {
			return nil, err
		}
		key = resolvedKey
	}
	return NewStepResult(value).WithValues(result.URIs).
		WithMetadata("bucket", bucket).WithMetadata("key", key).
		WithMetadata("uploaded", result.Uploaded).WithMetadata("unchanged", result.Unchanged), nil
}

func newS3UploadClient(ctx context.Context, region string, vars *Variables) (s3upload.Client, error) {
	return s3upload.NewClient(ctx, region, vars.AWSAuthContext, vars.Env)
}
