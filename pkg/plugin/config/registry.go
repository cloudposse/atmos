package config

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegistryHost validates discovery sources and resolves their single authentication host.
// A host-only registry configures authentication without enabling discovery.
//
//nolint:lintroller // Pure configuration validation without I/O.
func RegistryHost(registry *schema.PluginRegistry) (string, error) {
	if registry == nil {
		return "", errUtils.ErrPluginInvalidConfig
	}
	host := ""
	if registry.Host != "" {
		var err error
		host, err = normalizeRegistryHost(registry.Host)
		if err != nil {
			return "", err
		}
	}
	for index, source := range RegistrySources(registry) {
		sourceHost, err := registrySourceHost(source, registry.Catalog != "" && index == 0)
		if err != nil {
			return "", err
		}
		if host != "" && host != sourceHost {
			return "", fmt.Errorf("%w: discovery sources must share the configured registry host", errUtils.ErrPluginInvalidConfig)
		}
		host = sourceHost
	}
	if host == "" {
		return "", fmt.Errorf("%w: registry requires host, catalog, or repositories", errUtils.ErrPluginInvalidConfig)
	}
	return host, nil
}

// RegistrySources returns configured catalog and repository sources in declaration order.
//
//nolint:lintroller // Pure configuration lookup without I/O.
func RegistrySources(registry *schema.PluginRegistry) []string {
	if registry == nil {
		return nil
	}
	sources := make([]string, 0, len(registry.Repositories)+1)
	if registry.Catalog != "" {
		sources = append(sources, registry.Catalog)
	}
	return append(sources, registry.Repositories...)
}

func registrySourceHost(source string, catalog bool) (string, error) {
	if !strings.HasPrefix(source, "oci://") {
		return "", fmt.Errorf("%w: discovery sources require oci:// references", errUtils.ErrPluginInvalidConfig)
	}
	raw := strings.TrimPrefix(source, "oci://")
	host, _, separated := strings.Cut(raw, "/")
	if !separated || (!strings.ContainsAny(host, ".:") && host != "localhost") {
		return "", fmt.Errorf("%w: discovery sources require an explicit registry and repository", errUtils.ErrPluginInvalidConfig)
	}
	if catalog {
		ref, err := name.ParseReference(raw, name.StrictValidation)
		if err != nil {
			return "", fmt.Errorf("%w: invalid catalog reference", errUtils.ErrPluginInvalidConfig)
		}
		return normalizeRegistryHost(ref.Context().RegistryStr())
	}
	repo, err := name.NewRepository(raw, name.StrictValidation)
	if err != nil {
		return "", fmt.Errorf("%w: discovery repositories must be untagged OCI repositories", errUtils.ErrPluginInvalidConfig)
	}
	return normalizeRegistryHost(repo.RegistryStr())
}

func normalizeRegistryHost(host string) (string, error) {
	registry, err := name.NewRegistry(strings.ToLower(host), name.StrictValidation)
	if err != nil || strings.ContainsAny(host, "/@ \t\r\n") {
		return "", fmt.Errorf("%w: invalid registry host", errUtils.ErrPluginInvalidConfig)
	}
	return registry.Name(), nil
}
