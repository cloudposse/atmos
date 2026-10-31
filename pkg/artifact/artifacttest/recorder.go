package artifacttest

import (
	"context"
	"sync"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Recorder wraps an [artifact.Service] and records every publication it sees: the request, with the
// operation identity the caller supplied, and the typed result. It can also fail the first
// publications, which lets a test drive a wrapper-level retry through a real service.
//
// Entry points that run artifact operations (the CLI, workflow and custom-command steps, hooks)
// share one service seam, so a test injects a Recorder around a real composition and compares what
// every entry point asked for and received.
type Recorder struct {
	artifact.Service

	mu       sync.Mutex
	requests []artifact.PublishRequest
	results  []artifact.OperationResult
	failures int
	failure  error
}

var _ artifact.Service = (*Recorder)(nil)

// NewRecorder wraps the service.
func NewRecorder(inner artifact.Service) *Recorder {
	defer perf.Track(nil, "artifacttest.NewRecorder")()

	return &Recorder{Service: inner}
}

// FailPublications makes the next count publications fail with err before they reach the wrapped
// service. A failed attempt returns a partial result that carries the supplied operation ID.
func (r *Recorder) FailPublications(count int, err error) {
	defer perf.Track(nil, "artifacttest.Recorder.FailPublications")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures, r.failure = count, err
}

// Publish implements [artifact.Service].
//
//nolint:gocritic // The request is passed by value because artifact.Service fixes the signature.
func (r *Recorder) Publish(ctx context.Context, request artifact.PublishRequest) (*artifact.OperationResult, error) {
	defer perf.Track(nil, "artifacttest.Recorder.Publish")()

	outcome, err := r.PublishDetailed(ctx, request)
	return &outcome.Operation, err
}

// PublishDetailed returns the sealed Record next to the operation result, like the real service,
// which is what a dry run and the structured result need. The wrapped service must provide it; a
// service that does not still works, with an empty Record.
//
//nolint:gocritic // The request is passed by value to mirror Publish.
func (r *Recorder) PublishDetailed(ctx context.Context, request artifact.PublishRequest) (*service.PublishOutcome, error) {
	defer perf.Track(nil, "artifacttest.Recorder.PublishDetailed")()

	r.mu.Lock()
	r.requests = append(r.requests, request)
	if r.failures > 0 {
		r.failures--
		failure := r.failure
		partial := artifact.OperationResult{OperationID: request.Execution.OperationID, Action: artifact.ActionPublish, Repository: request.Repository}
		r.results = append(r.results, partial)
		r.mu.Unlock()
		return &service.PublishOutcome{Operation: partial}, failure
	}
	r.mu.Unlock()

	var (
		outcome *service.PublishOutcome
		err     error
	)
	if detailed, ok := r.Service.(interface {
		PublishDetailed(context.Context, artifact.PublishRequest) (*service.PublishOutcome, error)
	}); ok {
		outcome, err = detailed.PublishDetailed(ctx, request)
	} else {
		var result *artifact.OperationResult
		result, err = r.Service.Publish(ctx, request)
		outcome = &service.PublishOutcome{}
		if result != nil {
			outcome.Operation = *result
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, outcome.Operation)
	return outcome, err
}

// Requests returns the publish requests in the order they arrived.
func (r *Recorder) Requests() []artifact.PublishRequest {
	defer perf.Track(nil, "artifacttest.Recorder.Requests")()

	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]artifact.PublishRequest(nil), r.requests...)
}

// Results returns the publish results in the order they were produced.
func (r *Recorder) Results() []artifact.OperationResult {
	defer perf.Track(nil, "artifacttest.Recorder.Results")()

	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]artifact.OperationResult(nil), r.results...)
}
