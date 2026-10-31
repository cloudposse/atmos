package local

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// filePayloadMediaType is the media type of one copied file.
const filePayloadMediaType = "application/octet-stream"

// dirStore stores a payload as ordinary copied files below its resolved location, with the
// distribution documents and manifest adjacent at <location>/.atmos/artifact/.
type dirStore struct {
	repo *Repository
}

var _ payloadStore = dirStore{}

func (dirStore) supports(packaging artifact.Packaging) bool {
	return packaging == artifact.PackagingNone
}

// validate checks the staged blobs against the inventory before the publication starts, so a
// malformed preparation never leaves a pending reference behind.
func (dirStore) validate(job *publishJob) error {
	blobs, _, err := job.indexBlobs()
	if err != nil {
		return err
	}
	files := 0
	for index := range job.inventory {
		entry := &job.inventory[index]
		if entry.Path == layout.DistributionDir || strings.HasPrefix(entry.Path, layout.DistributionDir+"/") {
			return fmt.Errorf("%w: inventory path %q is inside the generated %s directory", errUtils.ErrArtifactUnsafePath, entry.Path, layout.DistributionDir)
		}
		if entry.Type != artifact.EntryFile {
			continue
		}
		blob, ok := blobs[entry.Path]
		if !ok || blob.Descriptor.Digest != entry.Digest || blob.Descriptor.Size != entry.Size {
			return fmt.Errorf("%w: no staged blob matches inventory file %q", errUtils.ErrArtifactIntegrityFailed, entry.Path)
		}
		files++
	}
	if files != len(blobs) {
		return fmt.Errorf("%w: %d staged payload blobs but %d inventory files", errUtils.ErrArtifactIntegrityFailed, len(blobs), files)
	}
	return nil
}

// stage copies every inventory entry and the distribution directory into the staging tree.
func (s dirStore) stage(ctx context.Context, t *tree, job *publishJob) error {
	root := job.stagedPayload()
	if err := t.MkdirAll(native(root), privateDir); err != nil {
		return err
	}
	blobs, docs, err := job.indexBlobs()
	if err != nil {
		return err
	}
	if err := s.writeInventory(ctx, t, job, blobs); err != nil {
		return err
	}
	if err := s.writeDistribution(ctx, t, job, docs); err != nil {
		return err
	}
	return s.applyModes(t, job)
}

func (s dirStore) writeInventory(ctx context.Context, t *tree, job *publishJob, blobs map[string]*artifact.StagedBlob) error {
	root := job.stagedPayload()
	for index := range job.inventory {
		entry := &job.inventory[index]
		target := root + "/" + entry.Path
		switch entry.Type {
		case artifact.EntryDirectory:
			if err := t.MkdirAll(native(target), privateDir); err != nil {
				return err
			}
		case artifact.EntrySymlink:
			if err := t.ensureParent(target); err != nil {
				return err
			}
			if err := t.Symlink(native(entry.Target), native(target)); err != nil {
				return fmt.Errorf("%w: create link %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
			}
		default:
			if err := s.stageEntry(ctx, t, blobs[entry.Path], target, entry); err != nil {
				return err
			}
			if err := job.blobStaged(s.repo); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s dirStore) stageEntry(ctx context.Context, t *tree, blob *artifact.StagedBlob, target string, entry *artifact.Entry) error {
	reader, err := blob.Open(ctx)
	if err != nil {
		return err
	}
	copyErr := t.stageFile(target, reader, entry.Digest, entry.Size)
	if closeErr := reader.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("stage %q: %w", entry.Path, copyErr)
	}
	return s.repo.chmodIfSupported(t, target, fsMode(entry.Mode))
}

// writeDistribution writes the documents and the generated manifest.json that the payload
// carries so it stays self-describing. The manifest is the canonical Record.
func (s dirStore) writeDistribution(ctx context.Context, t *tree, job *publishJob, docs map[artifact.DocumentRole]*artifact.StagedBlob) error {
	dist := job.stagedPayload() + "/" + layout.DistributionDir
	if err := t.MkdirAll(native(dist), dirMode); err != nil {
		return err
	}
	for role, blob := range docs {
		reader, err := blob.Open(ctx)
		if err != nil {
			return err
		}
		copyErr := t.stageFile(dist+"/"+layout.DocumentFileName(role), reader, blob.Descriptor.Digest, blob.Descriptor.Size)
		if closeErr := reader.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return fmt.Errorf("stage %s: %w", role, copyErr)
		}
		if err := s.repo.chmodIfSupported(t, dist+"/"+layout.DocumentFileName(role), publicFile); err != nil {
			return err
		}
	}
	return t.writeNew(dist+"/"+layout.ManifestName, job.recordBytes, publicFile)
}

// applyModes sets directory modes last, deepest first, so a read-only directory never blocks the
// writes below it.
func (s dirStore) applyModes(t *tree, job *publishJob) error {
	if !s.repo.hostModes {
		return nil
	}
	root := job.stagedPayload()
	for _, generated := range []string{root, root + "/.atmos", root + "/" + layout.DistributionDir} {
		if err := t.Chmod(native(generated), dirMode); err != nil {
			return err
		}
	}
	dirs := make([]artifact.Entry, 0)
	for index := range job.inventory {
		if job.inventory[index].Type == artifact.EntryDirectory {
			dirs = append(dirs, job.inventory[index])
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i].Path) > len(dirs[j].Path) })
	for index := range dirs {
		if err := t.Chmod(native(root+"/"+dirs[index].Path), fsMode(dirs[index].Mode)); err != nil {
			return err
		}
	}
	return nil
}

