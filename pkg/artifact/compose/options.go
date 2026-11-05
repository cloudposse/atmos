package compose

import (
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/s3"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signature"
)

// settings holds the injectable dependencies of a composition.
type settings struct {
	verifier     artifact.Verifier
	signer       artifact.Signer
	blobSigner   signature.BlobSigner
	immutability service.ImmutabilityPolicy
	identity     artifact.IdentityResolver
	transport    oci.TransportFactory
	preparation  registryauth.Preparation
	registry     *artifact.RepositoryRegistry
	service      []service.Option
	local        []local.Option
	s3           []s3.Option
	oci          []oci.Option
	producers    *artifact.ProducerRegistry
}

// Option customizes a composition.
type Option func(*settings)

// WithVerifier replaces the signature verifier shared by every repository kind. The default
// verifies Sigstore bundles and public-key bundles against the configured trust policy.
func WithVerifier(verifier artifact.Verifier) Option {
	defer perf.Track(nil, "compose.WithVerifier")()

	return func(s *settings) { s.verifier = verifier }
}

// WithSigner replaces the signer used by sign. It takes precedence over [WithBlobSigner].
func WithSigner(signer artifact.Signer) Option {
	defer perf.Track(nil, "compose.WithSigner")()

	return func(s *settings) { s.signer = signer }
}

// WithBlobSigner supplies the Cosign-style blob signer behind the Sigstore signer, typically the
// managed Cosign authoring path. Without a signer, sign reports an unsupported capability.
func WithBlobSigner(blob signature.BlobSigner) Option {
	defer perf.Track(nil, "compose.WithBlobSigner")()

	return func(s *settings) { s.blobSigner = blob }
}

// WithImmutabilityPolicy replaces the immutable-format admission policy. The default protects
// plugin releases from any generic overwrite or delete.
func WithImmutabilityPolicy(policy service.ImmutabilityPolicy) Option {
	defer perf.Track(nil, "compose.WithImmutabilityPolicy")()

	return func(s *settings) { s.immutability = policy }
}

// WithIdentityResolver replaces the lazy identity resolver handed to every backend. The default
// resolves an Atmos auth identity into the credential type its cloud backend expects.
func WithIdentityResolver(resolver artifact.IdentityResolver) Option {
	defer perf.Track(nil, "compose.WithIdentityResolver")()

	return func(s *settings) { s.identity = resolver }
}

// WithTransportFactory replaces the per-repository OCI registry transport factory. The default
// binds a repository's identity to its registry host through the shared registry authentication.
func WithTransportFactory(factory oci.TransportFactory) Option {
	defer perf.Track(nil, "compose.WithTransportFactory")()

	return func(s *settings) { s.transport = factory }
}

// WithRegistryPreparation supplies the plugin-aware preparation the default registry
// authentication runs when an identity needs an installed auth plugin.
func WithRegistryPreparation(preparation registryauth.Preparation) Option {
	defer perf.Track(nil, "compose.WithRegistryPreparation")()

	return func(s *settings) { s.preparation = preparation }
}

// WithRegistry registers the repository kinds in the given registry instead of a new private one.
// A kind that is already registered is an error.
func WithRegistry(registry *artifact.RepositoryRegistry) Option {
	defer perf.Track(nil, "compose.WithRegistry")()

	return func(s *settings) { s.registry = registry }
}

// WithServiceOptions passes options to the artifact service, for example a clock or retry policy.
func WithServiceOptions(opts ...service.Option) Option {
	defer perf.Track(nil, "compose.WithServiceOptions")()

	return func(s *settings) { s.service = append(s.service, opts...) }
}

// WithLocalOptions passes options to the local/dir and local/archive kinds.
func WithLocalOptions(opts ...local.Option) Option {
	defer perf.Track(nil, "compose.WithLocalOptions")()

	return func(s *settings) { s.local = append(s.local, opts...) }
}

// WithS3Options passes options to the aws/s3 kind, for example an injected object store.
func WithS3Options(opts ...s3.Option) Option {
	defer perf.Track(nil, "compose.WithS3Options")()

	return func(s *settings) { s.s3 = append(s.s3, opts...) }
}

// WithOCIOptions passes options to the oci/registry and oci/layout kinds, for example an injected
// transport.
func WithOCIOptions(opts ...oci.Option) Option {
	defer perf.Track(nil, "compose.WithOCIOptions")()

	return func(s *settings) { s.oci = append(s.oci, opts...) }
}

// WithProducers supplies the producer registry used to prepare artifacts. The default has only the
// generic files producer.
func WithProducers(producers *artifact.ProducerRegistry) Option {
	defer perf.Track(nil, "compose.WithProducers")()

	return func(s *settings) { s.producers = producers }
}
