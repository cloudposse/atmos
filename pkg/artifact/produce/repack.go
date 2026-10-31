package produce

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RepackRequest rebuilds an existing, sealed Record for a different packaging. The Record bytes
// and content identity are preserved exactly; only the physical representation changes.
type RepackRequest struct {
	// Record is the existing Record. It is not modified.
	Record artifact.Record

	// Tree is a directory holding the materialized inventory (without the reserved directory).
	Tree string

	// Documents holds the document bytes by role; each must match its Record descriptor.
	Documents map[artifact.DocumentRole][]byte

	// Packaging is the destination packaging.
	Packaging artifact.Packaging

	// OperationID is the logical operation identifier.
	OperationID string

	// PreparedAt is the preparation time of this copy operation (the Record keeps its own created_at).
	PreparedAt time.Time
}

// Repack stages an existing Record from a materialized tree for the requested packaging. Every
// file read from the tree is verified against its inventory digest.
func (p *Producer) Repack(ctx context.Context, request *RepackRequest) (*artifact.PreparedArtifact, error) {
	defer perf.Track(nil, "produce.Producer.Repack")()

	if request.OperationID == "" || request.PreparedAt.IsZero() {
		return nil, fmt.Errorf("%w: the operation ID and preparation time are required", errUtils.ErrArtifactConfigInvalid)
	}
	record := request.Record
	if err := record.Validate(); err != nil {
		return nil, err
	}
	docs, err := repackDocuments(&record, request.Documents)
	if err != nil {
		return nil, err
	}
	stagingDir, err := os.MkdirTemp(p.stagingRoot, "atmos-artifact-repack-*")
	if err != nil {
		return nil, fmt.Errorf("%w: create staging directory: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	prepared, err := finalize(ctx, &finalizeInput{
		record: &record, docs: docs, open: treeOpener(&record, request.Tree), stagingDir: stagingDir, packaging: request.Packaging,
		operationID: request.OperationID, preparedAt: request.PreparedAt, cleanup: func() error { return os.RemoveAll(stagingDir) },
	})
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}
	return prepared, nil
}

// treeOpener opens inventory files from a materialized tree, verifying each against its digest.
func treeOpener(record *artifact.Record, tree string) opener {
	inventory := make(map[string]*artifact.Entry, len(record.Inventory))
	for index := range record.Inventory {
		inventory[record.Inventory[index].Path] = &record.Inventory[index]
	}
	return func(entryPath string) (io.ReadCloser, error) {
		entry, ok := inventory[entryPath]
		if !ok || entry.Type != artifact.EntryFile {
			return nil, fmt.Errorf("%w: %q is not an inventory file", errUtils.ErrArtifactNotFound, entryPath)
		}
		file, err := os.Open(filepath.Join(tree, filepath.FromSlash(entryPath)))
		if err != nil {
			return nil, err
		}
		return newVerifyingReader(file, entry.Digest, entry.Size), nil
	}
}

// repackDocuments pairs each Record document descriptor with its bytes and verifies the digest.
func repackDocuments(record *artifact.Record, provided map[artifact.DocumentRole][]byte) ([]stagedDocument, error) {
	docs := make([]stagedDocument, 0, len(record.Documents))
	for _, descriptor := range record.Documents {
		data, ok := provided[descriptor.Role]
		if !ok {
			return nil, fmt.Errorf("%w: %s bytes are required to repack", errUtils.ErrArtifactDocumentMissing, descriptor.Role)
		}
		if int64(len(data)) != descriptor.Size || digest.FromBytes(data) != descriptor.Digest {
			return nil, fmt.Errorf("%w: %s does not match its descriptor", errUtils.ErrArtifactIntegrityFailed, descriptor.Role)
		}
		docs = append(docs, stagedDocument{descriptor: descriptor, data: data})
	}
	return docs, nil
}

// verifyingReader fails at EOF unless the bytes read match the expected digest and size.
type verifyingReader struct {
	rc       io.ReadCloser
	hasher   hash.Hash
	expected digest.Digest
	size     int64
	read     int64
}

func newVerifyingReader(rc io.ReadCloser, expected digest.Digest, size int64) *verifyingReader {
	return &verifyingReader{rc: rc, hasher: sha256.New(), expected: expected, size: size}
}

func (v *verifyingReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "produce.verifyingReader.Read")()

	n, err := v.rc.Read(p)
	v.read += int64(n)
	_, _ = v.hasher.Write(p[:n])
	if err == io.EOF {
		if v.read != v.size || digest.NewDigest(digest.SHA256, v.hasher) != v.expected {
			return n, fmt.Errorf("%w: file content does not match its inventory digest", errUtils.ErrArtifactIntegrityFailed)
		}
	}
	return n, err
}

func (v *verifyingReader) Close() error {
	defer perf.Track(nil, "produce.verifyingReader.Close")()

	return v.rc.Close()
}
