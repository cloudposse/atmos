package artifacts

import (
	"context"
	"sort"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	pluginsign "github.com/cloudposse/atmos/pkg/plugin/sign"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ServiceOptions are the per-invocation settings that reach the composition root.
type ServiceOptions struct {
	// AcceptTransparencyLog accepts Sigstore transparency log publication without prompting.
	AcceptTransparencyLog bool
}

// ServiceFactory builds the artifact service for the loaded configuration. Tests replace
// newService to inject a fake service; production composes the real repositories lazily, so
// building the service performs no network access, credential resolution, or directory creation.
type ServiceFactory func(config *schema.AtmosConfiguration, options ServiceOptions) (artifact.Service, error)

var newService ServiceFactory = productionService

// UseServiceFactory replaces the service factory and returns a function that restores the previous
// one. It is the seam command tests use to run the real commands against injected repositories,
// verifiers, and signers; production code never calls it.
func UseServiceFactory(factory ServiceFactory) (restore func()) {
	previous := newService
	newService = factory
	return func() { newService = previous }
}

// productionService wires the repositories, the shared verifier, the managed Cosign signer, and the
// plugin-aware registry authentication preparation into one artifact service.
func productionService(config *schema.AtmosConfiguration, options ServiceOptions) (artifact.Service, error) {
	composition, err := compose.New(
		config,
		compose.WithBlobSigner(pluginsign.Cosign{Config: config, Yes: options.AcceptTransparencyLog}),
		compose.WithRegistryPreparation(func(ctx context.Context) (func(), error) {
			return bootstrap.PrepareAvailable(ctx, config)
		}),
	)
	if err != nil {
		return nil, err
	}
	return composition.Service, nil
}

// configuredAliases lists the repository aliases in the loaded configuration, sorted.
func configuredAliases(config *schema.AtmosConfiguration) []string {
	if config == nil {
		return nil
	}
	aliases := make([]string, 0, len(config.Artifacts.Repositories))
	for alias := range config.Artifacts.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
