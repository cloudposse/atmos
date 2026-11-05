package produce

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Producer is the generic "files" producer. It satisfies [artifact.Producer].
type Producer struct {
	stagingRoot string
	limits      snapshot.Limits
	metadata    MetadataSource
	snapshotOps []snapshot.Option

	// profile layers kind-specific validation, metadata, and visible default exclusions over the
	// shared pipeline. It is nil for the generic files producer, which has none.
	profile *kindProfile

	// scaffoldValidator is the injected scaffold validator of a scaffold producer.
	scaffoldValidator ScaffoldValidator
}

var _ artifact.Producer = (*Producer)(nil)

// Option customizes a [Producer].
type Option func(*Producer)

// WithStagingRoot sets the directory that holds bounded disk staging.
func WithStagingRoot(dir string) Option {
	defer perf.Track(nil, "produce.WithStagingRoot")()

	return func(p *Producer) { p.stagingRoot = dir }
}

// WithLimits overrides the staging limits.
func WithLimits(limits snapshot.Limits) Option {
	defer perf.Track(nil, "produce.WithLimits")()

	return func(p *Producer) { p.limits = limits }
}

// WithMetadataSource supplies producer metadata (name, version, description, labels, kind data).
func WithMetadataSource(source MetadataSource) Option {
	defer perf.Track(nil, "produce.WithMetadataSource")()

	return func(p *Producer) { p.metadata = source }
}

// WithSnapshotOptions passes extra options to the snapshot capture, for example a capture hook in tests.
func WithSnapshotOptions(opts ...snapshot.Option) Option {
	defer perf.Track(nil, "produce.WithSnapshotOptions")()

	return func(p *Producer) { p.snapshotOps = append(p.snapshotOps, opts...) }
}

