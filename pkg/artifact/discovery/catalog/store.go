package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// payloadPath is the inventory path of the catalog document inside its Record.
const payloadPath = "catalog.json"

// payloadMode is the recorded permission of the catalog document.
const payloadMode = 0o644

// catalogDescription describes catalog Records.
const catalogDescription = "Atmos artifact catalog"

// RefOf returns the repository reference of a configured catalog URI. The URI
// ("oci://<registry>/<path>:<tag>") must lie inside the destination repository location
// ("<registry>[/<prefix>]"): a catalog outside it cannot be read or published through this
// repository, so it is rejected rather than followed to another path.
func RefOf(location, uri string) (artifact.Ref, error) {
	defer perf.Track(nil, "catalog.RefOf")()

	reference := strings.TrimPrefix(uri, sourceScheme)
	if reference == uri {
		return artifact.Ref{}, invalid("catalog %q must be an oci://<registry>/<path>:<tag> reference", sanitize(uri))
	}
	root := strings.ToLower(location) + "/"
	lowered := reference
	if separator := strings.Index(reference, "/"); separator > 0 {
		lowered = strings.ToLower(reference[:separator]) + reference[separator:]
	}
	relative, found := strings.CutPrefix(lowered, root)
	if !found || relative == "" {
		return artifact.Ref{}, invalid("catalog %q is outside the repository path %q; a catalog can only be read from and published to the repository it is configured on", sanitize(uri), location)
	}
	ref, err := artifact.ParseRef(relative)
	if err != nil {
		return artifact.Ref{}, fmt.Errorf("%w: catalog %q: %w", errUtils.ErrArtifactCatalogInvalid, sanitize(uri), err)
	}
	return ref, nil
}

// sanitize hides userinfo so a mistaken credential is never echoed.
func sanitize(uri string) string {
	if at := strings.LastIndex(uri, "@"); at >= 0 && strings.Contains(uri[:at], ":") && !strings.Contains(uri[at:], "sha256:") {
		return "oci://<redacted>@" + uri[at+1:]
	}
	return uri
}

// Loaded is a catalog read from a repository.
type Loaded struct {
	// File is the validated manifest.
	File *File

	// Pinned is the immutable reference the catalog was read at.
	Pinned artifact.PinnedReference

	// Digest is the OCI manifest digest, which names this exact catalog revision.
	Digest digest.Digest
}

// Head resolves the catalog without reading its payload, returning the pinned reference whose
// resolved digest names the current catalog. A missing catalog is an ErrArtifactNotFound error.
func Head(ctx context.Context, repo artifact.Repository, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "catalog.Head")()

	pinned, err := repo.Resolve(ctx, ref)
	if err != nil {
		return artifact.PinnedReference{}, fmt.Errorf("resolve catalog %s: %w", ref, err)
	}
	return pinned, nil
}

// Read fetches the catalog at a reference with a bounded, digest-verified read and validates it.
// Only a Record of kind "catalog" with the catalog payload layer is accepted; any other artifact
// found at the reference is rejected.
func Read(ctx context.Context, repo artifact.Repository, ref artifact.Ref) (*Loaded, error) {
	defer perf.Track(nil, "catalog.Read")()

	pinned, err := Head(ctx, repo, ref)
	if err != nil {
		return nil, err
	}
	return ReadPinned(ctx, repo, &pinned)
}

// ReadPinned fetches the catalog at an already resolved reference.
func ReadPinned(ctx context.Context, repo artifact.Repository, pinned *artifact.PinnedReference) (*Loaded, error) {
	defer perf.Track(nil, "catalog.ReadPinned")()

	content, err := repo.Read(ctx, *pinned)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", pinned.Ref, err)
	}
	defer content.Close()
	raw, err := payloadOf(content, pinned.Ref)
	if err != nil {
		return nil, err
	}
	file, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	return &Loaded{File: file, Pinned: *pinned, Digest: pinned.ResolvedDigest}, nil
}

