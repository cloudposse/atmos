//nolint:gocritic,lintroller // Descriptor-by-value parameters and thin store delegators mirror the ORAS content contracts so stores stay interchangeable.
package oci

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ReadOnlyStore is the source side of a graph copy.
type ReadOnlyStore interface {
	// Fetch streams the content identified by the descriptor.
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
	// Exists reports whether the content is present.
	Exists(ctx context.Context, target ocispec.Descriptor) (bool, error)
	// Resolve resolves a tag or digest reference to a descriptor.
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)
}

// ReadOnlyGraphStore is a source that can also report predecessors (referrers).
type ReadOnlyGraphStore interface {
	ReadOnlyStore
	// Predecessors returns manifests whose subject or children include the node.
	Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error)
}

// Source names the content a copy reads: a store and a tag or digest reference within it.
type Source struct {
	Store     ReadOnlyStore
	Reference string
}

// GraphSource is a Source that can also report predecessors, for extended copies.
type GraphSource struct {
	Store     ReadOnlyGraphStore
	Reference string
}

// Destination names where a copy writes: a store and the reference the root is published under.
// An empty Reference reuses the source reference.
type Destination struct {
	Store     Store
	Reference string
}

const defaultCopyConcurrency = 3

type copyConfig struct {
	concurrency int
	onCopied    func(ocispec.Descriptor)
	noSubjects  bool
}

// CopyOption configures a graph copy.
type CopyOption func(*copyConfig)

// WithCopyConcurrency bounds how many nodes transfer at once. Values below one use the default.
//
//nolint:lintroller // Functional option only.
func WithCopyConcurrency(concurrency int) CopyOption {
	return func(config *copyConfig) { config.concurrency = concurrency }
}

// WithCopyObserver reports every node that was actually transferred (not skipped as present).
//
//nolint:lintroller // Functional option only.
func WithCopyObserver(observe func(ocispec.Descriptor)) CopyOption {
	return func(config *copyConfig) { config.onCopied = observe }
}

// WithoutSubjectTraversal stops the copy at the selected manifest instead of also copying the
// manifest named by its subject field. Legacy writers (for example signature attachments)
// publish an attachment next to a subject that already exists at the destination.
//
//nolint:lintroller // Functional option only.
func WithoutSubjectTraversal() CopyOption {
	return func(config *copyConfig) { config.noSubjects = true }
}

func newCopyConfig(options []CopyOption) copyConfig {
	config := copyConfig{concurrency: defaultCopyConcurrency}
	for _, option := range options {
		option(&config)
	}
	if config.concurrency < 1 {
		config.concurrency = defaultCopyConcurrency
	}
	return config
}

func (config copyConfig) graphOptions() oras.CopyGraphOptions {
	options := oras.CopyGraphOptions{Concurrency: config.concurrency}
	if config.noSubjects {
		options.FindSuccessors = successorsWithoutSubject
	}
	if config.onCopied != nil {
		options.PostCopy = func(_ context.Context, desc ocispec.Descriptor) error {
			config.onCopied(desc)
			return nil
		}
	}
	return options
}

