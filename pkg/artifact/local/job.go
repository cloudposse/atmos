package local

import (
	"fmt"
	"io/fs"
	"runtime"
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
	inventory    []artifact.Entry
	key          string
	opID         string
	location     string
	staging      string
	payload      []ocispec.Descriptor
	documents    []ocispec.Descriptor
	staged       int
	installing   bool
}

// stagedPayload is the staging path that becomes the payload directory.
func (j *publishJob) stagedPayload() string {
	return j.staging + "/payload"
}

// retiredDir is where replaced content is moved aside until the new reference is completed.
func (j *publishJob) retiredDir() string {
	return j.staging + "/retired"
}

// blobStaged records one staged payload blob and fires the during-payload fault after the first.
func (j *publishJob) blobStaged(r *Repository) error {
	j.staged++
	if j.staged == 1 {
		return r.settings.fault(faultDuringPayload)
	}
	return nil
}

// newJob validates a prepared artifact against the repository and derives the publication inputs.
func (r *Repository) newJob(prepared *artifact.PreparedArtifact) (*publishJob, error) {
	if prepared == nil {
		return nil, fmt.Errorf("%w: no prepared artifact to publish", errUtils.ErrArtifactConfigInvalid)
	}
	if !r.store.supports(prepared.Packaging) {
		return nil, artifact.NewUnsupportedCapability(r.alias, "publish", "packaging/"+string(prepared.Packaging),
			fmt.Sprintf("%s repositories store %v payloads", r.kind, r.Capabilities().Packagings))
	}
	if err := r.checkArchivePermissions(); err != nil {
		return nil, err
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
		inventory: sortedEntries(record.Inventory), key: layout.ReferenceKey(record.Name, record.Version),
		opID: opID, staging: layout.StagingPath(opID),
	}
	if err := job.describeBlobs(); err != nil {
		return nil, err
	}
	if err := r.store.validate(job); err != nil {
		return nil, err
	}
	return job, nil
}

