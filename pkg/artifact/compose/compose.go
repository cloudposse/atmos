package compose

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/s3"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/artifact/sigstore"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Composition is the wired artifact subsystem for one configuration.
type Composition struct {
	// Service is the repository-independent artifact API.
	Service artifact.Service

	// Repositories resolves configured aliases. Describing an alias never authenticates; opening
	// one constructs it, and its identity is resolved only when an operation needs credentials.
	Repositories *service.ConfigRepositories

	// Registry holds the registered repository kinds.
	Registry *artifact.RepositoryRegistry
}

// New wires the artifact service from the `artifacts` section of the configuration. It validates
// the configuration and registers every repository kind, but performs no network access, resolves
// no credentials, and creates no directories. A nil configuration is an empty one.
//
// The same verifier instance serves every repository kind, so one trust policy governs local
// directories and archives, S3 buckets, and OCI registries and layouts alike. Verification
// policy (the global requirements and each repository's own) is combined by the service, and a
// repository can strengthen but never weaken the global policy.
func New(atmosConfig *schema.AtmosConfiguration, opts ...Option) (*Composition, error) {
	defer perf.Track(atmosConfig, "compose.New")()

	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}
	var applied settings
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	applied.defaults(atmosConfig)

	if applied.registry == nil {
		applied.registry = artifact.NewRepositoryRegistry()
	}
	if err := registerKinds(applied.registry, &applied); err != nil {
		return nil, err
	}
	cfg := atmosConfig.Artifacts
	if err := artifactconfig.NewValidator(artifactconfig.WithRegistry(applied.registry)).Validate(&cfg); err != nil {
		return nil, err
	}

	repos := newRepositories(&cfg, &applied)
	svcOptions := []service.Option{service.WithImmutabilityPolicy(applied.immutability)}
	if applied.signer != nil {
		svcOptions = append(svcOptions, service.WithSigner(applied.signer))
	}
	svc, err := service.New(repos, applied.producers, append(svcOptions, applied.service...)...)
	if err != nil {
		return nil, err
	}
	return &Composition{Service: svc, Repositories: repos, Registry: applied.registry}, nil
}

// defaults fills every dependency the caller did not inject.
func (s *settings) defaults(atmosConfig *schema.AtmosConfiguration) {
	if s.verifier == nil {
		s.verifier = sigstore.NewVerifier()
	}
	if s.signer == nil && s.blobSigner != nil {
		s.signer = sigstore.NewSigner(s.blobSigner)
	}
	if s.immutability == nil {
		s.immutability = oci.PluginImmutability()
	}
	if s.identity == nil {
		s.identity = NewIdentityResolver(atmosConfig)
	}
	if s.transport == nil {
		s.transport = NewTransportFactory(atmosConfig, s.preparation)
	}
}

// registerKinds registers the five repository kinds with the shared verifier.
func registerKinds(registry *artifact.RepositoryRegistry, applied *settings) error {
	localOpts := append([]local.Option{local.WithVerifier(applied.verifier)}, applied.local...)
	s3Opts := append([]s3.Option{s3.WithVerifier(applied.verifier)}, applied.s3...)
	ociOpts := append([]oci.Option{oci.WithVerifier(applied.verifier), oci.WithTransportFactory(applied.transport)}, applied.oci...)
	return errors.Join(
		local.RegisterWith(registry, localOpts...),
		s3.RegisterWith(registry, s3Opts...),
		oci.RegisterWith(registry, ociOpts...),
	)
}

// newRepositories builds the configuration-backed alias table. Its repository lookup lets a
// repository that delegates locking find its coordinator, which opens with its own identity.
func newRepositories(cfg *schema.ArtifactConfig, applied *settings) *service.ConfigRepositories {
	var repos *service.ConfigRepositories
	lookup := func(alias string) (artifact.Repository, error) {
		if repos == nil {
			return nil, fmt.Errorf("%w: repository %q is not available yet", errUtils.ErrArtifactConfigInvalid, alias)
		}
		return repos.Open(context.Background(), alias)
	}
	repos = service.FromConfig(
		cfg, applied.registry,
		artifact.WithIdentityResolver(applied.identity),
		artifact.WithRepositoryLookup(lookup),
	)
	return repos
}
