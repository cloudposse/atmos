package adapter

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StoreDecorator mutates store options before the backend is constructed. It lets
// callers attach caller-specific data (e.g. the active auth identity) without the
// resolver needing to know about it.
type StoreDecorator func(*planfile.StoreOptions)

// newBackend is the backend constructor, indirected so tests can substitute a
// factory without registering real cloud backends.
var newBackend = artifact.NewStore

// NewStoreFromCandidates constructs the first candidate that initializes
// successfully and returns it together with the candidate it came from.
//
// Candidates are tried in order, which is what makes `planfiles.priority`
// meaningful: a backend that cannot be initialized in the current environment
// (GitHub Artifacts without a token, S3 without credentials) is skipped in favor
// of the next one. Runtime failures do not switch stores — once a store is
// selected, it handles both upload and download so that `plan` and `deploy` agree
// on where the planfile lives.
//
// When every candidate fails, the errors are joined so the message explains each
// attempt rather than only the last.
func NewStoreFromCandidates(candidates []planfile.StoreCandidate, decorate StoreDecorator) (planfile.Store, planfile.StoreCandidate, error) {
	defer perf.Track(nil, "adapter.NewStoreFromCandidates")()

	if len(candidates) == 0 {
		return nil, planfile.StoreCandidate{}, fmt.Errorf("%w: no planfile store candidates were resolved", errUtils.ErrPlanfileStoreUnavailable)
	}

	var failures []error
	for _, candidate := range candidates {
		opts := candidate.Options
		if decorate != nil {
			decorate(&opts)
		}

		backend, err := newBackend(opts)
		if err != nil {
			log.Debug("Planfile store unavailable, trying the next candidate",
				"store", candidate.Description(), "source", string(candidate.Source), "error", err)
			failures = append(failures, fmt.Errorf("%s (from %s): %w", candidate.Description(), candidate.Source, err))
			continue
		}

		log.Debug("Selected planfile store", "store", candidate.Description(), "source", string(candidate.Source))

		candidate.Options = opts
		return NewStore(backend), candidate, nil
	}

	return nil, planfile.StoreCandidate{}, fmt.Errorf("%w: %w", errUtils.ErrPlanfileStoreUnavailable, errors.Join(failures...))
}