func sortedEntries(entries []artifact.Entry) []artifact.Entry {
	out := append([]artifact.Entry(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// describeBlobs derives the representation descriptors from the staged blobs and checks that the
// blobs agree with the Record before anything is written.
func (j *publishJob) describeBlobs() error {
	j.payload, j.documents = []ocispec.Descriptor{}, []ocispec.Descriptor{}
	docs := map[artifact.DocumentRole]bool{}
	for index := range j.prepared.Blobs {
		blob := &j.prepared.Blobs[index]
		if blob.Open == nil {
			return fmt.Errorf("%w: staged blob %q cannot be opened", errUtils.ErrArtifactConfigInvalid, blob.Path)
		}
		descriptor := blob.Descriptor
		if blob.Role == artifact.BlobDocument {
			if docs[blob.DocumentRole] {
				return fmt.Errorf("%w: duplicate %s document blob", errUtils.ErrArtifactRecordInvalid, blob.DocumentRole)
			}
			docs[blob.DocumentRole] = true
			descriptor.Annotations = map[string]string{ocispec.AnnotationTitle: layout.DocumentFileName(blob.DocumentRole)}
			j.documents = append(j.documents, descriptor)
			continue
		}
		if blob.Path != "" {
			descriptor.Annotations = map[string]string{ocispec.AnnotationTitle: blob.Path}
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

// indexBlobs maps file payload blobs by inventory path and document blobs by role.
func (j *publishJob) indexBlobs() (map[string]*artifact.StagedBlob, map[artifact.DocumentRole]*artifact.StagedBlob, error) {
	files := map[string]*artifact.StagedBlob{}
	docs := map[artifact.DocumentRole]*artifact.StagedBlob{}
	for index := range j.prepared.Blobs {
		blob := &j.prepared.Blobs[index]
		if blob.Role == artifact.BlobDocument {
			docs[blob.DocumentRole] = blob
			continue
		}
		if _, duplicate := files[blob.Path]; duplicate || blob.Path == "" {
			return nil, nil, fmt.Errorf("%w: payload blob path %q is missing or duplicated", errUtils.ErrArtifactIntegrityFailed, blob.Path)
		}
		files[blob.Path] = blob
	}
	if err := j.matchDocuments(docs); err != nil {
		return nil, nil, err
	}
	return files, docs, nil
}

// archiveBlobs returns the single archive payload blob and the document blobs.
func (j *publishJob) archiveBlobs() (*artifact.StagedBlob, map[artifact.DocumentRole]*artifact.StagedBlob, error) {
	var payload *artifact.StagedBlob
	docs := map[artifact.DocumentRole]*artifact.StagedBlob{}
	for index := range j.prepared.Blobs {
		blob := &j.prepared.Blobs[index]
		if blob.Role == artifact.BlobDocument {
			docs[blob.DocumentRole] = blob
			continue
		}
		if payload != nil {
			return nil, nil, fmt.Errorf("%w: an archive publication needs exactly one payload blob", errUtils.ErrArtifactPackagingInvalid)
		}
		payload = blob
	}
	if payload == nil {
		return nil, nil, fmt.Errorf("%w: an archive publication needs exactly one payload blob", errUtils.ErrArtifactPackagingInvalid)
	}
	if err := j.matchDocuments(docs); err != nil {
		return nil, nil, err
	}
	return payload, docs, nil
}

// matchDocuments checks every document blob against the Record's descriptor for its role.
func (j *publishJob) matchDocuments(docs map[artifact.DocumentRole]*artifact.StagedBlob) error {
	for role, blob := range docs {
		doc, ok := documentDescriptor(j.record, role)
		if !ok || doc.Digest != blob.Descriptor.Digest || doc.Size != blob.Descriptor.Size {
			return fmt.Errorf("%w: the staged %s document does not match the Record", errUtils.ErrArtifactIntegrityFailed, role)
		}
	}
	return nil
}

// representationDigest is the checksum of the transferred payload representation.
func (j *publishJob) representationDigest(packaging artifact.Packaging) digest.Digest {
	if packaging == artifact.PackagingNone {
		return dirRepresentationDigest(j.record)
	}
	if len(j.payload) == 1 {
		return j.payload[0].Digest
	}
	return ""
}

// permissionBits holds the ordinary permission bits of a mode.
const permissionBits fs.FileMode = 0o777

// fsMode converts a Record mode to a file mode.
func fsMode(mode uint32) fs.FileMode {
	return fs.FileMode(mode) & permissionBits
}

// checkArchivePermissions fails before any state changes when an explicitly requested archive
// mode cannot be enforced on this host.
func (r *Repository) checkArchivePermissions() error {
	if r.kind != artifact.RepositoryKindLocalArchive || !r.modeExplicit || r.hostModes {
		return nil
	}
	return artifact.NewUnsupportedCapability(r.alias, "publish", "permissions/archive-file-mode",
		fmt.Sprintf("the host (%s) cannot enforce permissions %04o on the archive file; remove the permissions setting to accept the host default, or publish from a POSIX host", runtime.GOOS, r.archiveMode))
}

// chmodIfSupported applies a mode when the host can enforce one.
func (r *Repository) chmodIfSupported(t *tree, rel string, mode fs.FileMode) error {
	if !r.hostModes {
		return nil
	}
	return t.Chmod(native(rel), mode)
}

// setArchiveMode applies the archive permissions to a staged archive file, sidecar, or sidecar
// directory. For an explicit request it also reads the mode back, because a filesystem that
// silently ignores chmod must not be reported as enforcing the configured permissions.
func (r *Repository) setArchiveMode(t *tree, rel string, mode fs.FileMode) error {
	if !r.hostModes {
		return nil
	}
	if err := t.Chmod(native(rel), mode); err != nil {
		return err
	}
	if !r.modeExplicit {
		return nil
	}
	info, err := t.Lstat(native(rel))
	if err != nil {
		return err
	}
	if info.Mode().Perm() != mode {
		return artifact.NewUnsupportedCapability(r.alias, "publish", "permissions/archive-file-mode",
			fmt.Sprintf("the filesystem under %q did not keep mode %04o (got %04o)", r.root, mode, info.Mode().Perm()))
	}
	return nil
}
