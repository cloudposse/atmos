package layout

import (
	"fmt"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Admission is the decision a repository takes for one publication after reading the current
// reference under the writer guard.
type Admission string

// Admission decisions.
const (
	// AdmitCreate writes a new reference.
	AdmitCreate Admission = "create"

	// AdmitReplace replaces a completed reference under force.
	AdmitReplace Admission = "replace"

	// AdmitUnchanged leaves the completed reference untouched: the same intent already exists.
	AdmitUnchanged Admission = "unchanged"

	// AdmitResumeCreate resumes a pending first publication of the same intent.
	AdmitResumeCreate Admission = "resume-create"

	// AdmitResumeReplace resumes a pending replacement of the same intent.
	AdmitResumeReplace Admission = "resume-replace"
)

// Outcome maps an admission to the operation outcome it produces.
func (a Admission) Outcome() artifact.Outcome {
	defer perf.Track(nil, "layout.Admission.Outcome")()

	switch a {
	case AdmitReplace, AdmitResumeReplace:
		return artifact.OutcomeReplaced
	case AdmitUnchanged:
		return artifact.OutcomeUnchanged
	default:
		return artifact.OutcomeCreated
	}
}

// Resuming reports whether the admission continues an interrupted publication.
func (a Admission) Resuming() bool {
	defer perf.Track(nil, "layout.Admission.Resuming")()

	return a == AdmitResumeCreate || a == AdmitResumeReplace
}

// AdmitInput is everything the admission rules need.
type AdmitInput struct {
	// Current is the reference as stored, or nil when it does not exist.
	Current *Reference

	// CurrentIntent is the intent digest of the completed Record. It is only read when Current is
	// completed; empty means it could not be computed, which never counts as unchanged.
	CurrentIntent digest.Digest

	// IncomingRecord and IncomingIntent identify the artifact being published.
	IncomingRecord digest.Digest
	IncomingIntent digest.Digest

	// ExpectedPrior is the reference the service observed; nil means the reference must not exist.
	ExpectedPrior *artifact.PinnedReference

	// Force permits replacing a conflicting completed reference.
	Force bool
}

// Admit applies the compare-and-swap, intent, force, and recovery rules of the publication
// lifecycle:
//
//   - A stale expected-prior reference is a conflict, never a silent overwrite.
//   - The same record or intent as the completed reference is unchanged.
//   - A different completed intent conflicts unless force is set.
//   - A pending reference of the same intent resumes; a pending reference of a different intent
//     is unavailable and names the owning operation, even under force. A pending operation is
//     never stolen automatically.
func Admit(in *AdmitInput) (Admission, error) {
	defer perf.Track(nil, "layout.Admit")()

	if in.Current == nil {
		if in.ExpectedPrior != nil {
			return "", fmt.Errorf("%w: the reference no longer exists but the writer expected %s", errUtils.ErrArtifactConflict, in.ExpectedPrior.ResolvedDigest)
		}
		return AdmitCreate, nil
	}
	if in.Current.State == StatePending {
		return admitPending(in)
	}
	if err := checkExpectedPrior(in.Current, in.ExpectedPrior); err != nil {
		return "", err
	}
	if in.Current.RecordDigest == in.IncomingRecord || (in.CurrentIntent != "" && in.CurrentIntent == in.IncomingIntent) {
		return AdmitUnchanged, nil
	}
	if !in.Force {
		return "", fmt.Errorf("%w: %s:%s already exists with a different intent; use force to replace it", errUtils.ErrArtifactConflict, in.Current.Name, in.Current.Version)
	}
	return AdmitReplace, nil
}

func admitPending(in *AdmitInput) (Admission, error) {
	if in.Current.IntentDigest != in.IncomingIntent {
		return "", fmt.Errorf("%w: %s:%s is pending for operation %s with a different intent; recover or repair it with the same intent before publishing",
			errUtils.ErrArtifactPublicationUnavailable, in.Current.Name, in.Current.Version, in.Current.OperationID)
	}
	if in.Current.PriorRecordDigest != "" {
		return AdmitResumeReplace, nil
	}
	return AdmitResumeCreate, nil
}

func checkExpectedPrior(current *Reference, expected *artifact.PinnedReference) error {
	switch {
	case expected == nil:
		return fmt.Errorf("%w: %s:%s now exists (%s) but the writer expected it to be absent", errUtils.ErrArtifactConflict, current.Name, current.Version, current.RecordDigest)
	case expected.ResolvedDigest != current.RecordDigest:
		return fmt.Errorf("%w: %s:%s is %s but the writer expected %s", errUtils.ErrArtifactConflict, current.Name, current.Version, current.RecordDigest, expected.ResolvedDigest)
	}
	return nil
}
