package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// objectPlan is one object a publication writes: where it goes (relative to the resolved
// location), where its bytes come from, and what they must verify against.
type objectPlan struct {
	place       func(location string) string
	staged      *artifact.StagedBlob
	data        []byte
	size        int64
	digest      digest.Digest
	contentType string
	payload     bool
}

// open returns a fresh reader over the object's bytes.
func (o *objectPlan) open(ctx context.Context) (io.ReadCloser, error) {
	if o.staged != nil {
		return o.staged.Open(ctx)
	}
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

// plan validates the staged blobs against the Record and lists the objects to write, payload
// first. It runs before any state changes, so a malformed preparation never leaves a pending
// reference behind.
func (j *publishJob) plan() ([]objectPlan, error) {
	payload, docs := j.splitBlobs()
	if err := j.matchDocuments(docs); err != nil {
		return nil, err
	}
	if isArchive(j.prepared.Packaging) {
		return j.archivePlan(payload, docs)
	}
	return j.filesPlan(payload, docs)
}

// sortedDocs returns the document roles in a stable order.
func sortedDocs(docs map[artifact.DocumentRole]*artifact.StagedBlob) []artifact.DocumentRole {
	roles := make([]artifact.DocumentRole, 0, len(docs))
	for role := range docs {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, k int) bool { return roles[i] < roles[k] })
	return roles
}

// filesPlan plans per-file packaging: one object per inventory file, the distribution documents,
// and the manifest.
func (j *publishJob) filesPlan(payload []*artifact.StagedBlob, docs map[artifact.DocumentRole]*artifact.StagedBlob) ([]objectPlan, error) {
	byPath, err := indexPayload(payload)
	if err != nil {
		return nil, err
	}
	items, err := j.fileItems(byPath)
	if err != nil {
		return nil, err
	}
	for _, role := range sortedDocs(docs) {
		items = append(items, j.documentItem(docs[role], func(location string) string {
			return layout.DistributionPath(location, layout.DocumentFileName(role))
		}))
	}
	return append(items, j.metadataItem(func(location string) string { return layout.DistributionPath(location, layout.ManifestName) })), nil
}

// indexPayload maps the staged payload blobs by inventory path.
func indexPayload(payload []*artifact.StagedBlob) (map[string]*artifact.StagedBlob, error) {
	byPath := map[string]*artifact.StagedBlob{}
	for _, staged := range payload {
		if _, duplicate := byPath[staged.Path]; duplicate || staged.Path == "" {
			return nil, fmt.Errorf("%w: payload blob path %q is missing or duplicated", errUtils.ErrArtifactIntegrityFailed, staged.Path)
		}
		byPath[staged.Path] = staged
	}
	return byPath, nil
}

// fileItems plans one object per inventory file and checks each against its staged blob.
func (j *publishJob) fileItems(byPath map[string]*artifact.StagedBlob) ([]objectPlan, error) {
	var items []objectPlan
	for index := range j.record.Inventory {
		entry := &j.record.Inventory[index]
		if entry.Path == layout.DistributionDir || strings.HasPrefix(entry.Path, layout.DistributionDir+"/") {
			return nil, fmt.Errorf("%w: inventory path %q is inside the generated %s directory", errUtils.ErrArtifactUnsafePath, entry.Path, layout.DistributionDir)
		}
		if entry.Type != artifact.EntryFile {
			continue
		}
		staged, ok := byPath[entry.Path]
		if !ok || staged.Descriptor.Digest != entry.Digest || staged.Descriptor.Size != entry.Size {
			return nil, fmt.Errorf("%w: no staged blob matches inventory file %q", errUtils.ErrArtifactIntegrityFailed, entry.Path)
		}
		path := entry.Path
		items = append(items, objectPlan{
			place:  func(location string) string { return location + "/" + path },
			staged: staged, size: entry.Size, digest: entry.Digest, contentType: contentTypeBinary, payload: true,
		})
	}
	if len(items) != len(byPath) {
		return nil, fmt.Errorf("%w: %d staged payload blobs but %d inventory files", errUtils.ErrArtifactIntegrityFailed, len(byPath), len(items))
	}
	return items, nil
}

// archivePlan plans archive packaging: one archive object, the document sidecars, and the
// metadata sidecar.
func (j *publishJob) archivePlan(payload []*artifact.StagedBlob, docs map[artifact.DocumentRole]*artifact.StagedBlob) ([]objectPlan, error) {
	if len(payload) != 1 {
		return nil, fmt.Errorf("%w: an archive publication needs exactly one payload blob", errUtils.ErrArtifactPackagingInvalid)
	}
	staged := payload[0]
	items := []objectPlan{{
		place:  func(location string) string { return location },
		staged: staged, size: staged.Descriptor.Size, digest: staged.Descriptor.Digest, contentType: contentTypeBinary, payload: true,
	}}
	for _, role := range sortedDocs(docs) {
		items = append(items, j.documentItem(docs[role], func(location string) string {
			return layout.DocumentSidecarPath(location, role)
		}))
	}
	return append(items, j.metadataItem(layout.MetadataSidecarPath)), nil
}

func (j *publishJob) documentItem(staged *artifact.StagedBlob, place func(string) string) objectPlan {
	return objectPlan{place: place, staged: staged, size: staged.Descriptor.Size, digest: staged.Descriptor.Digest, contentType: contentTypeMD}
}

// metadataItem is the portable copy of the canonical Record that travels beside the payload.
func (j *publishJob) metadataItem(place func(string) string) objectPlan {
	return objectPlan{place: place, data: j.recordBytes, size: int64(len(j.recordBytes)), digest: j.recordDigest, contentType: contentTypeJSON}
}
