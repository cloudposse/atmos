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
// of the next one. Runtime failures do not switch stores: within a single run the
// store selected here handles both upload and download, so a failure mid-transfer
// cannot move the planfile.
//
// That guarantee is per process, not across CI steps. `plan` and `deploy` resolve
// independently, and selection depends on the environment at construction, so an
// environment that differs between the two steps (a token present at plan and absent
// at deploy) can land them on different stores. Skipping a store the user configured
// is therefore logged at warning level — see logSkippedCandidate.
//
// When every candidate fails, the errors are joined so the message explains each
// attempt rather than only the last.
func NewStoreFromCandidates(candidates []planfile.StoreCandidate, decorate StoreDecorator) (planfile.Store, planfile.StoreCandidate, error) {
	defer perf.Track(nil, "adapter.NewStoreFromCandidates")()

	if len(candidates) == 0 {
		return nil, planfile.StoreCandidate{}, fmt.Errorf("%w: no planfile store candidates were resolved", errUtils.ErrPlanfileStoreUnavailable)
	}

	var failures []error
	skippedConfigured := false

	for _, candidate := range candidates {
		opts := candidate.Options
		if decorate != nil {
			decorate(&opts)
		}

		backend, err := newBackend(opts)
		if err != nil {
			if candidate.IsConfigured() {
				skippedConfigured = true
			}
			logSkippedCandidate(&candidate, err)
			failures = append(failures, fmt.Errorf("%s (from %s): %w", candidate.Description(), candidate.Source, err))
			continue
		}

		logSelectedCandidate(&candidate, skippedConfigured)

		candidate.Options = opts
		return NewStore(backend), candidate, nil
	}

	return nil, planfile.StoreCandidate{}, fmt.Errorf("%w: %w", errUtils.ErrPlanfileStoreUnavailable, errors.Join(failures...))
}

// logSkippedCandidate reports a candidate that could not be initialized.
//
// A store the user configured is warned about: the run continues with a different backend, so
// the planfile silently lands somewhere other than where it was asked to go — the same class of
// failure as the environment-detection override this package exists to prevent, and just as hard
// to spot from a successful-looking `plan`. Environment-detected and built-in fallback
// candidates stay at debug: nobody asked for them, and skipping them is the normal path on any
// machine without CI credentials.
func logSkippedCandidate(candidate *planfile.StoreCandidate, err error) {
	defer perf.Track(nil, "adapter.logSkippedCandidate")()

	fields := []any{
		"store", candidate.Description(),
		"source", string(candidate.Source),
		"error", err,
	}

	if candidate.IsConfigured() {
		log.Warn("Configured planfile store is unavailable; trying the next candidate", fields...)
		return
	}

	log.Debug("Planfile store unavailable, trying the next candidate", fields...)
}

// logSelectedCandidate reports the store that will be used. It is raised to warning level only
// when a store the user configured was skipped to get here, so the two messages together say
// what was asked for and what is actually being used. With nothing skipped — the ordinary case —
// this stays at debug and the command prints nothing.
func logSelectedCandidate(candidate *planfile.StoreCandidate, skippedConfigured bool) {
	defer perf.Track(nil, "adapter.logSelectedCandidate")()

	fields := []any{
		"store", candidate.Description(),
		"source", string(candidate.Source),
	}

	if skippedConfigured {
		log.Warn("Using planfile store", fields...)
		return
	}

	log.Debug("Selected planfile store", fields...)
}
