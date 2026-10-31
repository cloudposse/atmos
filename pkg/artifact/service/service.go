package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Operation names used in capability errors and lock requests.
const (
	opPublish = "publish"
	opPull    = "pull"
	opCopy    = "copy"
	opDelete  = "delete"
	opSign    = "sign"
	opVerify  = "verify"
	opInspect = "inspect"
	opShow    = "show"
	opPin     = "pin"
	opList    = "list"
)

// requireRepository rejects an empty repository alias: a missing required repository is an error.
func requireRepository(alias string) error {
	if alias == "" {
		return fmt.Errorf("%w: a repository is required", errUtils.ErrArtifactConfigInvalid)
	}
	return nil
}

// Defaults for retry behavior. Retries apply only to publications that failed with a temporary
// error and whose reconciliation proves nothing was committed.
const (
	// DefaultAttempts is the default number of publication attempts.
	DefaultAttempts = 3

	// DefaultBackoff is the default base delay between attempts; it doubles each attempt.
	DefaultBackoff = 200 * time.Millisecond
)

// Repacker rebuilds an existing Record for a different packaging. [produce.Producer] implements it.
type Repacker interface {
	// Repack stages an existing Record from a materialized tree.
	Repack(ctx context.Context, request *produce.RepackRequest) (*artifact.PreparedArtifact, error)
}

// Service implements [artifact.Service] over repositories.
type Service struct {
	repos       Repositories
	producers   *artifact.ProducerRegistry
	repacker    Repacker
	signer      artifact.Signer
	journal     Journal
	immutable   ImmutabilityPolicy
	clock       func() time.Time
	newID       func() string
	sleep       func(ctx context.Context, d time.Duration) error
	owner       string
	stagingRoot string
	concurrency int
	attempts    int
	backoff     time.Duration
}

var _ artifact.Service = (*Service)(nil)

// Option customizes a [Service].
type Option func(*Service)

// WithClock injects the clock. The service captures the time once per operation, in UTC.
func WithClock(clock func() time.Time) Option {
	defer perf.Track(nil, "service.WithClock")()

	return func(s *Service) { s.clock = clock }
}

// WithIDGenerator injects the operation ID generator.
func WithIDGenerator(newID func() string) Option {
	defer perf.Track(nil, "service.WithIDGenerator")()

	return func(s *Service) { s.newID = newID }
}

// WithConcurrency sets the transfer and discovery concurrency. The default is
// [artifact.DefaultConcurrency].
func WithConcurrency(n int) Option {
	defer perf.Track(nil, "service.WithConcurrency")()

	return func(s *Service) { s.concurrency = n }
}

// WithRetry sets the number of publication attempts and the base backoff.
func WithRetry(attempts int, backoff time.Duration) Option {
	defer perf.Track(nil, "service.WithRetry")()

	return func(s *Service) {
		s.attempts = attempts
		s.backoff = backoff
	}
}

// WithSleep injects the backoff sleep; tests use a no-op.
func WithSleep(sleep func(ctx context.Context, d time.Duration) error) Option {
	defer perf.Track(nil, "service.WithSleep")()

	return func(s *Service) { s.sleep = sleep }
}

// WithJournal registers a lifecycle observer.
func WithJournal(journal Journal) Option {
	defer perf.Track(nil, "service.WithJournal")()

	return func(s *Service) { s.journal = journal }
}

// WithImmutabilityPolicy registers the immutable-format admission seam.
func WithImmutabilityPolicy(policy ImmutabilityPolicy) Option {
	defer perf.Track(nil, "service.WithImmutabilityPolicy")()

	return func(s *Service) { s.immutable = policy }
}

// WithSigner supplies the signer used by Sign.
func WithSigner(signer artifact.Signer) Option {
	defer perf.Track(nil, "service.WithSigner")()

	return func(s *Service) { s.signer = signer }
}

// WithOwner sets the lock owner recorded in claims, for example host and process.
func WithOwner(owner string) Option {
	defer perf.Track(nil, "service.WithOwner")()

	return func(s *Service) { s.owner = owner }
}

// WithStagingRoot sets the directory for bounded disk staging (preparation and copy trees).
func WithStagingRoot(dir string) Option {
	defer perf.Track(nil, "service.WithStagingRoot")()

	return func(s *Service) { s.stagingRoot = dir }
}

// WithRepacker overrides the repacker used by Copy's staging fallback.
func WithRepacker(repacker Repacker) Option {
	defer perf.Track(nil, "service.WithRepacker")()

	return func(s *Service) { s.repacker = repacker }
}

// New returns a Service. With a nil producer registry, only the generic files producer is available.
func New(repos Repositories, producers *artifact.ProducerRegistry, opts ...Option) (*Service, error) {
	defer perf.Track(nil, "service.New")()

	s := &Service{
		repos:       repos,
		producers:   producers,
		clock:       time.Now,
		newID:       uuid.NewString,
		sleep:       sleepContext,
		concurrency: artifact.DefaultConcurrency,
		attempts:    DefaultAttempts,
		backoff:     DefaultBackoff,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.concurrency < 1 {
		s.concurrency = artifact.DefaultConcurrency
	}
	if s.attempts < 1 {
		s.attempts = 1
	}
	if s.owner == "" {
		s.owner = defaultOwner()
	}
	if s.producers == nil {
		s.producers = artifact.NewProducerRegistry()
		if err := produce.Register(s.producers, produce.WithStagingRoot(s.stagingRoot)); err != nil {
			return nil, err
		}
	}
	if s.repacker == nil {
		s.repacker = produce.New(produce.WithStagingRoot(s.stagingRoot))
	}
	return s, nil
}

func defaultOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// operation carries the identity of one logical operation: the ID and the UTC preparation time
// are captured exactly once, before the first attempt, and reused by every retry.
type operation struct {
	id         string
	preparedAt time.Time
	started    time.Time
	clock      func() time.Time
	result     artifact.OperationResult
}

func (s *Service) begin(action artifact.Action, repository, requested string) *operation {
	now := s.clock().UTC()
	return &operation{
		id:         s.newID(),
		preparedAt: now,
		started:    now,
		clock:      s.clock,
		result: artifact.OperationResult{
			OperationID: "",
			Action:      action,
			Repository:  repository,
			Requested:   requested,
		},
	}
}

// done finalizes the typed result: the operation ID, the duration, and an error diagnostic on failure.
func (o *operation) done(err error) artifact.OperationResult {
	o.result.OperationID = o.id
	o.result.Duration = o.clock().Sub(o.started)
	if err != nil {
		o.result.Diagnostics = append(o.result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "operation-failed", Message: err.Error()})
	}
	return o.result
}

func (o *operation) warn(code, message string) {
	o.result.Diagnostics = append(o.result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: code, Message: message})
}

func (o *operation) info(code, message string) {
	o.result.Diagnostics = append(o.result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityInfo, Code: code, Message: message})
}

// record forwards a lifecycle event to the journal, if any.
func (s *Service) record(ctx context.Context, entry *JournalEntry) {
	if s.journal != nil {
		s.journal.Record(ctx, entry)
	}
}
