package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	artifactoci "github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/keyring"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewTransportFactory returns the default OCI registry transport factory. It builds the same
// client the plugin installer uses: Docker and GHCR credentials with the shared Atmos keyring as a
// fallback, and, when the repository configures an `identity`, that Atmos auth identity bound to
// the repository's exact registry host. A configured identity never falls through to ambient
// credentials, and its credentials are never presented to another registry. Constructing the
// client authenticates nothing; the identity authenticates on the first registry request. The
// preparation, when non-nil, lets an identity provided by an installed auth plugin authenticate.
func NewTransportFactory(config *schema.AtmosConfiguration, preparation registryauth.Preparation) artifactoci.TransportFactory {
	defer perf.Track(config, "compose.NewTransportFactory")()

	return func(_ context.Context, alias string, repo *schema.ArtifactRepositoryConfig, _ artifact.IdentityResolver) (atmosoci.Transport, error) {
		clientOptions := []atmosoci.ClientOption{atmosoci.WithCredentialFallback(credentialChain(config))}
		if repo.Identity != "" {
			resolver, err := identityBinding(config, alias, repo, preparation)
			if err != nil {
				return nil, err
			}
			clientOptions = append(clientOptions, atmosoci.WithAuthenticationResolver(resolver))
		}
		return atmosoci.NewClient(config, clientOptions...), nil
	}
}

// credentialChain is the shared Atmos keyring fallback, optional on a system keyring so a missing
// one never blocks anonymous or Docker-authenticated access.
func credentialChain(config *schema.AtmosConfiguration) atmosoci.CredentialKeychain {
	return atmosoci.CredentialKeychain{
		Open:     func() (keyring.Keyring, error) { return credentials.OpenKeyring(&config.Auth) },
		Optional: config.Auth.Keyring.Type == "" && credentials.KeyringType(&config.Auth) == "system",
	}
}

// identityBinding binds the repository's identity to its registry host. The identity must exist,
// so a typo fails when the repository opens instead of silently using other credentials.
func identityBinding(config *schema.AtmosConfiguration, alias string, repo *schema.ArtifactRepositoryConfig, preparation registryauth.Preparation) (*registryauth.Resolver, error) {
	if _, found := config.Auth.Identities[repo.Identity]; !found {
		return nil, fmt.Errorf("%w: artifact repository %q references identity %q, which is not defined under auth.identities", errUtils.ErrIdentityNotFound, alias, repo.Identity)
	}
	var options artifact.OCIRegistryOptions
	if err := artifact.DecodeOptions(repo.Options, &options); err != nil {
		return nil, err
	}
	registry, err := name.NewRegistry(strings.ToLower(options.Registry), name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("%w: artifact repository %q registry %q: %w", errUtils.ErrArtifactConfigInvalid, alias, options.Registry, err)
	}
	var resolverOptions []registryauth.Option
	if preparation != nil {
		resolverOptions = append(resolverOptions, registryauth.WithPreparation(preparation))
	}
	return registryauth.New(config, map[string]string{registry.Name(): repo.Identity}, resolverOptions...), nil
}
