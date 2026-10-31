package oci

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Register registers the oci/registry and oci/layout repository kinds in the process-wide
// registry. A composition root calls it explicitly; nothing registers from init. Options such as
// [WithTransport] and [WithVerifier] apply to every repository the registered factories build.
func Register(opts ...Option) error {
	defer perf.Track(nil, "oci.Register")()

	return RegisterWith(artifact.DefaultRepositoryRegistry(), opts...)
}

// RegisterWith registers the OCI kinds in the given registry. A kind that is already registered
// is an error, so double registration is caught instead of silently replaced.
func RegisterWith(registry *artifact.RepositoryRegistry, opts ...Option) error {
	defer perf.Track(nil, "oci.RegisterWith")()

	for _, kind := range Kinds() {
		desc := Descriptor(kind, opts...)
		if err := registry.Register(&desc, factoryFor(kind, opts)); err != nil {
			return err
		}
	}
	return nil
}

// Kinds lists the repository kinds this package registers.
func Kinds() []string {
	defer perf.Track(nil, "oci.Kinds")()

	return []string{artifact.RepositoryKindOCIRegistry, artifact.RepositoryKindOCILayout}
}

// Descriptor returns the static descriptor of an OCI kind: the built-in descriptor refined with
// the capabilities the registered backend actually provides (delete scope, enumeration, evidence
// formats of the verifier). It needs no I/O.
func Descriptor(kind string, opts ...Option) artifact.KindDescriptor {
	defer perf.Track(nil, "oci.Descriptor")()

	desc, _ := artifact.BuiltinKind(kind)
	applied := applySettings(opts)
	enumerates := kind == artifact.RepositoryKindOCILayout
	if kind == artifact.RepositoryKindOCIRegistry {
		enumerates = !applied.noCatalog && transportEnumerates(&applied)
	}
	desc.Capabilities = staticCapabilities(kind, applied.verifier, enumerates)
	return *desc
}

// transportEnumerates reports whether the configured transport can list registry repositories.
// A factory builds its transport later, so it is assumed capable; listing reports the truth.
func transportEnumerates(applied *settings) bool {
	if applied.factory != nil || applied.transport == nil {
		return true
	}
	_, ok := applied.transport.(atmosoci.RegistryCatalog)
	return ok
}

func factoryFor(kind string, opts []Option) artifact.RepositoryFactory {
	return func(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, ropts ...artifact.RepositoryOption) (artifact.Repository, error) {
		return newRepository(ctx, &construction{alias: alias, kind: kind, cfg: cfg, applied: applySettings(opts), ropts: artifact.ApplyRepositoryOptions(ropts...)})
	}
}

// NewRepository builds one repository from its configuration without going through a registry.
func NewRepository(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, opts ...Option) (*Repository, error) {
	defer perf.Track(nil, "oci.NewRepository")()

	if cfg == nil {
		return nil, fmt.Errorf("%w: repository %q has no configuration", errUtils.ErrArtifactConfigInvalid, alias)
	}
	return newRepository(ctx, &construction{alias: alias, kind: cfg.Kind, cfg: cfg, applied: applySettings(opts)})
}

// construction is everything needed to build one repository.
type construction struct {
	alias   string
	kind    string
	cfg     *schema.ArtifactRepositoryConfig
	applied settings
	ropts   artifact.RepositoryOptions
}

func newRepository(ctx context.Context, build *construction) (*Repository, error) {
	alias, kind, cfg, applied, ropts := build.alias, build.kind, build.cfg, build.applied, build.ropts
	if cfg.Kind != "" && cfg.Kind != kind {
		return nil, fmt.Errorf("%w: configuration kind %q cannot construct a %q repository", errUtils.ErrArtifactConfigInvalid, cfg.Kind, kind)
	}
	var be backend
	var err error
	switch kind {
	case artifact.RepositoryKindOCIRegistry:
		be, err = registryBackendFor(ctx, alias, cfg, &applied, ropts)
	case artifact.RepositoryKindOCILayout:
		be, err = layoutBackendFor(cfg)
	default:
		err = fmt.Errorf("%w: %q is not an OCI repository kind", errUtils.ErrArtifactConfigInvalid, kind)
	}
	if err != nil {
		return nil, err
	}
	delegate := ""
	if cfg.Locking != nil {
		delegate = cfg.Locking.Repository
	}
	return newRepositoryCore(alias, be, applied, delegate, ropts.LookupRepository), nil
}

func registryBackendFor(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, applied *settings, ropts artifact.RepositoryOptions) (backend, error) {
	var options artifact.OCIRegistryOptions
	if err := artifact.DecodeOptions(cfg.Options, &options); err != nil {
		return nil, err
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}
	transport, err := transportFor(ctx, alias, cfg, applied, ropts)
	if err != nil {
		return nil, err
	}
	return newRegistryBackend(options.Registry, options.Prefix, transport, applied.noCatalog), nil
}

// transportFor picks the transport: the identity-binding factory, the injected transport, or the
// default client. A configured identity without a factory is an error, so it is never ignored.
func transportFor(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, applied *settings, ropts artifact.RepositoryOptions) (atmosoci.Transport, error) {
	switch {
	case applied.factory != nil:
		transport, err := applied.factory(ctx, alias, cfg, ropts.ResolveIdentity)
		if err != nil {
			return nil, fmt.Errorf("repository %q transport: %w", alias, err)
		}
		return transport, nil
	case cfg.Identity != "":
		return nil, fmt.Errorf("%w: repository %q binds identity %q but no transport factory was registered to apply it; registering the OCI kinds needs WithTransportFactory", errUtils.ErrArtifactConfigInvalid, alias, cfg.Identity)
	case applied.transport != nil:
		return applied.transport, nil
	default:
		return atmosoci.NewClient(nil), nil
	}
}

func layoutBackendFor(cfg *schema.ArtifactRepositoryConfig) (backend, error) {
	var options artifact.OCILayoutOptions
	if err := artifact.DecodeOptions(cfg.Options, &options); err != nil {
		return nil, err
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(options.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: layout path %q: %w", errUtils.ErrArtifactConfigInvalid, options.Path, err)
	}
	return newLayoutBackend(root), nil
}
