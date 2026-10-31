package local

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Register registers the local/dir and local/archive repository kinds in the process-wide
// registry. It is called explicitly by a composition root, never from init. Options such as
// [WithVerifier] apply to every repository the registered factories construct. It does not touch
// the legacy CI artifact registry, whose own "local/dir" store is a different thing.
func Register(opts ...Option) error {
	defer perf.Track(nil, "local.Register")()

	return RegisterWith(artifact.DefaultRepositoryRegistry(), opts...)
}

// RegisterWith registers the local kinds in the given registry. A kind that is already
// registered is an error, so double registration is caught instead of silently replaced.
func RegisterWith(registry *artifact.RepositoryRegistry, opts ...Option) error {
	defer perf.Track(nil, "local.RegisterWith")()

	for _, kind := range []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive} {
		desc := Descriptor(kind, opts...)
		if err := registry.Register(&desc, factoryFor(kind, opts)); err != nil {
			return err
		}
	}
	return nil
}

// Descriptor returns the static descriptor of a local kind: the built-in descriptor refined with
// the capabilities the registered backend actually provides (deletion, enumeration, conditional
// updates, detached evidence for the verifier's formats). It needs no I/O.
func Descriptor(kind string, opts ...Option) artifact.KindDescriptor {
	defer perf.Track(nil, "local.Descriptor")()

	desc, _ := artifact.BuiltinKind(kind)
	applied := applySettings(opts)
	desc.Capabilities = staticCapabilities(kind, hostEnforcesModes(), applied.verifier)
	return *desc
}

func factoryFor(kind string, opts []Option) artifact.RepositoryFactory {
	return func(_ context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, _ ...artifact.RepositoryOption) (artifact.Repository, error) {
		repo, err := newRepository(alias, kind, cfg, opts)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}
}

// Kinds lists the repository kinds this package registers.
func Kinds() []string {
	defer perf.Track(nil, "local.Kinds")()

	return []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive}
}

// ensureKind rejects a configuration that names another kind.
func ensureKind(want string, cfg *schema.ArtifactRepositoryConfig) error {
	if cfg.Kind != "" && cfg.Kind != want {
		return fmt.Errorf("%w: configuration kind %q cannot construct a %q repository", errUtils.ErrArtifactConfigInvalid, cfg.Kind, want)
	}
	return nil
}
