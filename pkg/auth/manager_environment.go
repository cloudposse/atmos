package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GetEnvironmentVariables returns the environment variables for an identity
// without performing authentication or validation.
func (m *manager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.Manager.GetEnvironmentVariables")()

	// Verify identity exists.
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This builds the authentication chain and sets manager reference so the identity
	// can resolve the root provider for file-based credentials.
	// This is best-effort - if it fails, the identity will fall back to config-based resolution.
	_ = m.ensureIdentityHasManager(identityName)

	// Get environment variables from the identity.
	env, err := identity.Environment()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get environment variables: %w", errUtils.ErrAuthManager, err)
	}

	// Compose integration environment variables.
	env = m.composeIntegrationEnvironment(identityName, env)

	return env, nil
}

// PrepareShellEnvironment prepares environment variables for subprocess execution.
// Takes current environment list and returns it with auth credentials configured.
// This calls identity.PrepareEnvironment() internally to configure file-based credentials.
func (m *manager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "auth.Manager.PrepareShellEnvironment")()

	// Verify identity exists.
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This is best-effort - if it fails, the identity will fall back to config-based resolution.
	_ = m.ensureIdentityHasManager(identityName)

	// Convert input environment list to map for identity.PrepareEnvironment().
	envMap := environListToMap(currentEnv)

	// Call identity.PrepareEnvironment() to configure auth credentials.
	// This is provider-specific (AWS sets AWS_SHARED_CREDENTIALS_FILE, AWS_PROFILE, etc.).
	preparedEnvMap, err := identity.PrepareEnvironment(ctx, envMap)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to prepare shell environment for identity %q: %w", errUtils.ErrAuthManager, identityName, err)
	}

	// Compose integration environment variables.
	preparedEnvMap = m.composeIntegrationEnvironment(identityName, preparedEnvMap)

	// Convert map back to list for subprocess execution.
	return mapToEnvironList(preparedEnvMap), nil
}

// composeIntegrationEnvironment collects environment variables from all integrations
// linked to the identity and composes them into the base environment.
func (m *manager) composeIntegrationEnvironment(identityName string, base map[string]string) map[string]string {
	defer perf.Track(nil, "auth.Manager.composeIntegrationEnvironment")()

	linkedIntegrations := m.findIntegrationsForIdentity(identityName, false)
	if len(linkedIntegrations) == 0 {
		return base
	}

	for _, integrationName := range linkedIntegrations {
		integrationConfig, exists := m.config.Integrations[integrationName]
		if !exists {
			continue
		}

		integration, err := integrations.Create(&integrations.IntegrationConfig{
			Name:   integrationName,
			Config: &integrationConfig,
		})
		if err != nil {
			log.Debug("Failed to create integration for environment", "integration", integrationName, "error", err)
			continue
		}

		vars, err := integration.Environment()
		if err != nil {
			log.Debug("Failed to get integration environment", "integration", integrationName, "error", err)
			continue
		}

		base = composeEnvironmentVariables(base, vars)
	}

	return base
}

// composeEnvironmentVariables merges additions into base.
// KUBECONFIG values are colon-separated with deduplication.
// All other keys use last-write-wins.
func composeEnvironmentVariables(base, additions map[string]string) map[string]string {
	if base == nil {
		base = make(map[string]string)
	}

	for key, value := range additions {
		if key == "KUBECONFIG" {
			base[key] = appendPathList(base[key], value)
		} else {
			base[key] = value
		}
	}

	return base
}

// appendPathList appends a path to a path list (colon-separated) with deduplication.
func appendPathList(existing, newPath string) string {
	if existing == "" {
		return newPath
	}
	if newPath == "" {
		return existing
	}

	// Check for duplicates.
	sep := string(os.PathListSeparator)
	parts := strings.Split(existing, sep)
	for _, part := range parts {
		if part == newPath {
			return existing
		}
	}

	return existing + sep + newPath
}

// environListToMap converts environment variable list to map.
// Input: ["KEY=value", "FOO=bar"]
// Output: {"KEY": "value", "FOO": "bar"}.
func environListToMap(envList []string) map[string]string {
	envMap := make(map[string]string, len(envList))
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}
	return envMap
}

// mapToEnvironList converts environment variable map to list.
// Input: {"KEY": "value", "FOO": "bar"}
// Output: ["KEY=value", "FOO=bar"].
func mapToEnvironList(envMap map[string]string) []string {
	envList := make([]string, 0, len(envMap))
	for key, value := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", key, value))
	}
	return envList
}
