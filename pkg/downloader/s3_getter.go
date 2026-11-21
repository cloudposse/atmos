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

// WithAWSSession scopes a component's assumed AWS session to its source downloads. The session takes
// precedence over an Atmos auth context, because the role is assumed on top of it.
func WithAWSSession(ctx context.Context, session map[string]string) context.Context {
	defer perf.Track(nil, "downloader.WithAWSSession")()
	return s3source.WithSession(ctx, session)
}
