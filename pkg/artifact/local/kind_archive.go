package local

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// Staging names inside an operation's staging directory.
const (
	stagedArchive  = "archive"
	stagedMetadata = "metadata.json"
	stagedDocs     = "docs"
)

// archiveStore stores a payload as one tar, tgz, or zip file with adjacent metadata and document
// sidecars. The configured archive permissions apply to the file and its sidecars, never to the
// members inside the archive.
type archiveStore struct {
	repo *Repository
}

var _ payloadStore = archiveStore{}

func (archiveStore) supports(packaging artifact.Packaging) bool {
	switch packaging {
	case artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		return true
	default:
		return false
	}
}

// validate checks that the publication carries exactly one archive blob matching the Record's
// documents, before any state changes.
func (archiveStore) validate(job *publishJob) error {
	_, _, err := job.archiveBlobs()
	return err
}

// stage copies the archive and the document sidecars into the staging directory and applies the
// archive permissions before anything becomes visible at the final location.
func (s archiveStore) stage(ctx context.Context, t *tree, job *publishJob) error {
	payload, docs, err := job.archiveBlobs()
	if err != nil {
		return err
	}
	archivePath := job.staging + "/" + stagedArchive
	if err := s.copyBlob(ctx, t, payload, archivePath); err != nil {
		return err
	}
	if err := job.blobStaged(s.repo); err != nil {
		return err
	}
	if err := s.repo.setArchiveMode(t, archivePath, s.repo.archiveMode); err != nil {
		return err
	}
	if err := s.stageDocuments(ctx, t, job, docs); err != nil {
		return err
	}
	metadata := job.staging + "/" + stagedMetadata
	if err := t.writeNew(metadata, job.recordBytes, privateFile); err != nil {
		return err
	}
	return s.repo.setArchiveMode(t, metadata, s.repo.archiveMode)
}

// stageDocuments copies the document sidecars and gives their directory the matching mode.
func (s archiveStore) stageDocuments(ctx context.Context, t *tree, job *publishJob, docs map[artifact.DocumentRole]*artifact.StagedBlob) error {
	for role, blob := range docs {
		docPath := job.staging + "/" + stagedDocs + "/" + layout.DocumentFileName(role)
		if err := s.copyBlob(ctx, t, blob, docPath); err != nil {
			return err
		}
		if err := s.repo.setArchiveMode(t, docPath, s.repo.archiveMode); err != nil {
			return err
		}
	}
	if len(docs) == 0 {
		return nil
	}
	return s.repo.setArchiveMode(t, job.staging+"/"+stagedDocs, directoryMode(s.repo.archiveMode))
}

func (archiveStore) copyBlob(ctx context.Context, t *tree, blob *artifact.StagedBlob, dest string) error {
	reader, err := blob.Open(ctx)
	if err != nil {
		return err
	}
	copyErr := t.stageFile(dest, reader, blob.Descriptor.Digest, blob.Descriptor.Size)
	if closeErr := reader.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("stage %s: %w", dest, copyErr)
	}
	return nil
}

// install retires the previous archive and sidecars, then renames the staged ones into place.
func (archiveStore) install(t *tree, job *publishJob) error {
	retired := job.retiredDir()
	for _, move := range []struct{ from, to string }{
		{job.location, retired + "/archive"},
		{layout.DocsSidecarDir(job.location), retired + "/docs"},
		{layout.MetadataSidecarPath(job.location), retired + "/metadata.json"},
	} {
		if err := t.moveAside(move.from, move.to); err != nil {
			return err
		}
	}
	if err := t.ensureParent(job.location); err != nil {
		return err
	}
	installs := []struct{ from, to string }{{job.staging + "/" + stagedArchive, job.location}}
	if ok, err := t.exists(job.staging + "/" + stagedDocs); err != nil {
		return err
	} else if ok {
		installs = append(installs, struct{ from, to string }{job.staging + "/" + stagedDocs, layout.DocsSidecarDir(job.location)})
	}
	installs = append(installs, struct{ from, to string }{job.staging + "/" + stagedMetadata, layout.MetadataSidecarPath(job.location)})
	for _, install := range installs {
		if err := t.Rename(native(install.from), native(install.to)); err != nil {
			return err
		}
	}
	return nil
}

// streams returns the archive as the single payload stream plus the document sidecars.
func (s archiveStore) streams(_ context.Context, t *tree, v *view, guard func() error) ([]artifact.ContentStream, error) {
	if len(v.rep.Payload) != 1 {
		return nil, fmt.Errorf("%w: an archive representation must have exactly one payload, found %d", errUtils.ErrArtifactIntegrityFailed, len(v.rep.Payload))
	}
	descriptor := v.rep.Payload[0]
	file, info, err := t.openFile(v.rep.Location, "archive")
	if err != nil {
		return nil, err
	}
	if info.Size() != descriptor.Size {
		_ = file.Close()
		return nil, fmt.Errorf("%w: archive %q is %d bytes; the representation declares %d", errUtils.ErrArtifactIntegrityFailed, v.rep.Location, info.Size(), descriptor.Size)
	}
	streams := []artifact.ContentStream{{
		Descriptor: descriptor, Role: artifact.BlobPayload,
		Reader: newVerifyingReader(file, descriptor.Digest, descriptor.Size, guard),
	}}
	for index := range v.record.Documents {
		doc := &v.record.Documents[index]
		reader, err := s.document(t, v, doc, guard)
		if err != nil {
			return streams, err
		}
		streams = append(streams, artifact.ContentStream{Descriptor: blobDescriptor(doc), Role: artifact.BlobDocument, Reader: reader})
	}
	return streams, nil
}

// document opens a document from its adjacent sidecar. A missing sidecar is an integrity failure:
// reading it must not fall back to opening the archive, and the completed reference promised the
// sidecar exists.
func (archiveStore) document(t *tree, v *view, doc *artifact.Document, guard func() error) (io.ReadCloser, error) {
	file, _, err := t.openFile(layout.DocumentSidecarPath(v.rep.Location, doc.Role), string(doc.Role)+" sidecar")
	if err != nil {
		return nil, err
	}
	return newVerifyingReader(file, doc.Digest, doc.Size, guard), nil
}

func (archiveStore) verifySidecars(t *tree, v *view) error {
	return t.verifyBytes(layout.MetadataSidecarPath(v.rep.Location), "metadata sidecar", v.recordBytes)
}

// remove deletes the archive, its metadata sidecar, and its document sidecars.
func (archiveStore) remove(t *tree, v *view) ([]string, error) {
	location := v.rep.Location
	removed := []string{}
	targets := []string{location, layout.MetadataSidecarPath(location)}
	for index := range v.record.Documents {
		targets = append(targets, layout.DocumentSidecarPath(location, v.record.Documents[index].Role))
	}
	for _, target := range targets {
		ok, err := t.exists(target)
		if err != nil {
			return removed, err
		}
		if !ok {
			continue
		}
		if err := t.Remove(native(target)); err != nil {
			return removed, err
		}
		removed = append(removed, t.absolute(target))
	}
	_ = t.Remove(native(layout.DocsSidecarDir(location)))
	return removed, nil
}

// readBits holds the read permission bits; execute follows read for a directory.
const readBits fs.FileMode = 0o444

// directoryMode derives the directory mode that goes with a file mode: execute follows read.
func directoryMode(file fs.FileMode) fs.FileMode {
	return file | (file&readBits)>>2
}
