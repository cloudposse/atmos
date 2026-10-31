package s3

import (
	"crypto/sha256"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// publishJob is everything one publication attempt derives from the prepared artifact, computed
// once before any state changes.
type publishJob struct {
	prepared     *artifact.PreparedArtifact
	record       *artifact.Record
	recordBytes  []byte
	recordDigest digest.Digest
	key          string
	opID         string
	location     string
	payload      []ocispec.Descriptor
	documents    []ocispec.Descriptor
}

// newJob validates a prepared artifact against the repository and derives the publication inputs.
func (r *Repository) newJob(prepared *artifact.PreparedArtifact) (*publishJob, error) {
	if prepared == nil {
		return nil, fmt.Errorf("%w: no prepared artifact to publish", errUtils.ErrArtifactConfigInvalid)
	}
	if !supportedPackaging(prepared.Packaging) {
		return nil, artifact.NewUnsupportedCapability(r.alias, "publish", "packaging/"+string(prepared.Packaging),
			fmt.Sprintf("%s repositories store %v payloads", artifact.RepositoryKindS3, r.Capabilities().Packagings))
	}
	canonical, err := prepared.Record.Canonical()
	if err != nil {
		return nil, err
	}
	record := prepared.Record
	opID := prepared.OperationID
	if opID == "" {
		opID = uuid.NewString()
	}
	job := &publishJob{
		prepared: prepared, record: &record, recordBytes: canonical, recordDigest: digest.FromBytes(canonical),
		key: layout.ReferenceKey(record.Name, record.Version), opID: opID,
	}
	if err := job.describeBlobs(); err != nil {
		return nil, err
	}
	if _, err := job.plan(); err != nil {
		return nil, err
	}
	return job, nil
}

// describeBlobs derives the representation descriptors from the staged blobs and checks that the
// blobs agree with the Record before anything is written.
func (j *publishJob) describeBlobs() error {
	j.payload, j.documents = []ocispec.Descriptor{}, []ocispec.Descriptor{}
	docs := map[artifact.DocumentRole]bool{}
	for index := range j.prepared.Blobs {
		staged := &j.prepared.Blobs[index]
		if staged.Open == nil {
			return fmt.Errorf("%w: staged blob %q cannot be opened", errUtils.ErrArtifactConfigInvalid, staged.Path)
		}
		descriptor := staged.Descriptor
		if staged.Role == artifact.BlobDocument {
			if docs[staged.DocumentRole] {
				return fmt.Errorf("%w: duplicate %s document blob", errUtils.ErrArtifactRecordInvalid, staged.DocumentRole)
			}
			docs[staged.DocumentRole] = true
			descriptor.Annotations = map[string]string{ocispec.AnnotationTitle: layout.DocumentFileName(staged.DocumentRole)}
			j.documents = append(j.documents, descriptor)
			continue
		}
		if staged.Path != "" {
			descriptor.Annotations = map[string]string{ocispec.AnnotationTitle: staged.Path}
		}
		j.payload = append(j.payload, descriptor)
	}
	for index := range j.record.Documents {
		if !docs[j.record.Documents[index].Role] {
			return fmt.Errorf("%w: the Record lists a %s document with no staged blob", errUtils.ErrArtifactIntegrityFailed, j.record.Documents[index].Role)
		}
	}
	if len(docs) != len(j.record.Documents) {
		return fmt.Errorf("%w: %d document blobs but %d documents in the Record", errUtils.ErrArtifactIntegrityFailed, len(docs), len(j.record.Documents))
	}
	return nil
}

// splitBlobs separates the staged document blobs (by role) from the payload blobs.
func (j *publishJob) splitBlobs() (payload []*artifact.StagedBlob, docs map[artifact.DocumentRole]*artifact.StagedBlob) {
	docs = map[artifact.DocumentRole]*artifact.StagedBlob{}
	for index := range j.prepared.Blobs {
		staged := &j.prepared.Blobs[index]
		if staged.Role == artifact.BlobDocument {
			docs[staged.DocumentRole] = staged
			continue
		}
		payload = append(payload, staged)
	}
	return payload, docs
}

// matchDocuments checks every document blob against the Record's descriptor for its role.
func (j *publishJob) matchDocuments(docs map[artifact.DocumentRole]*artifact.StagedBlob) error {
	for role, staged := range docs {
		doc, ok := documentDescriptor(j.record, role)
		if !ok || doc.Digest != staged.Descriptor.Digest || doc.Size != staged.Descriptor.Size {
			return fmt.Errorf("%w: the staged %s document does not match the Record", errUtils.ErrArtifactIntegrityFailed, role)
		}
	}
	return nil
}

// documentDescriptor returns the Record's descriptor for a role.
func documentDescriptor(record *artifact.Record, role artifact.DocumentRole) (*artifact.Document, bool) {
	for index := range record.Documents {
		if record.Documents[index].Role == role {
			return &record.Documents[index], true
		}
	}
	return nil, false
}

// representationDigest is the checksum of the transferred payload representation: the archive
// digest, or a digest of the sorted per-file payload digests.
func (j *publishJob) representationDigest() digest.Digest {
	if isArchive(j.prepared.Packaging) {
		if len(j.payload) == 1 {
			return j.payload[0].Digest
		}
		return ""
	}
	hasher := sha256.New()
	files := setOfRecord(j.record).files
	sort.Strings(files)
	byPath := map[string]digest.Digest{}
	for index := range j.record.Inventory {
		byPath[j.record.Inventory[index].Path] = j.record.Inventory[index].Digest
	}
	for _, file := range files {
		_, _ = io.WriteString(hasher, file+"\x00"+byPath[file].String()+"\n")
	}
	return digest.NewDigest(digest.SHA256, hasher)
}
