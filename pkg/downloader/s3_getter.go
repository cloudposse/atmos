package downloader

import (
	"context"

	s3source "github.com/cloudposse/atmos/pkg/downloader/s3"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// WithAWSAuthResolver defers authentication until an S3 source needs downloading.
func WithAWSAuthResolver(ctx context.Context, resolver func(context.Context) (*schema.AWSAuthContext, error)) context.Context {
	defer perf.Track(nil, "downloader.WithAWSAuthResolver")()
	return s3source.WithAuthResolver(ctx, resolver)
}

// WithAWSAuthContext scopes source-download credentials without changing the environment.
func WithAWSAuthContext(ctx context.Context, auth *schema.AWSAuthContext) context.Context {
	defer perf.Track(nil, "downloader.WithAWSAuthContext")()
	return s3source.WithAuthContext(ctx, auth)
}
