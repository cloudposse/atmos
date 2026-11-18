package s3

import (
	"context"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Register registers the aws/s3 repository kind in the process-wide registry. It is called
// explicitly by a composition root, never from init. Options such as [WithVerifier] apply to every
// repository the registered factory constructs. It does not touch the legacy CI artifact registry,
// whose own "aws/s3" store is a different thing.
func Register(opts ...Option) error {
	defer perf.Track(nil, "s3.Register")()

	return RegisterWith(artifact.DefaultRepositoryRegistry(), opts...)
}

// RegisterWith registers the aws/s3 kind in the given registry. A kind that is already registered
// is an error, so double registration is caught instead of silently replaced.
func RegisterWith(registry *artifact.RepositoryRegistry, opts ...Option) error {
	defer perf.Track(nil, "s3.RegisterWith")()

	desc := Descriptor(opts...)
	return registry.Register(&desc, factory(opts))
}

// Descriptor returns the static descriptor of the aws/s3 kind: the built-in descriptor refined with
// the capabilities the registered backend provides. It needs no I/O.
func Descriptor(opts ...Option) artifact.KindDescriptor {
	defer perf.Track(nil, "s3.Descriptor")()

	desc, _ := artifact.BuiltinKind(artifact.RepositoryKindS3)
	applied := applySettings(opts)
	conditional := applied.store == nil || applied.store.Capabilities().ConditionalPut
	desc.Capabilities = staticCapabilities(conditional, applied.verifier)
	return *desc
}

func factory(opts []Option) artifact.RepositoryFactory {
	return func(_ context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, ropts ...artifact.RepositoryOption) (artifact.Repository, error) {
		applied := artifact.ApplyRepositoryOptions(ropts...)
		repo, err := newRepository(alias, cfg, opts, &applied)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}
}

// Kinds lists the repository kinds this package registers.
func Kinds() []string {
	defer perf.Track(nil, "s3.Kinds")()

	return []string{artifact.RepositoryKindS3}
}