// payloadOf extracts the catalog document from the content of a catalog Record.
func payloadOf(content *artifact.Content, ref artifact.Ref) ([]byte, error) {
	if content.Record == nil || content.Record.Kind != artifact.KindCatalog {
		return nil, invalid("%s is not an artifact catalog; a catalog is a Record of kind %q", ref, artifact.KindCatalog)
	}
	for index := range content.Streams {
		stream := &content.Streams[index]
		if stream.Role != artifact.BlobPayload {
			continue
		}
		if stream.Descriptor.MediaType != artifact.MediaTypeCatalogPayload {
			return nil, invalid("%s has payload type %q, not %q", ref, stream.Descriptor.MediaType, artifact.MediaTypeCatalogPayload)
		}
		raw, err := artifact.ReadBounded(stream.Reader, artifact.MaxRecordSize)
		if err != nil {
			return nil, fmt.Errorf("read catalog %s: %w", ref, err)
		}
		if err := checkInventory(content.Record, raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	return nil, invalid("%s has no catalog payload layer", ref)
}

// checkInventory requires the Record to describe exactly the document that was read.
func checkInventory(record *artifact.Record, raw []byte) error {
	if len(record.Inventory) != 1 || record.Inventory[0].Path != payloadPath || record.Inventory[0].Digest != digest.FromBytes(raw) || record.Inventory[0].Size != int64(len(raw)) {
		return fmt.Errorf("%w: the catalog payload does not match its Record", errUtils.ErrArtifactIntegrityFailed)
	}
	return nil
}

// WriteOptions control one catalog publication.
type WriteOptions struct {
	// Prior is the catalog observed immediately before the write; nil means the catalog must not exist.
	Prior *artifact.PinnedReference

	// Lock is the writer lock already held, if any.
	Lock artifact.LockHandle

	// Now is the creation time recorded in the Record.
	Now time.Time

	// OperationID is the logical operation identifier.
	OperationID string
}

// Write publishes a catalog as a generic Record of kind "catalog". The repository re-checks that the
// prior reference is still current immediately before moving the tag, and a lost race surfaces as
// ErrArtifactCatalogConflict. Without atomic compare-and-swap that check is not a lock, so the caller
// must still serialize catalog writers.
func Write(ctx context.Context, repo artifact.Repository, ref artifact.Ref, file *File, options *WriteOptions) (*artifact.PublishResult, error) {
	defer perf.Track(nil, "catalog.Write")()

	prepared, err := prepare(ref, file, options)
	if err != nil {
		return nil, err
	}
	defer prepared.Close()
	result, err := repo.Publish(ctx, prepared, artifact.PublishOptions{ExpectedPrior: options.Prior, Force: options.Prior != nil, Lock: options.Lock})
	if err != nil {
		if errors.Is(err, errUtils.ErrArtifactConflict) {
			// The catalog sentinel comes first, so the command hint for a catalog conflict wins over
			// the generic one that suggests --force.
			return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactCatalogConflict, err)
		}
		return nil, err
	}
	return result, nil
}

// prepare builds the Record and the staged payload of a catalog.
func prepare(ref artifact.Ref, file *File, options *WriteOptions) (*artifact.PreparedArtifact, error) {
	if ref.Version == "" {
		return nil, invalid("a catalog is published under a tag; %s names a digest", ref)
	}
	Normalize(file)
	raw, err := json.Marshal(file)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > artifact.MaxRecordSize {
		return nil, fmt.Errorf("%w: the catalog is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(raw), artifact.MaxRecordSize)
	}
	if _, err := Parse(raw); err != nil {
		return nil, err
	}
	sum := digest.FromBytes(raw)
	record := artifact.Record{
		Name: ref.Name, Version: ref.Version, Kind: artifact.KindCatalog,
		CreatedAt: artifact.NewTimestamp(options.Now),
		Metadata:  artifact.Metadata{Description: catalogDescription},
		Inventory: []artifact.Entry{{Path: payloadPath, Type: artifact.EntryFile, Size: int64(len(raw)), Mode: payloadMode, Digest: sum}},
		Documents: []artifact.Document{},
	}
	if err := record.Seal(); err != nil {
		return nil, err
	}
	intent, err := record.IntentDigest()
	if err != nil {
		return nil, err
	}
	return &artifact.PreparedArtifact{
		Record: record, IntentDigest: intent, OperationID: options.OperationID, PreparedAt: options.Now.UTC(),
		Packaging: artifact.PackagingCatalog,
		Blobs: []artifact.StagedBlob{{
			Descriptor: ocispec.Descriptor{MediaType: artifact.MediaTypeCatalogPayload, Digest: sum, Size: int64(len(raw))},
			Role:       artifact.BlobPayload, Path: payloadPath,
			Open: func(context.Context) (readCloser, error) { return nopCloser{bytes.NewReader(raw)}, nil },
		}},
	}, nil
}

// readCloser is io.ReadCloser, named locally to keep the staged blob literal short.
type readCloser = io.ReadCloser

// nopCloser closes nothing: the staged bytes live in memory.
type nopCloser struct{ io.Reader }

// Close implements io.Closer.
//
//nolint:lintroller // Trivial no-op.
func (nopCloser) Close() error { return nil }
