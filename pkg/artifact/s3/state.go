package s3

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// reconcileTimeout bounds the reads that settle an ambiguous write after the caller's context ended.
const reconcileTimeout = 30 * time.Second

// refState is a stored reference together with the ETag that guards its next conditional write.
type refState struct {
	ref  *layout.Reference
	etag string
}

// detached returns a context that ignores the caller's cancellation, for reconciliation and cleanup
// that must complete even when the caller gave up.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), reconcileTimeout)
}

// loadReference returns the stored reference, or nil when it does not exist.
func (r *Repository) loadReference(ctx context.Context, key string) (*refState, error) {
	raw, etag, err := r.readObject(ctx, layout.ReferencePath(key), layout.MaxDocumentBytes)
	if err != nil || raw == nil {
		return nil, err
	}
	ref, err := layout.DecodeReference(raw)
	if err != nil {
		return nil, fmt.Errorf("reference %s: %w", key, err)
	}
	if ref.Key() != key {
		return nil, fmt.Errorf("%w: reference object %s describes %s:%s", errUtils.ErrArtifactIntegrityFailed, key, ref.Name, ref.Version)
	}
	return &refState{ref: ref, etag: etag}, nil
}

// loadRepresentation returns the stored representation, or nil when it does not exist.
func (r *Repository) loadRepresentation(ctx context.Context, key string) (*layout.Representation, error) {
	raw, _, err := r.readObject(ctx, layout.RepresentationPath(key), layout.MaxDocumentBytes)
	if err != nil || raw == nil {
		return nil, err
	}
	rep, err := layout.DecodeRepresentation(raw)
	if err != nil {
		return nil, fmt.Errorf("representation %s: %w", key, err)
	}
	if layout.ReferenceKey(rep.Name, rep.Version) != key {
		return nil, fmt.Errorf("%w: representation object %s describes %s:%s", errUtils.ErrArtifactIntegrityFailed, key, rep.Name, rep.Version)
	}
	return rep, nil
}

// loadOperation returns the stored journal, or nil when it does not exist.
func (r *Repository) loadOperation(ctx context.Context, operationID string) (*layout.Operation, error) {
	raw, _, err := r.readObject(ctx, layout.OperationPath(operationID), layout.MaxDocumentBytes)
	if err != nil || raw == nil {
		return nil, err
	}
	op, err := layout.DecodeOperation(raw)
	if err != nil {
		return nil, fmt.Errorf("operation %s: %w", operationID, err)
	}
	if op.OperationID != operationID {
		return nil, fmt.Errorf("%w: operation journal for %q describes %q", errUtils.ErrArtifactIntegrityFailed, operationID, op.OperationID)
	}
	return op, nil
}

// loadRecord reads and verifies the immutable Record with the given digest. A missing Record is
// not found; a Record whose bytes do not hash to its object name, or that is not canonical, fails
// integrity.
func (r *Repository) loadRecord(ctx context.Context, recordDigest digest.Digest) (*artifact.Record, []byte, error) {
	if err := artifact.ValidateDigest(recordDigest); err != nil {
		return nil, nil, err
	}
	raw, _, err := r.readObject(ctx, layout.RecordPath(recordDigest), artifact.MaxRecordSize)
	if err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, recordDigest)
	}
	if got := digest.FromBytes(raw); got != recordDigest {
		return nil, nil, fmt.Errorf("%w: record object %s holds bytes that hash to %s", errUtils.ErrArtifactIntegrityFailed, recordDigest, got)
	}
	record, err := artifact.DecodeRecord(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: record %s: %w", errUtils.ErrArtifactIntegrityFailed, recordDigest, err)
	}
	return record, raw, nil
}

// writeRecord stores the immutable canonical Record.
func (r *Repository) writeRecord(ctx context.Context, recordDigest digest.Digest, canonical []byte) error {
	return r.putImmutable(ctx, layout.RecordPath(recordDigest), canonical, contentTypeJSON, artifact.MaxRecordSize)
}

