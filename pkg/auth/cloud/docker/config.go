package docker

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Permission mode for Docker config directory.
	dockerConfigPerm = 0o700
	// Permission mode for Docker config.json file.
	dockerConfigFilePerm = 0o600
	// Docker config file name.
	configFileName = "config.json"
	// Maximum time to wait to acquire the Docker config file lock.
	fileLockTimeout = 10 * time.Second
)

// ConfigManager manages Docker config.json for ECR authentication.
// It uses file locking to prevent concurrent modification.
//
// The file is round-tripped losslessly: only the target registry's auths entry
// changes, and every other key (credsStore, credHelpers, currentContext,
// plugins, proxies, per-registry identitytoken, unknown keys) is preserved.
// When Docker would read the registry's credentials from a credential helper
// (credHelpers[registry] or credsStore), the credentials are stored in that
// helper, as `docker login` does, because Docker ignores inline auths then.
type ConfigManager struct {
	configDir  string
	configPath string
	mu         sync.Mutex
	// helper runs Docker credential helpers. Nil means the default PATH-based runner.
	helper CredentialHelper
}

// NewConfigManager creates a new Docker config manager using the default Docker config path.
// The config is stored in ~/.docker/config.json by default (or $DOCKER_CONFIG/config.json
// if DOCKER_CONFIG is set). This ensures compatibility with Docker CLI without requiring
// additional environment variable configuration.
func NewConfigManager() (*ConfigManager, error) {
	defer perf.Track(nil, "docker.NewConfigManager")()

	configDir := getDockerConfigDir()

	// Ensure directory exists with secure permissions.
	if err := os.MkdirAll(configDir, dockerConfigPerm); err != nil {
		return nil, fmt.Errorf("%w: failed to create docker config directory: %w", errUtils.ErrDockerConfigWrite, err)
	}

	return &ConfigManager{
		configDir:  configDir,
		configPath: filepath.Join(configDir, configFileName),
		helper:     newExecCredentialHelper(),
	}, nil
}

// getDockerConfigDir returns the Docker config directory.
// Uses DOCKER_CONFIG environment variable if set, otherwise defaults to ~/.docker.
// Returns empty string if home directory cannot be determined (caller should handle error).
func getDockerConfigDir() string {
	// Check DOCKER_CONFIG environment variable (standard Docker behavior).
	//nolint:forbidigo // DOCKER_CONFIG is a standard Docker env var, not Atmos config
	if dockerConfig := os.Getenv("DOCKER_CONFIG"); dockerConfig != "" {
		return dockerConfig
	}

	homeDir, err := homedir.Dir()
	if err != nil {
		// Return empty string - caller will get a clear error when mkdir fails.
		// This is more predictable than silently using cwd.
		return ""
	}

	return filepath.Join(homeDir, ".docker")
}

// WriteAuth writes ECR authorization to Docker config.
// The auth entry is stored as base64(username:password). When Docker resolves the
// registry through a credential helper (credHelpers[registry] or credsStore), the
// credentials are stored in that helper instead and only an empty placeholder
// entry is written to auths, matching `docker login`. A helper failure is returned
// as ErrDockerCredentialHelper and leaves config.json unchanged.
func (m *ConfigManager) WriteAuth(registry, username, password string) error {
	defer perf.Track(nil, "docker.ConfigManager.WriteAuth")()

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.withConfigLock(func() error {
		// Load existing config or create new one.
		config, err := m.loadConfig()
		if err != nil {
			return err
		}

		if helper := config.credentialHelperFor(registry); helper != "" {
			// Docker ignores inline auths for this registry, so writing them would
			// make the login silently ineffective. Store through the helper.
			if err := m.credentialHelper().Store(helper, registry, username, password); err != nil {
				return withHelperHint(err, helper)
			}
			config.setAuth(registry, helperPlaceholderEntry)
			return m.saveConfig(config)
		}

		// Encode credentials as base64(username:password).
		authValue := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

		// Set auth for the registry.
		config.setAuth(registry, inlineAuthEntry(authValue))

		return m.saveConfig(config)
	})
}

