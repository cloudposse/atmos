package config

import (
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Validate rejects reserved configuration instead of silently ignoring policy.
//
//nolint:lintroller // Pure configuration validation without I/O.
func Validate(plugins *schema.PluginsConfig) error {
	if err := validatePublishers(&plugins.Policy); err != nil {
		return err
	}
	if plugins.MaxConcurrency < 0 {
		return fmt.Errorf("%w: plugins.max_concurrency must be at least 1, or 0 for the default", errUtils.ErrPluginInvalidConfig)
	}
	if plugins.Policy.CacheTTL != "" {
		return fmt.Errorf("%w: plugins.policy.cache_ttl is not supported by the native pilot", errUtils.ErrPluginInvalidConfig)
	}
	if err := validateDiscovery(&plugins.Discovery); err != nil {
		return err
	}
	for alias := range plugins.Packages {
		if err := validatePackage(alias, plugins.Packages[alias]); err != nil {
			return err
		}
	}
	return validateRegistries(plugins.Registries)
}

func validateRegistries(registries map[string]schema.PluginRegistry) error {
	for alias, registry := range registries {
		if _, err := RegistryHost(&registry); err != nil {
			return fmt.Errorf("registry %s: %w", alias, err)
		}
		if registry.Auth != "" && registry.Auth != "docker" {
			return fmt.Errorf("%w: registry %s supports only the shared docker credential chain", errUtils.ErrPluginInvalidConfig, alias)
		}
	}
	return nil
}

func validateDiscovery(discovery *schema.PluginDiscoveryConfig) error {
	if discovery.CacheTTL == "" {
		return nil
	}
	ttl, err := time.ParseDuration(discovery.CacheTTL)
	if err != nil || ttl < 0 {
		return fmt.Errorf("%w: plugins.discovery.cache_ttl must be a nonnegative duration", errUtils.ErrPluginInvalidConfig)
	}
	return nil
}

func validatePublishers(policy *schema.PluginPolicy) error {
	if policy.RequireSignature && len(policy.TrustedPublishers) == 0 && len(policy.PublisherConstraints) == 0 {
		return fmt.Errorf("%w: require_signature requires trusted_publishers", errUtils.ErrPluginInvalidConfig)
	}
	groups := append([][]schema.PluginPublisher{policy.TrustedPublishers}, policy.PublisherConstraints...)
	for _, group := range groups {
		for _, publisher := range group {
			if !validPublisher(publisher) {
				return fmt.Errorf("%w: each trusted publisher needs a namespace, signing identity and exact issuer", errUtils.ErrPluginInvalidConfig)
			}
		}
	}
	return nil
}

func validPublisher(publisher schema.PluginPublisher) bool {
	validNamespace := publisher.Publisher != "" && !strings.ContainsAny(publisher.Publisher, "/ \t\r\n*?[]")
	validIdentity := strings.Trim(publisher.Identity, "* \t\r\n") != ""
	validIssuer := publisher.Issuer != "" && !strings.ContainsAny(publisher.Issuer, "*?[] \t\r\n")
	return validNamespace && validIdentity && validIssuer
}

// validatePackage names the package and field up front, before any toast or network access,
// instead of letting an empty value surface later as an obscure registry error.
//
//nolint:gocritic // The declaration is an immutable value from the configuration map.
func validatePackage(alias string, entry schema.PluginPackage) error {
	if entry.Scope != "" && entry.Scope != "project" {
		return fmt.Errorf("%w: package %s requires project scope", errUtils.ErrPluginInvalidConfig, alias)
	}
	if entry.Enabled != nil && !*entry.Enabled {
		return nil
	}
	if entry.Path != "" {
		if err := validatePath(alias, entry.Path); err != nil {
			return err
		}
	} else if err := validateRelease(alias, &entry); err != nil {
		return err
	}
	if _, err := PackageTimeout(entry); err != nil {
		return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.timeout %q is not a positive duration", errUtils.ErrPluginInvalidConfig, alias, entry.Timeout)).
			WithHintf("Set plugins.packages.%s.timeout to a positive Go duration such as `5m`, or remove it for no deadline.", alias).
			Err()
	}
	return nil
}

// validateRelease requires the fields a registry package needs. A package with a path needs neither:
// source and version stay valid beside it and apply when the path is absent.
func validateRelease(alias string, entry *schema.PluginPackage) error {
	if strings.TrimSpace(entry.Source) == "" {
		return missingField(alias, "source", "Set it to an OCI source such as `oci://ghcr.io/acme/oidc`, declare `path` to run a local build, or remove the package.")
	}
	if strings.TrimSpace(entry.Version) == "" {
		return missingField(alias, "version", "Set a version constraint such as `^1.2.0`, or run `atmos plugin install <source>` to record one.")
	}
	return nil
}

// validatePath rejects a path that cannot name a directory. Whether the directory exists and holds a
// plugin is checked at activation, because a profile may name a build that does not exist yet.
func validatePath(alias, path string) error {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.path must name a directory", errUtils.ErrPluginInvalidConfig, alias)).
			WithHintf("Set plugins.packages.%s.path to the directory that holds `plugin.yaml`, such as `build/%s`, or remove it to use the release.", alias, alias).
			Err()
	}
	return nil
}

func missingField(alias, field, hint string) error {
	return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.%s is required", errUtils.ErrPluginInvalidConfig, alias, field)).
		WithHint(hint).
		Err()
}

// PackageTimeout parses the optional per-call deadline; zero means none.
//
//nolint:lintroller,gocritic // Pure parsing of an immutable configuration value; preserve the public value-based API.
func PackageTimeout(entry schema.PluginPackage) (time.Duration, error) {
	if entry.Timeout == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(entry.Timeout)
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("%w: plugins.packages timeout %q must be a positive duration such as 5m", errUtils.ErrPluginInvalidConfig, entry.Timeout)
	}
	return timeout, nil
}
