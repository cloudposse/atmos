package secrets

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxDependencyDepth = 128

// Validate the entire dependency chain before locking or authenticating any store.
// This catches both direct bootstrap cycles and cycles through other secret providers.
func validateGraph(config *schema.AtmosConfiguration, node string, active map[string]bool) error {
	return walkDependencies(config, node, active, make(map[string]bool))
}

func walkDependencies(config *schema.AtmosConfiguration, node string, active, complete map[string]bool) error {
	if active[node] || len(active) > maxDependencyDepth {
		return errUtils.ErrInvalidAuthConfig
	}
	if complete[node] {
		return nil
	}
	active[node] = true
	defer delete(active, node)
	next, err := dependencies(config, node)
	if err != nil {
		return err
	}
	for _, child := range next {
		if err := walkDependencies(config, child, active, complete); err != nil {
			return err
		}
	}
	// Shared dependencies are checked once, avoiding exponential traversal of a DAG.
	complete[node] = true
	return nil
}

func dependencies(config *schema.AtmosConfiguration, node string) ([]string, error) {
	kind, name, _ := strings.Cut(node, ":")
	switch kind {
	case "provider":
		return providerDependencies(config, name)
	case "store":
		store, exists := config.StoresConfig[name]
		if !exists || !store.Secret {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		if store.Identity != "" {
			return []string{"identity:" + store.Identity}, nil
		}
	case "identity":
		return identityDependencies(config, name)
	}
	return nil, nil
}

func providerDependencies(config *schema.AtmosConfiguration, name string) ([]string, error) {
	provider, exists := config.Auth.Providers[name]
	if !exists {
		return nil, errUtils.ErrInvalidAuthConfig
	}
	if provider.Kind != "secrets/store" {
		return nil, nil
	}
	refs, ok := provider.Spec["credentials"].(map[string]any)
	if !ok || len(refs) == 0 {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	next := make([]string, 0, len(refs))
	for _, raw := range refs {
		ref, ok := raw.(map[string]any)
		if !ok {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		storeName, _ := ref["store"].(string)
		next = append(next, "store:"+storeName)
	}
	return next, nil
}

func identityDependencies(config *schema.AtmosConfiguration, name string) ([]string, error) {
	identity, exists := config.Auth.Identities[name]
	if !exists {
		return nil, errUtils.ErrInvalidAuthConfig
	}
	switch {
	case identity.Via != nil && identity.Via.Identity != "":
		return []string{"identity:" + identity.Via.Identity}, nil
	case identity.Via != nil && identity.Via.Provider != "":
		return []string{"provider:" + identity.Via.Provider}, nil
	case identity.Provider != "":
		return []string{"provider:" + identity.Provider}, nil
	default:
		return nil, nil
	}
}