// writeRepresentation replaces the representation record.
func (r *Repository) writeRepresentation(ctx context.Context, rep *layout.Representation) error {
	raw, err := rep.Marshal()
	if err != nil {
		return err
	}
	return r.putBytes(ctx, layout.RepresentationPath(layout.ReferenceKey(rep.Name, rep.Version)), raw)
}

// writeOperation replaces the operation journal.
func (r *Repository) writeOperation(ctx context.Context, op *layout.Operation) error {
	raw, err := op.Marshal()
	if err != nil {
		return err
	}
	return r.putBytes(ctx, layout.OperationPath(op.OperationID), raw)
}

// writeReference replaces the reference record under a condition and returns the new ETag. The
// condition is If-None-Match for a first write and If-Match with the observed ETag afterwards.
// A response that was lost after the request was sent is reconciled by reading the reference back:
// an identical reference proves the write was applied, an untouched one lets the idempotent
// conditional write be repeated, and anything else is another writer's change and a conflict.
func (r *Repository) writeReference(ctx context.Context, ref *layout.Reference, cond blob.Condition) (string, error) {
	raw, err := ref.Marshal()
	if err != nil {
		return "", err
	}
	key := layout.ReferencePath(ref.Key())
	var lastErr error
	for range putAttempts {
		etag, putErr := r.putConditional(ctx, key, raw, contentTypeJSON, cond)
		if putErr == nil {
			return etag, nil
		}
		if !blob.IsAmbiguous(putErr) {
			return "", putErr
		}
		lastErr = putErr
		applied, retry, reconcileErr := r.reconcileReference(ctx, ref, cond)
		switch {
		case reconcileErr != nil:
			return "", errors.Join(putErr, reconcileErr)
		case applied != nil:
			return applied.etag, nil
		case !retry:
			return "", fmt.Errorf("%w: %s:%s was changed by another writer while this write's outcome was unknown", errUtils.ErrArtifactConflict, ref.Name, ref.Version)
		}
	}
	return "", lastErr
}

// reconcileReference reads the reference back after an ambiguous write. It reports the applied
// state when the stored reference equals want, and whether the write provably did not happen.
func (r *Repository) reconcileReference(ctx context.Context, want *layout.Reference, cond blob.Condition) (applied *refState, retry bool, err error) {
	readCtx, cancel := detached(ctx)
	defer cancel()
	current, err := r.loadReference(readCtx, want.Key())
	switch {
	case err != nil:
		return nil, false, err
	case current != nil && *current.ref == *want:
		return current, false, nil
	case cond.Kind == blob.ConditionIfAbsent && current == nil:
		return nil, true, nil
	case cond.Kind == blob.ConditionIfMatch && current != nil && current.etag == cond.ETag:
		return nil, true, nil
	}
	return nil, false, nil
}

// referenceDirPrefix is the listing prefix of the reference objects.
func referenceDirPrefix() string {
	return layout.ReferencesDir() + "/"
}

// listReferences returns every stored reference, sorted by object key, failing on a corrupt one.
// Only objects directly below the references directory are read: journals, staging leftovers,
// claims, and user payloads are never interpreted.
func (r *Repository) listReferences(ctx context.Context) ([]*refState, error) {
	objects, err := r.listPrefix(ctx, referenceDirPrefix())
	if err != nil {
		return nil, err
	}
	var keys []string
	for index := range objects {
		name := strings.TrimPrefix(objects[index].Key, referenceDirPrefix())
		if key, ok := strings.CutSuffix(name, ".json"); ok && !strings.Contains(key, "/") {
			keys = append(keys, key)
		}
	}
	states := make([]*refState, len(keys))
	err = r.forEach(ctx, len(keys), func(ctx context.Context, index int) error {
		state, loadErr := r.loadReference(ctx, keys[index])
		states[index] = state
		return loadErr
	})
	if err != nil {
		return nil, err
	}
	refs := states[:0]
	for _, state := range states {
		if state != nil {
			refs = append(refs, state)
		}
	}
	return refs, nil
}