// install retires the previous payload directory and renames the staged tree into place.
func (dirStore) install(t *tree, job *publishJob) error {
	if err := t.moveAside(job.location, job.retiredDir()+"/payload"); err != nil {
		return err
	}
	if err := t.ensureParent(job.location); err != nil {
		return err
	}
	return t.Rename(native(job.stagedPayload()), native(job.location))
}

// streams returns one verified stream per inventory file plus the documents.
func (s dirStore) streams(_ context.Context, t *tree, v *view, guard func() error) ([]artifact.ContentStream, error) {
	if ok, err := t.exists(v.rep.Location); err != nil || !ok {
		return nil, fmt.Errorf("%w: payload directory %q is missing", errUtils.ErrArtifactIntegrityFailed, v.rep.Location)
	}
	streams := make([]artifact.ContentStream, 0, len(v.record.Inventory)+len(v.record.Documents))
	for index := range v.record.Inventory {
		entry := &v.record.Inventory[index]
		if entry.Type != artifact.EntryFile {
			continue
		}
		file, _, err := t.openFile(v.rep.Location+"/"+entry.Path, "payload file")
		if err != nil {
			return streams, err
		}
		streams = append(streams, artifact.ContentStream{
			Descriptor: ocispec.Descriptor{MediaType: filePayloadMediaType, Digest: entry.Digest, Size: entry.Size},
			Role:       artifact.BlobPayload,
			Path:       entry.Path,
			Reader:     newVerifyingReader(file, entry.Digest, entry.Size, guard),
		})
	}
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

func (dirStore) document(t *tree, v *view, doc *artifact.Document, guard func() error) (io.ReadCloser, error) {
	file, _, err := t.openFile(layout.DistributionPath(v.rep.Location, layout.DocumentFileName(doc.Role)), string(doc.Role)+" document")
	if err != nil {
		return nil, err
	}
	return newVerifyingReader(file, doc.Digest, doc.Size, guard), nil
}

func (dirStore) verifySidecars(t *tree, v *view) error {
	return t.verifyBytes(layout.DistributionPath(v.rep.Location, layout.ManifestName), "manifest", v.recordBytes)
}

// remove deletes the inventory files, the distribution directory, and then the directories that
// became empty. Files the artifact does not own stay in place and keep their directory alive.
func (dirStore) remove(t *tree, v *view) ([]string, error) {
	location := v.rep.Location
	if ok, err := t.exists(location); err != nil || !ok {
		return nil, err
	}
	t.makeWritable(location)
	if err := removeFiles(t, location, v.record); err != nil {
		return nil, err
	}
	removeDirectories(t, location, v.record)
	return []string{t.absolute(location)}, nil
}

// removeFiles deletes every non-directory inventory entry and the generated distribution files.
func removeFiles(t *tree, location string, record *artifact.Record) error {
	for index := range record.Inventory {
		if entry := &record.Inventory[index]; entry.Type != artifact.EntryDirectory {
			if err := removeIfPresent(t, location+"/"+entry.Path); err != nil {
				return err
			}
		}
	}
	for _, name := range []string{layout.ManifestName, "README.md", "CHANGELOG.md"} {
		if err := removeIfPresent(t, layout.DistributionPath(location, name)); err != nil {
			return err
		}
	}
	return nil
}

// removeDirectories removes the artifact's directories deepest first. A directory that still holds
// unmanaged files is left in place.
func removeDirectories(t *tree, location string, record *artifact.Record) {
	var dirs []string
	for index := range record.Inventory {
		if record.Inventory[index].Type == artifact.EntryDirectory {
			dirs = append(dirs, location+"/"+record.Inventory[index].Path)
		}
	}
	dirs = append(dirs, location+"/"+layout.DistributionDir, location+"/.atmos", location)
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = t.Remove(native(dir))
	}
}

// removeIfPresent removes a path, ignoring a path that is already gone.
func removeIfPresent(t *tree, rel string) error {
	if ok, err := t.exists(rel); err != nil || !ok {
		return err
	}
	return t.Remove(native(rel))
}

// dirRepresentationDigest is the checksum of the per-file payload: the SHA-256 of the sorted
// "path NUL digest LF" lines.
func dirRepresentationDigest(record *artifact.Record) digest.Digest {
	hasher := sha256.New()
	for index := range record.Inventory {
		if entry := &record.Inventory[index]; entry.Type == artifact.EntryFile {
			_, _ = io.WriteString(hasher, entry.Path+"\x00"+entry.Digest.String()+"\n")
		}
	}
	return digest.NewDigest(digest.SHA256, hasher)
}
