package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/schema"
)

// injectIdentityEnvironment injects authenticated identity environment variables into container config.
// This is provider-agnostic - it works with AWS, Azure, GitHub, GCP, or any auth provider.
func injectIdentityEnvironment(ctx context.Context, config *devcontainer.Config, identityName string) error {
	defer func() {
		// Use nil for atmosConfig since we're in a utility function.
		// perf.Track(nil, "exec.injectIdentityEnvironment")()
	}()

	if identityName == "" {
		// No identity specified - skip injection.
		return nil
	}

	// 1. Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// 2. Create auth manager.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, nil)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// 3. Authenticate identity (provider-agnostic!)
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to authenticate identity '%s': %w", errUtils.ErrAuthenticationFailed, identityName, err)
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
		return err
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
func addCredentialMounts(config *devcontainer.Config, paths []types.Path) error {
	hostPath, containerPath := parseMountPaths(config.WorkspaceMount, config.WorkspaceFolder)

	for _, credPath := range paths {
		// Check if path exists if required.
		if credPath.Required {
			if _, err := os.Stat(credPath.Location); err != nil {
				return fmt.Errorf("required credential path %s (%s) does not exist: %w",
					credPath.Location, credPath.Purpose, err)
			}
		} else if _, err := os.Stat(credPath.Location); err != nil {
			// Optional path doesn't exist, skip it.
			continue
		}

		// Translate host path to container path.
		userHome, _ := os.UserHomeDir()
		containerMountPath := translatePath(credPath.Location, hostPath, containerPath, userHome)

		// Check metadata for hints.
		readOnly := true // Default to read-only for security.
		if ro, ok := credPath.Metadata["read_only"]; ok && ro == "false" {
			readOnly = false
		}

		// Build mount string in devcontainer format: "type=bind,source=/host,target=/container,readonly"
		mountStr := fmt.Sprintf("type=bind,source=%s,target=%s", credPath.Location, containerMountPath)
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
func getAtmosXDGEnvironment(config *devcontainer.Config) map[string]string {
	// Determine container base path (where workspace is mounted).
	containerBasePath := config.WorkspaceFolder
	if containerBasePath == "" {
		containerBasePath = "/workspace" // Default fallback.
	}

	// Calculate container-relative .atmos path.
	atmosPath := filepath.Join(containerBasePath, ".atmos")

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
		return filepath.Join(containerWorkspace, relPath)
	}

	// If path is under user home, translate to container workspace.
	// Example: ~/.aws/config → /workspace/.aws/config
	if userHome != "" && strings.HasPrefix(hostFilePath, userHome) {
		relPath := strings.TrimPrefix(hostFilePath, userHome)
		return filepath.Join(containerWorkspace, relPath)
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
