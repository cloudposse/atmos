package devcontainer

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// injectIdentityEnvironment injects authenticated identity environment variables into container config.
// This is provider-agnostic - it works with AWS, Azure, GitHub, GCP, or any auth provider.
func injectIdentityEnvironment(ctx context.Context, config *Config, identityName string) error {
	defer func() {
		// Use nil for atmosConfig since we're in a utility function.
		// perf.Track(nil, "devcontainer.injectIdentityEnvironment")()
	}()

	if identityName == "" {
		// No identity specified - skip injection.
		return nil
	}

	// 1. Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return errUtils.Build(err).
			WithExplanation("Failed to load Atmos configuration while injecting identity").
			WithHint("Verify that `atmos.yaml` exists and is properly configured").
			WithHint("Run `atmos version` to check Atmos configuration validity").
			WithHint("See Atmos docs: https://atmos.tools/cli/configuration/").
			WithContext("identity_name", identityName).
			WithExitCode(2).
			Err()
	}

	// 2. Create auth manager.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, nil)
	if err != nil {
		return errUtils.Build(err).
			WithExplanation("Failed to initialize authentication manager").
			WithHint("Verify that auth configuration is valid in `atmos.yaml`").
			WithHint("Check that required auth providers are properly configured").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/auth/").
			WithContext("identity_name", identityName).
			WithExitCode(2).
			Err()
	}

	// 3. Authenticate identity (provider-agnostic!)
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanationf("Failed to authenticate identity `%s`", identityName).
			WithHintf("Verify that the identity `%s` is configured in `atmos.yaml`", identityName).
			WithHint("Run `atmos auth identity list` to see available identities").
			WithHint("Use `atmos auth identity configure` to set up the identity").
			WithHint("Check that required credentials are present and valid").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/auth/auth-identity-configure/").
			WithContext("identity_name", identityName).
			WithExitCode(1).
			Err()
	}

	// 4. Get environment variables from authenticated identity.
	// This uses Identity.Environment() method - works with ANY provider!
	envVars := whoami.Environment
	if envVars == nil {
		envVars = make(map[string]string)
	}

	// 5. Add Atmos XDG environment variables for container paths.
	atmosXDGVars := getAtmosXDGEnvironment(config)
	for k, v := range atmosXDGVars {
		envVars[k] = v
	}

	// 6. Convert credential paths to container mounts (provider-agnostic!).
	if err := addCredentialMounts(config, whoami.Paths); err != nil {
		return errUtils.Build(err).
			WithContext("identity_name", identityName).
			Err()
	}

	// 7. Inject environment variables into container config.
	if config.ContainerEnv == nil {
		config.ContainerEnv = make(map[string]string)
	}
	for k, v := range envVars {
		config.ContainerEnv[k] = v
	}

	return nil
}

// addCredentialMounts adds credential file/directory mounts to devcontainer config.
func addCredentialMounts(config *Config, paths []types.Path) error {
	hostPath, containerPath := parseMountPaths(config.WorkspaceMount, config.WorkspaceFolder)

	for _, credPath := range paths {
		// Expand ~ in paths before using them.
		expandedPath, err := homedir.Expand(credPath.Location)
		if err != nil {
			// If expansion fails, fall back to original path.
			expandedPath = credPath.Location
		}

		// Check if path exists if required.
		if credPath.Required {
			if _, err := os.Stat(expandedPath); err != nil {
				return errUtils.Build(err).
					WithExplanationf("Required credential path `%s` does not exist", expandedPath).
					WithHintf("The identity requires this credential file/directory: %s", credPath.Purpose).
					WithHint("Ensure the required credentials are present before starting the devcontainer").
					WithHint("Check the identity configuration for required credential paths").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/auth/auth-identity-configure/").
					WithContext("credential_path", expandedPath).
					WithContext("purpose", credPath.Purpose).
					WithContext("required", "true").
					WithExitCode(1).
					Err()
			}
		} else if _, err := os.Stat(expandedPath); err != nil {
			// Optional path doesn't exist, skip it.
			continue
		}

		// Translate host path to container path.
		userHome, _ := homedir.Dir()
		containerMountPath := translatePath(expandedPath, hostPath, containerPath, userHome)

		// Check metadata for hints.
		readOnly := true // Default to read-only for security.
		if ro, ok := credPath.Metadata["read_only"]; ok && ro == "false" {
			readOnly = false
		}

		// Build mount string in devcontainer format: "type=bind,source=/host,target=/container,readonly"
		mountStr := fmt.Sprintf("type=bind,source=%s,target=%s", expandedPath, containerMountPath)
		if readOnly {
			mountStr += ",readonly"
		}

		if config.Mounts == nil {
			config.Mounts = []string{}
		}
		config.Mounts = append(config.Mounts, mountStr)
	}

	return nil
}

