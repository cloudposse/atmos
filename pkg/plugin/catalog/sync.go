package catalog

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// ReleaseSelection identifies a published package version to reconcile into a catalog.
type ReleaseSelection struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
}

// PublicVerifier verifies both package settings and anonymously readable OCI bytes.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_registry_sync_test.go -package=catalog github.com/cloudposse/atmos/pkg/oci Registry
//go:generate go run go.uber.org/mock/mockgen -source=sync.go -destination=mock_sync_test.go -package=catalog
type PublicVerifier interface {
	RequirePublic(context.Context, string) error
	Verify(context.Context, string, string, string) (string, error)
}

type syncOptions struct {
	skipMissing bool
	public      PublicVerifier
}

// SyncOption configures catalog reconciliation.
type SyncOption func(*syncOptions)

// WithSkipMissing permits only registry not-found releases to remain absent.
//
//nolint:lintroller // Functional option.
func WithSkipMissing() SyncOption { return func(o *syncOptions) { o.skipMissing = true } }

// WithPublicVerification enforces public visibility before writing and verifies published bytes afterward.
//
//nolint:lintroller // Functional option.
func WithPublicVerification(verifier PublicVerifier) SyncOption {
	return func(o *syncOptions) { o.public = verifier }
}

// Syncer reconciles releases using an injected registry and extension registry.
type Syncer struct {
	registry    atmosoci.Registry
	descriptors *extension.Registry
}

// NewSyncer constructs a catalog reconciler.
//
//nolint:lintroller // Constructor only.
func NewSyncer(registry atmosoci.Registry, descriptors *extension.Registry) *Syncer {
	return &Syncer{registry: registry, descriptors: descriptors}
}

// Sync merges verified releases into the current catalog and publishes once.
// Historical versions and existing yank metadata are retained. Callers must serialize publishers.
func (s *Syncer) Sync(ctx context.Context, target string, releases []ReleaseSelection, options ...SyncOption) error {
	defer perf.Track(nil, "plugincatalog.Sync")()
	settings := syncOptions{}
	for _, option := range options {
		option(&settings)
	}
	if len(releases) == 0 {
		return fmt.Errorf("%w: no releases selected for catalog reconciliation", errUtils.ErrPluginInvalidConfig)
	}
	targetRef, err := ParseReleaseReference(target)
	if err != nil {
		return err
	}
	if settings.public != nil {
		if err := settings.public.RequirePublic(ctx, targetRef.Context().Name()); err != nil {
			return err
		}
	}
	file, digest, err := Read(ctx, s.registry, target)
	if atmosoci.IsNotFound(err) {
		file = New("plugins")
	} else if err != nil {
		return err
	}
	if err := s.mergeReleases(ctx, file, releases, settings); err != nil {
		return err
	}
	if err := Publish(ctx, s.registry, target, file, digest); err != nil {
		return err
	}
	return verifyCatalogPublication(ctx, s.registry, targetRef, settings.public)
}

func (s *Syncer) mergeReleases(ctx context.Context, file *File, releases []ReleaseSelection, settings syncOptions) error {
	added := 0
	for _, selected := range releases {
		err := s.syncRelease(ctx, file, selected, settings.public)
		if err != nil {
			var missing *unavailableRelease
			if settings.skipMissing && errors.As(err, &missing) {
				continue
			}
			return err
		}
		added++
	}
	if added == 0 {
		return fmt.Errorf("%w: no selected releases are available", errUtils.ErrPluginVersionNotFound)
	}
	return nil
}

func verifyCatalogPublication(ctx context.Context, registry atmosoci.Registry, targetRef name.Reference, public PublicVerifier) error {
	if public != nil {
		published, err := registry.Head(ctx, targetRef)
		if err != nil {
			return err
		}
		_, err = public.Verify(ctx, targetRef.Context().Name(), targetRef.Identifier(), published.Digest.String())
		return err
	}
	return nil
}

func (s *Syncer) syncRelease(ctx context.Context, file *File, selected ReleaseSelection, public PublicVerifier) error {
	repository, err := pluginoci.ParseDestination(selected.Repository)
	if err != nil {
		return err
	}
	if selected.Version == "" {
		return errUtils.ErrPluginInvalidConfig
	}
	reference, err := ParseReleaseReference(repository.Name() + ":" + selected.Version)
	if err != nil {
		return err
	}
	descriptor, err := fetchRelease(ctx, s.registry, selected, reference, public)
	if err != nil {
		return err
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return err
	}
	release, err := pluginoci.Inspect(index, descriptor.Digest.String())
	if err != nil {
		return err
	}
	if NormalizeVersion(release.Version) != NormalizeVersion(selected.Version) || path.Base(release.Name) != selected.Name {
		return errUtils.ErrPluginInvalidConfig
	}
	return Add(file, selected.Repository, release, s.descriptors)
}

func fetchRelease(ctx context.Context, registry atmosoci.Registry, selected ReleaseSelection, reference name.Reference, public PublicVerifier) (*remote.Descriptor, error) {
	if public != nil {
		if err := public.RequirePublic(ctx, selected.Repository); err != nil {
			return nil, err
		}
	}
	descriptor, err := registry.Get(ctx, reference)
	if err != nil {
		if atmosoci.IsNotFound(err) {
			return nil, &unavailableRelease{cause: err}
		}
		return nil, err
	}
	if public != nil {
		if _, err := public.Verify(ctx, selected.Repository, selected.Version, descriptor.Digest.String()); err != nil {
			return nil, err
		}
	}
	return descriptor, nil
}

// unavailableRelease distinguishes a missing release tag from missing blobs in a corrupt release.
type unavailableRelease struct{ cause error }

//nolint:lintroller // Trivial error accessor.
func (e *unavailableRelease) Error() string { return e.cause.Error() }

//nolint:lintroller // Trivial error accessor.
func (e *unavailableRelease) Unwrap() error { return e.cause }
