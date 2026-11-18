package s3

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// Transport operations, used to choose the generic sentinel of an unclassified failure.
const (
	opRead   = "read"
	opWrite  = "write"
	opDelete = "delete"
)

// mapErr translates a blob failure into the artifact vocabulary while keeping the original error in
// the chain, so callers can still match both with errors.Is. Context errors pass through untouched.
func (r *Repository) mapErr(op, what string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case blob.IsConditionalUnsupported(err):
		unsupported := artifact.NewUnsupportedCapability(r.alias, op, "conditional-writes",
			fmt.Sprintf("the S3 endpoint cannot enforce the conditional write needed for %s; use an endpoint that honors If-None-Match and If-Match, or AWS S3", what))
		unsupported.Cause = err
		return unsupported
	case blob.IsPreconditionFailed(err):
		return fmt.Errorf("%w: %s changed concurrently: %w", errUtils.ErrArtifactConflict, what, err)
	case blob.IsAmbiguous(err):
		return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactPublicationAmbiguous, what, err)
	case errors.Is(err, errUtils.ErrBlobIdentityUnresolved):
		return fmt.Errorf("repository %q cannot authenticate to S3 for %s: %w", r.alias, what, err)
	default:
		return fmt.Errorf("%w: %s: %w", genericSentinel(op), what, err)
	}
}

func genericSentinel(op string) error {
	switch op {
	case opRead:
		return errUtils.ErrArtifactDownloadFailed
	case opDelete:
		return errUtils.ErrArtifactDeleteFailed
	default:
		return errUtils.ErrArtifactUploadFailed
	}
}

// isArtifactError reports whether err already carries an artifact sentinel the caller matches on,
// so a generic wrapper does not hide it.
func isArtifactError(err error) bool {
	for _, sentinel := range []error{
		errUtils.ErrArtifactConflict, errUtils.ErrArtifactUnsupportedCapability, errUtils.ErrArtifactIntegrityFailed,
		errUtils.ErrArtifactPackagingInvalid, errUtils.ErrArtifactUnsafePath, errUtils.ErrArtifactTooLarge,
		errUtils.ErrArtifactPublicationUnavailable, errUtils.ErrArtifactRecordInvalid, errUtils.ErrArtifactPublicationAmbiguous,
		errUtils.ErrArtifactNotFound, errUtils.ErrBlobIdentityUnresolved,
		context.Canceled, context.DeadlineExceeded,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return isCrash(err)
}

// wrapWrite classifies a publication failure. Context errors and already-typed artifact errors pass
// through so callers keep matching on them.
func wrapWrite(step string, err error) error {
	if isArtifactError(err) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactUploadFailed, step, err)
}
