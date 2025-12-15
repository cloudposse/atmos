package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// dockerConfigPerm is the permission mode for Docker config directory.
	dockerConfigPerm = 0700
	// dockerConfigFilePerm is the permission mode for Docker config.json file.
	dockerConfigFilePerm = 0600
	// configFileName is the Docker config file name.
	configFileName = "config.json"
	// lockFileSuffix is the suffix for lock files.
	lockFileSuffix = ".lock"
)

// ConfigManager manages Docker config.json for ECR authentication.
// It uses file locking to prevent concurrent modification.
type ConfigManager struct {
	configDir  string
	configPath string
	mu         sync.Mutex
}

// dockerConfig represents the Docker config.json structure.
type dockerConfig struct {
	Auths map[string]authEntry `json:"auths,omitempty"`
}

// authEntry represents a single auth entry in Docker config.
type authEntry struct {
	Auth string `json:"auth,omitempty"`
}

// NewConfigManager creates a new Docker config manager using XDG paths.
// The config is stored in ~/.config/atmos/docker/config.json by default,
// respecting ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
func NewConfigManager() (*ConfigManager, error) {
	defer perf.Track(nil, "docker.NewConfigManager")()

	configDir, err := xdg.GetXDGConfigDir("docker", dockerConfigPerm)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	return &ConfigManager{
		configDir:  configDir,
		configPath: filepath.Join(configDir, configFileName),
	}, nil
}

// WriteAuth writes ECR authorization to Docker config.
// The auth entry is stored as base64(username:password).
func (m *ConfigManager) WriteAuth(registry, username, password string) error {
	defer perf.Track(nil, "docker.ConfigManager.WriteAuth")()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Acquire file lock.
	lock := flock.New(m.configPath + lockFileSuffix)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("%w: failed to acquire lock: %w", errUtils.ErrDockerConfigWrite, err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	// Load existing config or create new one.
	config, err := m.loadConfig()
	if err != nil {
		return err
	}

	// Encode credentials as base64(username:password).
	authValue := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	// Set auth for the registry.
	if config.Auths == nil {
		config.Auths = make(map[string]authEntry)
	}
	config.Auths[registry] = authEntry{Auth: authValue}

	// Write config back.
	return m.saveConfig(config)
}

// RemoveAuth removes ECR authorization from Docker config.
func (m *ConfigManager) RemoveAuth(registries ...string) error {
	defer perf.Track(nil, "docker.ConfigManager.RemoveAuth")()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Acquire file lock.
	lock := flock.New(m.configPath + lockFileSuffix)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("%w: failed to acquire lock: %w", errUtils.ErrDockerConfigWrite, err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	// Load existing config.
	config, err := m.loadConfig()
	if err != nil {
		return err
	}

	// Remove auth entries for specified registries.
	for _, registry := range registries {
		delete(config.Auths, registry)
	}

	// Write config back.
	return m.saveConfig(config)
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

	registries := make([]string, 0, len(config.Auths))
	for registry := range config.Auths {
		registries = append(registries, registry)
	}
	return registries, nil
}

// loadConfig loads the Docker config from file or returns empty config if not exists.
func (m *ConfigManager) loadConfig() (*dockerConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &dockerConfig{Auths: make(map[string]authEntry)}, nil
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrDockerConfigRead, err)
	}

	var config dockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %w", errUtils.ErrDockerConfigRead, err)
	}

	if config.Auths == nil {
		config.Auths = make(map[string]authEntry)
	}

	return &config, nil
}

// saveConfig writes the Docker config to file.
func (m *ConfigManager) saveConfig(config *dockerConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to marshal config: %w", errUtils.ErrDockerConfigWrite, err)
	}

	if err := os.WriteFile(m.configPath, data, dockerConfigFilePerm); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	return nil
}