// getAtmosXDGEnvironment returns Atmos-specific XDG environment variables for the container.
// These ensure Atmos inside the container uses the correct paths for config, cache, and data.
func getAtmosXDGEnvironment(config *Config) map[string]string {
	// Determine container base path (where workspace is mounted).
	containerBasePath := config.WorkspaceFolder
	if containerBasePath == "" {
		containerBasePath = "/workspace" // Default fallback.
	}

	// Calculate container-relative .atmos path.
	// Use path.Join (not filepath.Join) to ensure Unix-style paths for containers.
	//nolint:forbidigo // Container paths must use Unix separators (/) not OS-specific separators
	atmosPath := path.Join(containerBasePath, ".atmos")

	return map[string]string{
		// XDG Base Directory Specification paths.
		"XDG_CONFIG_HOME": atmosPath,
		"XDG_DATA_HOME":   atmosPath,
		"XDG_CACHE_HOME":  atmosPath,

		// Atmos-specific paths.
		"ATMOS_BASE_PATH": containerBasePath,
	}
}

// translatePath translates a single host path to container path.
func translatePath(hostFilePath, hostWorkspace, containerWorkspace, userHome string) string {
	// If path starts with host workspace, translate to container path.
	if strings.HasPrefix(hostFilePath, hostWorkspace) {
		relPath := strings.TrimPrefix(hostFilePath, hostWorkspace)
		// TrimPrefix leaves leading separator; strip it so path.Join doesn't treat as absolute.
		relPath = strings.TrimLeft(relPath, `/\`)
		// Replace all backslashes with forward slashes for container paths.
		relPath = strings.ReplaceAll(relPath, `\`, `/`)
		// Use path.Join (not filepath.Join) to ensure Unix-style paths for containers.
		//nolint:forbidigo // Container paths must use Unix separators (/) not OS-specific separators
		return path.Join(containerWorkspace, relPath)
	}

	// If path is under user home, translate to container workspace.
	// Example: ~/.aws/config → /workspace/.aws/config
	if userHome != "" && strings.HasPrefix(hostFilePath, userHome) {
		relPath := strings.TrimPrefix(hostFilePath, userHome)
		// TrimPrefix leaves leading separator; strip it so path.Join doesn't treat as absolute.
		relPath = strings.TrimLeft(relPath, `/\`)
		// Replace all backslashes with forward slashes for container paths.
		relPath = strings.ReplaceAll(relPath, `\`, `/`)
		// Use path.Join (not filepath.Join) to ensure Unix-style paths for containers.
		//nolint:forbidigo // Container paths must use Unix separators (/) not OS-specific separators
		return path.Join(containerWorkspace, relPath)
	}

	// No translation needed.
	return hostFilePath
}

// parseMountPaths extracts source and target paths from workspace mount string.
func parseMountPaths(workspaceMount, workspaceFolder string) (hostPath, containerPath string) {
	// Parse mount string: "type=bind,source=/host/path,target=/container/path"
	if workspaceMount != "" {
		parts := strings.Split(workspaceMount, ",")
		for _, part := range parts {
			if strings.HasPrefix(part, "source=") {
				hostPath = strings.TrimPrefix(part, "source=")
			}
		}
	}

	containerPath = workspaceFolder
	if containerPath == "" {
		containerPath = "/workspace"
	}

	// If hostPath wasn't found in mount string, use current directory.
	if hostPath == "" {
		cwd, err := os.Getwd()
		if err == nil {
			hostPath = cwd
		}
	}

	return hostPath, containerPath
}