// New returns a files producer.
func New(opts ...Option) *Producer {
	defer perf.Track(nil, "produce.New")()

	p := &Producer{}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// Register adds a files producer to registry.
func Register(registry *artifact.ProducerRegistry, opts ...Option) error {
	defer perf.Track(nil, "produce.Register")()

	return registry.Register(New(opts...))
}

// Kind returns [artifact.KindFiles] for the generic producer, or the kind of the producer built by
// [NewScaffold] or [NewComponent].
func (p *Producer) Kind() string {
	defer perf.Track(nil, "produce.Producer.Kind")()

	if p.profile != nil {
		return p.profile.kind
	}
	return artifact.KindFiles
}

// Prepare snapshots the source and returns a sealed PreparedArtifact. The caller owns it and must
// Close it; on any error all owned temporary data is already removed.
//
//nolint:gocritic // The request is passed by value because artifact.Producer fixes the signature.
func (p *Producer) Prepare(ctx context.Context, request artifact.PrepareRequest) (*artifact.PreparedArtifact, error) {
	defer perf.Track(nil, "produce.Producer.Prepare")()

	if err := validatePrepareRequest(&request, p.Kind()); err != nil {
		return nil, err
	}
	meta, err := p.producerMetadata(ctx, request.Source)
	if err != nil {
		return nil, err
	}
	name, version, err := ResolveIdentity(request.Name, request.Version, &meta, request.Source)
	if err != nil {
		return nil, err
	}
	if err := artifact.ValidateLabels(mergeLabels(meta.Labels, request.Metadata.Labels)); err != nil {
		return nil, err
	}
	snap, err := snapshot.Capture(ctx, request.Source, p.captureOptions()...)
	if err != nil {
		return nil, err
	}
	prepared, err := p.prepareFrom(ctx, snap, &request, &identity{name: name, version: version, meta: meta})
	if err != nil {
		_ = snap.Close()
		return nil, err
	}
	prepared.Exclusions = snap.Exclusions
	return prepared, nil
}

func (p *Producer) captureOptions() []snapshot.Option {
	opts := []snapshot.Option{snapshot.WithStagingRoot(p.stagingRoot), snapshot.WithLimits(p.limits)}
	if p.profile != nil && len(p.profile.rules) > 0 {
		opts = append(opts, snapshot.WithExcluder(excluderFor(p.profile.rules)))
	}
	return append(opts, p.snapshotOps...)
}

// producerMetadata returns the kind's own validated metadata, overlaid by an injected metadata
// source (whose non-empty values win). The generic producer has only the injected source.
func (p *Producer) producerMetadata(ctx context.Context, source string) (ProducerMetadata, error) {
	var base ProducerMetadata
	if p.profile != nil && p.profile.inspect != nil {
		inspected, err := p.profile.inspect(ctx, source)
		if err != nil {
			return ProducerMetadata{}, err
		}
		base = inspected
	}
	if p.metadata == nil {
		return base, nil
	}
	injected, err := p.metadata(ctx, source)
	if err != nil {
		return ProducerMetadata{}, err
	}
	return overlayMetadata(&base, &injected), nil
}

// identity is the resolved name, version, and producer metadata of one preparation.
type identity struct {
	name    string
	version string
	meta    ProducerMetadata
}

func (p *Producer) prepareFrom(ctx context.Context, snap *snapshot.Snapshot, request *artifact.PrepareRequest, id *identity) (*artifact.PreparedArtifact, error) {
	if err := checkReserved(snap.Entries); err != nil {
		return nil, err
	}
	docs, err := collectDocuments(snap, request.Source, documentOverrides{
		artifact.DocumentReadme:    request.Readme,
		artifact.DocumentChangelog: request.Changelog,
	})
	if err != nil {
		return nil, err
	}
	record := artifact.Record{
		Name:      id.name,
		Version:   id.version,
		Kind:      p.Kind(),
		CreatedAt: artifact.NewTimestamp(request.PreparedAt),
		Metadata: artifact.Metadata{
			Description: firstNonEmpty(request.Metadata.Description, id.meta.Description),
			Labels:      mergeLabels(id.meta.Labels, request.Metadata.Labels),
			Provenance:  request.Metadata.Provenance,
		},
		Inventory: payloadInventory(snap.Entries),
		Documents: documentDescriptors(docs),
		KindData:  id.meta.KindData,
	}
	if err := record.Seal(); err != nil {
		return nil, err
	}
	return finalize(ctx, &finalizeInput{
		record: &record, docs: docs, open: snap.Open, stagingDir: snap.Root, packaging: request.Packaging,
		operationID: request.OperationID, preparedAt: request.PreparedAt, cleanup: snap.Close,
	})
}

// finalizeInput is everything needed to stage blobs and build the PreparedArtifact.
type finalizeInput struct {
	record      *artifact.Record
	docs        []stagedDocument
	open        opener
	stagingDir  string
	packaging   artifact.Packaging
	operationID string
	preparedAt  time.Time
	cleanup     func() error
}

// finalize canonicalizes the Record, stages the blobs, and builds the PreparedArtifact.
func finalize(ctx context.Context, in *finalizeInput) (*artifact.PreparedArtifact, error) {
	recordBytes, err := in.record.Canonical()
	if err != nil {
		return nil, err
	}
	intent, err := in.record.IntentDigest()
	if err != nil {
		return nil, err
	}
	assembled := assembly{record: in.record, recordBytes: recordBytes, docs: in.docs, open: in.open, stagingDir: in.stagingDir}
	blobs, err := assembled.blobs(ctx, in.packaging)
	if err != nil {
		return nil, err
	}
	return &artifact.PreparedArtifact{
		Record:       *in.record,
		IntentDigest: intent,
		OperationID:  in.operationID,
		PreparedAt:   in.preparedAt,
		Packaging:    in.packaging,
		Blobs:        blobs,
		Cleanup:      in.cleanup,
	}, nil
}

func documentDescriptors(docs []stagedDocument) []artifact.Document {
	descriptors := make([]artifact.Document, 0, len(docs))
	for index := range docs {
		descriptors = append(descriptors, docs[index].descriptor)
	}
	return descriptors
}

// validatePrepareRequest checks the fields the service is required to supply.
func validatePrepareRequest(request *artifact.PrepareRequest, kind string) error {
	if request.Kind != "" && request.Kind != kind {
		return fmt.Errorf("%w: the %s producer cannot prepare kind %q", errUtils.ErrArtifactConfigInvalid, kind, request.Kind)
	}
	if request.OperationID == "" || request.PreparedAt.IsZero() {
		return fmt.Errorf("%w: the operation ID and preparation time are captured once by the service before preparing", errUtils.ErrArtifactConfigInvalid)
	}
	switch request.Packaging {
	case artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		return nil
	default:
		return fmt.Errorf("%w: unsupported packaging %q", errUtils.ErrArtifactPackagingInvalid, request.Packaging)
	}
}
