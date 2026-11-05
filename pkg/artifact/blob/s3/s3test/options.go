package s3test

import "github.com/cloudposse/atmos/pkg/perf"

// ConditionalBehavior selects how the server treats If-None-Match and If-Match.
type ConditionalBehavior int

// Conditional behaviors.
const (
	// ConditionalEnforce honors the conditions like S3.
	ConditionalEnforce ConditionalBehavior = iota

	// ConditionalIgnore silently ignores the headers, like a non-conforming emulator.
	ConditionalIgnore

	// ConditionalNotImplemented answers 501 NotImplemented to any conditional request.
	ConditionalNotImplemented
)

// defaultBucket is the bucket the server serves unless overridden.
const defaultBucket = "test-bucket"

// maxKeysCap is the page cap S3 applies to ListObjectsV2.
const maxKeysCap = 1000

// settings holds server configuration.
type settings struct {
	bucket      string
	discard     bool
	conditional ConditionalBehavior
	pageCap     int
	minPartSize int64
}

// Option configures a [Server].
type Option func(*settings)

// WithBucket sets the bucket name the server serves.
func WithBucket(name string) Option {
	defer perf.Track(nil, "s3test.WithBucket")()

	return func(s *settings) { s.bucket = name }
}

// WithDiscardBodies keeps only sizes and ETags, so streaming tests can push large bodies without
// the server holding them. Reading such an object fails.
func WithDiscardBodies() Option {
	defer perf.Track(nil, "s3test.WithDiscardBodies")()

	return func(s *settings) { s.discard = true }
}

// WithConditionalBehavior selects how conditional headers are treated.
func WithConditionalBehavior(behavior ConditionalBehavior) Option {
	defer perf.Track(nil, "s3test.WithConditionalBehavior")()

	return func(s *settings) { s.conditional = behavior }
}

// WithPageCap caps the entries per ListObjectsV2 page below the client's max-keys, to force
// pagination with few objects.
func WithPageCap(n int) Option {
	defer perf.Track(nil, "s3test.WithPageCap")()

	return func(s *settings) { s.pageCap = n }
}

// WithMinPartSize rejects completion when a non-final part is smaller than n bytes, as S3 does
// with 5 MiB.
func WithMinPartSize(n int64) Option {
	defer perf.Track(nil, "s3test.WithMinPartSize")()

	return func(s *settings) { s.minPartSize = n }
}