// successorsWithoutSubject is the default ORAS successor discovery minus the subject edge,
// which ORAS lists first for OCI image manifests and indexes.
func successorsWithoutSubject(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	successors, err := content.Successors(ctx, fetcher, desc)
	hasSubject := desc.MediaType == ocispec.MediaTypeImageManifest || desc.MediaType == ocispec.MediaTypeImageIndex
	if err != nil || !hasSubject || len(successors) == 0 {
		return successors, err
	}
	raw, err := content.FetchAll(ctx, fetcher, desc)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Subject *ocispec.Descriptor `json:"subject"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	if manifest.Subject != nil && successors[0].Digest == manifest.Subject.Digest {
		return successors[1:], nil
	}
	return successors, nil
}

// Copy transfers the graph below the source reference (a manifest, image, or index with every
// child) into the destination under its reference (the source reference when empty). Raw manifest and index bytes are streamed
// unchanged and every node is digest-verified by the receiving store, so identity
// survives registry, layout, and registry hops. Cancelling ctx stops traversal.
//
// If a write fails with an unknown outcome the destination is resolved before deciding:
// when the destination reference already addresses the source root the copy is complete. There are no blind
// mutation retries, and rejected credentials are never retried anonymously.
func Copy(ctx context.Context, src Source, dst Destination, options ...CopyOption) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.Copy")()

	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}
	if dst.Reference == "" {
		dst.Reference = src.Reference
	}
	config := newCopyConfig(options)
	root, err := oras.Copy(ctx, guardedSource{ReadOnlyStore: src.Store}, src.Reference, guardedStore{Store: dst.Store}, dst.Reference,
		oras.CopyOptions{CopyGraphOptions: config.graphOptions()})
	if err != nil {
		return reconcile(ctx, err, src.Store, src.Reference, dst)
	}
	return root, nil
}

// ExtendedCopy is Copy plus every predecessor of the graph (signatures, SBOMs, and other
// referrers), discovered through the source's referrers API or tag-schema fallback.
func ExtendedCopy(ctx context.Context, src GraphSource, dst Destination, options ...CopyOption) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.ExtendedCopy")()

	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}
	if dst.Reference == "" {
		dst.Reference = src.Reference
	}
	config := newCopyConfig(options)
	graph := guardedGraphSource{guardedSource: guardedSource{ReadOnlyStore: src.Store}, graph: src.Store}
	root, err := oras.ExtendedCopy(ctx, graph, src.Reference, guardedStore{Store: dst.Store}, dst.Reference,
		oras.ExtendedCopyOptions{ExtendedCopyGraphOptions: oras.ExtendedCopyGraphOptions{CopyGraphOptions: config.graphOptions()}})
	if err != nil {
		return reconcile(ctx, err, src.Store, src.Reference, dst)
	}
	return root, nil
}

// reconcile decides the outcome of a failed copy by looking at the destination.
func reconcile(ctx context.Context, cause error, src ReadOnlyStore, srcRef string, dst Destination) (ocispec.Descriptor, error) {
	failure := classifyCopyError(cause)
	if !ambiguousWrite(ctx, failure) {
		return ocispec.Descriptor{}, failure
	}
	want, err := src.Resolve(ctx, srcRef)
	if err != nil {
		return ocispec.Descriptor{}, failure
	}
	have, err := dst.Store.Resolve(ctx, dst.Reference)
	if err != nil || have.Digest != want.Digest {
		return ocispec.Descriptor{}, failure
	}
	return want, nil
}

func classifyCopyError(err error) error {
	if errors.Is(err, errUtils.ErrOCIRegistry) || errors.Is(err, errUtils.ErrOCILayout) {
		return err
	}
	return wrapRegistry(err)
}

// guardedSource aborts as soon as the context ends, even for stores that ignore it.
type guardedSource struct{ ReadOnlyStore }

func (g guardedSource) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return g.ReadOnlyStore.Fetch(ctx, target)
}

func (g guardedSource) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return g.ReadOnlyStore.Exists(ctx, target)
}

type guardedGraphSource struct {
	guardedSource
	graph ReadOnlyGraphStore
}

func (g guardedGraphSource) Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return g.graph.Predecessors(ctx, node)
}

// guardedStore does the same for destinations, so cancellation never starts another write.
type guardedStore struct{ Store }

func (g guardedStore) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return g.Store.Exists(ctx, target)
}

func (g guardedStore) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := g.Store.Push(ctx, expected, content)
	if err == nil || errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	// The write may have landed even though it was reported as failed. Look at the
	// destination before deciding; never write again.
	if ambiguousWrite(ctx, err) {
		if exists, existsErr := g.Store.Exists(ctx, expected); existsErr == nil && exists {
			return nil
		}
	}
	return err
}

// PushReference forwards to the wrapped store when it can push a manifest under a reference,
// which lets registry destinations write the root manifest directly under its tag.
func (g guardedStore) PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := g.pushReference(ctx, expected, content, reference)
	if err == nil || errors.Is(err, errdef.ErrAlreadyExists) || !ambiguousWrite(ctx, err) {
		return err
	}
	if resolved, resolveErr := g.Resolve(ctx, reference); resolveErr == nil && resolved.Digest == expected.Digest {
		return nil
	}
	return err
}

func (g guardedStore) pushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	if pusher, ok := g.Store.(ReferencePusher); ok {
		return pusher.PushReference(ctx, expected, content, reference)
	}
	if err := g.Store.Push(ctx, expected, content); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return g.Tag(ctx, expected, reference)
}
