package s3

import (
	"fmt"
	"os"
	"time"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Defaults of the repository.
const (
	// DefaultConcurrency bounds the objects and multipart parts uploaded at once.
	DefaultConcurrency = blobs3.DefaultConcurrency

	// DefaultLockWait is how long Lock waits for a competing owner before it reports the claim.
	DefaultLockWait = 30 * time.Second

	// The interval between attempts to take a held claim.
	defaultLockPoll = 250 * time.Millisecond
)

// settings holds the construction options of an S3 repository.
type settings struct {
	store       blob.Store
	verifier    artifact.Verifier
	owner       string
	concurrency int
	partSize    int64
	lockWait    time.Duration
	lockPoll    time.Duration
	faults      faultHook
}

// Option configures an S3 repository or its registration.
type Option func(*settings)

// WithStore injects the object store. Without it the repository builds the AWS SDK store from its
// configuration. The injected store must be rooted at the configured prefix, because the repository
// hands it keys relative to that root. It does not need to apply the configured tags: the
// repository passes them on every write.
func WithStore(store blob.Store) Option {
	defer perf.Track(nil, "s3.WithStore")()

	return func(s *settings) { s.store = store }
}

// WithVerifier injects the cryptographic verifier used by Verify. The formats it understands are
// the evidence formats the repository advertises. Without a verifier the repository stores no
// evidence and Sign and Verify return unsupported-capability errors.
func WithVerifier(verifier artifact.Verifier) Option {
	defer perf.Track(nil, "s3.WithVerifier")()

	return func(s *settings) { s.verifier = verifier }
}

// WithOwner sets the owner recorded in lock claims and operation journals when the caller does not
// supply one. The default is "<hostname>:<pid>".
func WithOwner(owner string) Option {
	defer perf.Track(nil, "s3.WithOwner")()

	return func(s *settings) { s.owner = owner }
}

// WithConcurrency bounds the objects uploaded at once and, for a store built from configuration,
// the multipart parts per object. The default is [DefaultConcurrency].
func WithConcurrency(workers int) Option {
	defer perf.Track(nil, "s3.WithConcurrency")()

	return func(s *settings) {
		if workers > 0 {
			s.concurrency = workers
		}
	}
}

// WithPartSize sets the multipart part size of a store built from configuration. Real S3 requires
// at least 5 MiB for every part but the last; smaller sizes are for emulators.
func WithPartSize(size int64) Option {
	defer perf.Track(nil, "s3.WithPartSize")()

	return func(s *settings) {
		if size > 0 {
			s.partSize = size
		}
	}
}

// WithLockWait bounds how long Lock waits for a competing owner. Past it, Lock fails with a
// conflict that names the claim object. Claims never expire, so waiting longer only helps when the
// owner is alive.
func WithLockWait(wait time.Duration) Option {
	defer perf.Track(nil, "s3.WithLockWait")()

	return func(s *settings) {
		if wait > 0 {
			s.lockWait = wait
		}
	}
}

func applySettings(opts []Option) settings {
	applied := settings{concurrency: DefaultConcurrency, lockWait: DefaultLockWait, lockPoll: defaultLockPoll}
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	if applied.owner == "" {
		applied.owner = defaultOwner()
	}
	return applied
}

func defaultOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}