// RemoveAuth removes ECR authorization from Docker config.
// It reverses WriteAuth: the registry's auths entry is deleted and, when Docker
// resolves the registry through a credential helper, the credentials are erased
// from that helper. Registries without an auths entry are left alone, so the
// helper is never asked to erase credentials Atmos (or `docker login`) did not
// record. If a helper erase fails, that registry's entry is kept so the logout
// can be retried, and the error is returned.
func (m *ConfigManager) RemoveAuth(registries ...string) error {
	defer perf.Track(nil, "docker.ConfigManager.RemoveAuth")()

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.withConfigLock(func() error {
		// Load existing config.
		config, err := m.loadConfig()
		if err != nil {
			return err
		}

		// Remove auth entries for specified registries.
		var helperErrs []error
		changed := false
		for _, registry := range registries {
			if !config.hasAuth(registry) {
				continue
			}
			if helper := config.credentialHelperFor(registry); helper != "" {
				if err := m.credentialHelper().Erase(helper, registry); err != nil {
					helperErrs = append(helperErrs, withHelperHint(err, helper))
					continue
				}
			}
			config.removeAuth(registry)
			changed = true
		}

		if changed {
			if err := m.saveConfig(config); err != nil {
				helperErrs = append(helperErrs, err)
			}
		}
		return errors.Join(helperErrs...)
	})
}

// credentialHelper returns the configured credential helper runner.
func (m *ConfigManager) credentialHelper() CredentialHelper {
	if m.helper == nil {
		return newExecCredentialHelper()
	}
	return m.helper
}

// withHelperHint classifies a credential helper failure as ErrDockerCredentialHelper
// and makes it actionable.
func withHelperHint(err error, helper string) error {
	if !errors.Is(err, errUtils.ErrDockerCredentialHelper) {
		err = fmt.Errorf("%w: %w", errUtils.ErrDockerCredentialHelper, err)
	}
	return fmt.Errorf("%w (config.json routes this registry to %s%s via credsStore/credHelpers, so Docker ignores inline auths for it; "+
		"make sure that helper is installed and supports store/erase, or map the registry to inline auths with an empty credHelpers entry)",
		err, credentialHelperPrefix, helper)
}

func (m *ConfigManager) withConfigLock(fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	if err := cache.NewFileLock(m.configPath).WithLockContext(ctx, fn); err != nil {
		if errors.Is(err, errUtils.ErrCacheLocked) {
			return fmt.Errorf("%w: failed to acquire lock: %w", errUtils.ErrDockerConfigWrite, err)
		}
		return err
	}
	return nil
}

// GetConfigDir returns the directory containing the Docker config.
func (m *ConfigManager) GetConfigDir() string {
	return m.configDir
}

// GetConfigPath returns the full path to the Docker config file.
func (m *ConfigManager) GetConfigPath() string {
	return m.configPath
}

// GetAuthenticatedRegistries returns list of authenticated ECR registries.
func (m *ConfigManager) GetAuthenticatedRegistries() ([]string, error) {
	defer perf.Track(nil, "docker.ConfigManager.GetAuthenticatedRegistries")()

	m.mu.Lock()
	defer m.mu.Unlock()

	config, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	return config.registries(), nil
}

// loadConfig loads the Docker config from file or returns empty config if not exists.
func (m *ConfigManager) loadConfig() (*rawConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return newRawConfig(), nil
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrDockerConfigRead, err)
	}

	return parseRawConfig(data)
}

// saveConfig writes the Docker config to file atomically (temp file + rename).
// A symlinked config.json is updated through the link, and an existing file
// keeps its permission bits; a new file is created with 0600.
func (m *ConfigManager) saveConfig(config *rawConfig) error {
	data, err := config.marshal()
	if err != nil {
		return fmt.Errorf("%w: failed to marshal config: %w", errUtils.ErrDockerConfigWrite, err)
	}

	target := m.configPath
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}

	perm := os.FileMode(dockerConfigFilePerm)
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() {
		perm = info.Mode().Perm()
	}

	if err := filesystem.NewOSFileSystem().WriteFileAtomic(target, data, perm); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	return nil
}
