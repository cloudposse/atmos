package backend

import "github.com/cloudposse/atmos/pkg/perf"

// createOptions holds optional settings for a backend create operation.
type createOptions struct {
	// bucketNamespace is forwarded as-is to the S3 CreateBucket call when non-empty.
	bucketNamespace string
}

// CreateOption configures optional behavior for a BackendCreateFunc.
// Backend types ignore options that do not apply to them.
type CreateOption func(*createOptions)

// WithBucketNamespace sets the S3 bucket namespace passed through to CreateBucket.
// The S3 provisioner checks the value against the namespaces the AWS SDK defines;
// an empty value leaves the field unset.
func WithBucketNamespace(namespace string) CreateOption {
	defer perf.Track(nil, "backend.WithBucketNamespace")()

	return func(o *createOptions) {
		o.bucketNamespace = namespace
	}
}

// BucketNamespaceForTesting returns the bucket namespace carried by a list of create options.
// Tests outside this package use it to assert which options a create function received.
func BucketNamespaceForTesting(opts ...CreateOption) string {
	defer perf.Track(nil, "backend.BucketNamespaceForTesting")()

	return applyCreateOptions(opts).bucketNamespace
}

// applyCreateOptions resolves a list of options into a createOptions value.
func applyCreateOptions(opts []CreateOption) createOptions {
	var o createOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
