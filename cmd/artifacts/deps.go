package artifacts

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifactwire"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ServiceOptions are the per-invocation settings that reach the composition root.
type ServiceOptions = invoke.ServiceOptions

// ServiceFactory builds the artifact service for the loaded configuration. Tests replace
// newService to inject a fake service; production composes the real repositories lazily, so
// building the service performs no network access, credential resolution, or directory creation.
type ServiceFactory = invoke.ServiceFactory

var newService ServiceFactory = artifactwire.NewService

// UseServiceFactory replaces the service factory and returns a function that restores the previous
// one. It is the seam command tests use to run the real commands against injected repositories,
// verifiers, and signers; production code never calls it.
func UseServiceFactory(factory ServiceFactory) (restore func()) {
	previous := newService
	newService = factory
	return func() { newService = previous }
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
