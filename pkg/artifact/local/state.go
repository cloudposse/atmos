package local

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// readManaged reads a managed document, mapping a missing file to (nil, nil).
func (t *tree) readManaged(rel string, limit int64) ([]byte, error) {
	raw, err := t.readBounded(rel, limit)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: read %s: %w", errUtils.ErrArtifactIntegrityFailed, rel, err)
	}
	return raw, nil
}

// loadReference returns the stored reference, or nil when it does not exist.
func (t *tree) loadReference(key string) (*layout.Reference, error) {
	raw, err := t.readManaged(layout.ReferencePath(key), layout.MaxDocumentBytes)
	if err != nil || raw == nil {
		return nil, err
	}
	ref, err := layout.DecodeReference(raw)
	if err != nil {
		return nil, fmt.Errorf("reference %s: %w", key, err)
	}
	if ref.Key() != key {
		return nil, fmt.Errorf("%w: reference file %s describes %s:%s", errUtils.ErrArtifactIntegrityFailed, key, ref.Name, ref.Version)
	}
	return ref, nil
}

// loadRepresentation returns the stored representation, or nil when it does not exist.
func (t *tree) loadRepresentation(key string) (*layout.Representation, error) {
	raw, err := t.readManaged(layout.RepresentationPath(key), layout.MaxDocumentBytes)
	if err != nil || raw == nil {
		return nil, err
	}
	rep, err := layout.DecodeRepresentation(raw)
	if err != nil {
		return nil, fmt.Errorf("representation %s: %w", key, err)
	}
	if layout.ReferenceKey(rep.Name, rep.Version) != key {
		return nil, fmt.Errorf("%w: representation file %s describes %s:%s", errUtils.ErrArtifactIntegrityFailed, key, rep.Name, rep.Version)
	}
	return rep, nil
}

// loadOperation returns the stored journal, or nil when it does not exist.
func (t *tree) loadOperation(operationID string) (*layout.Operation, error) {
	raw, err := t.readManaged(layout.OperationPath(operationID), layout.MaxDocumentBytes)
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
// not found; a Record whose bytes do not hash to its file name, or that is not canonical, fails
// integrity.
func (t *tree) loadRecord(recordDigest digest.Digest) (*artifact.Record, []byte, error) {
	if err := artifact.ValidateDigest(recordDigest); err != nil {
		return nil, nil, err
	}
	raw, err := t.readManaged(layout.RecordPath(recordDigest), artifact.MaxRecordSize)
	if err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, recordDigest)
	}
	if got := digest.FromBytes(raw); got != recordDigest {
		return nil, nil, fmt.Errorf("%w: record file %s holds bytes that hash to %s", errUtils.ErrArtifactIntegrityFailed, recordDigest, got)
	}
	record, err := artifact.DecodeRecord(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: record %s: %w", errUtils.ErrArtifactIntegrityFailed, recordDigest, err)
	}
	return record, raw, nil
}

// writeReference replaces the reference record atomically.
func (t *tree) writeReference(ref *layout.Reference) error {
	raw, err := ref.Marshal()
	if err != nil {
		return err
	}
	return t.writeAtomic(layout.ReferencePath(ref.Key()), raw)
}

// writeRepresentation replaces the representation record atomically.
func (t *tree) writeRepresentation(rep *layout.Representation) error {
	raw, err := rep.Marshal()
	if err != nil {
		return err
	}
	return t.writeAtomic(layout.RepresentationPath(layout.ReferenceKey(rep.Name, rep.Version)), raw)
}

// writeOperation replaces the operation journal atomically.
func (t *tree) writeOperation(op *layout.Operation) error {
	raw, err := op.Marshal()
	if err != nil {
		return err
	}
	return t.writeAtomic(layout.OperationPath(op.OperationID), raw)
}

// writeRecord stores the immutable canonical Record. An existing file with identical bytes is
// left alone; different bytes under the same digest name are corruption.
func (t *tree) writeRecord(recordDigest digest.Digest, canonical []byte) error {
	existing, err := t.readManaged(layout.RecordPath(recordDigest), artifact.MaxRecordSize)
	if err != nil {
		return err
	}
	if existing != nil && string(existing) != string(canonical) {
		return fmt.Errorf("%w: record file %s already exists with different bytes", errUtils.ErrArtifactIntegrityFailed, recordDigest)
	}
	if existing != nil {
		return nil
	}
	return t.writeAtomic(layout.RecordPath(recordDigest), canonical)
}

// listReferences returns every stored reference, sorted by file name, failing on a corrupt one.
func (t *tree) listReferences() ([]*layout.Reference, error) {
	entries, err := fs.ReadDir(t.FS(), layout.ReferencesDir())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: list references: %w", errUtils.ErrArtifactIntegrityFailed, err)
	}
	refs := make([]*layout.Reference, 0, len(entries))
	for _, entry := range entries {
		key, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok {
			continue
		}
		ref, err := t.loadReference(key)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
