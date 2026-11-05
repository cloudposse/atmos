package s3

import (
	"context"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

const storeName = "s3"

// Store is an S3 [blob.Store].
type Store struct {
	cfg settings

	mu  sync.Mutex
	api API

	probeMu    sync.Mutex
	probed     bool
	probeError error
}

// New returns a Store for bucket. No network or credential work happens until the first operation.
func New(bucket string, opts ...Option) (*Store, error) {
	defer perf.Track(nil, "blob.s3.New")()

	if bucket == "" {
		return nil, blob.NewError(blob.KindInvalid, blob.OpNew, "", fmt.Errorf("%w: bucket is required", errUtils.ErrBlobInvalid))
	}
	cfg := settings{bucket: bucket, partSize: DefaultPartSize, concurrency: DefaultConcurrency}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Store{cfg: cfg, api: cfg.api}, nil
}

// NewFromClient returns a Store over an existing client.
func NewFromClient(api API, bucket string, opts ...Option) (*Store, error) {
	defer perf.Track(nil, "blob.s3.NewFromClient")()

	return New(bucket, append(opts, WithAPI(api))...)
}

// Name implements [blob.Store].
func (s *Store) Name() string {
	defer perf.Track(nil, "s3.Store.Name")()

	return storeName
}

// Bucket returns the bucket name.
func (s *Store) Bucket() string {
	defer perf.Track(nil, "s3.Store.Bucket")()

	return s.cfg.bucket
}

// Capabilities implements [blob.Store].
func (s *Store) Capabilities() blob.Capabilities {
	defer perf.Track(nil, "s3.Store.Capabilities")()

	conditional := s.cfg.conditional != ConditionalDisabled
	return blob.Capabilities{
		ConditionalPut:    conditional,
		ConditionalDelete: conditional,
		Multipart:         true,
		Tags:              true,
		Metadata:          true,
		Range:             true,
		Listing:           true,
		ListDelimiter:     true,
	}
}

// client returns the S3 client, building it on first use. A failed build is not cached, so a
// transient identity failure can be retried by the next operation.
func (s *Store) client(ctx context.Context) (API, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api != nil {
		return s.api, nil
	}
	client, err := NewClient(ctx, &s.cfg.client)
	if err != nil {
		return nil, blob.NewError(blob.KindIdentity, "client", "", err)
	}
	s.api = client
	return s.api, nil
}

// validateKey applies the configured key rules.
func (s *Store) validateKey(key string) error {
	if s.cfg.opaqueKeys {
		return blob.ValidateOpaqueKey(key)
	}
	return blob.ValidateKey(key)
}

// objectKey maps a store key to the bucket key.
func (s *Store) objectKey(key string) string {
	if s.cfg.prefix == "" {
		return key
	}
	return s.cfg.prefix + "/" + key
}

// storeKey maps a bucket key back to a store key.
func (s *Store) storeKey(objectKey string) string {
	if s.cfg.prefix == "" {
		return objectKey
	}
	return strings.TrimPrefix(objectKey, s.cfg.prefix+"/")
}

// retryOptions disables SDK retries for conditional mutations. A retried conditional request whose
// first attempt already committed would otherwise answer 412 and look like a lost race.
func retryOptions(conditional bool) []func(*sdkOptions) {
	if !conditional {
		return nil
	}
	return []func(*sdkOptions){noRetry}
}

var _ blob.Store = (*Store)(nil)
