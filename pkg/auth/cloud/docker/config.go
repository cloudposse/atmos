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
	// warn emits user-facing warnings. Nil means ui.Warning (stderr).
	warn func(string)
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
// as ErrDockerCredentialHelper and leaves config.json unchanged; when the helper
// does not implement store (it gets credentials itself, like
// docker-credential-ecr-login), the error also matches
// ErrDockerCredentialHelperStoreUnsupported and HelperManagedNotice returns the
// message callers show instead of failing. If config.json
// cannot be saved after a successful helper store, the stored credential is
// erased again so it is not orphaned in the helper. After a successful write it
// warns (without failing) when DOCKER_AUTH_CONFIG is set, because Docker reads
// credentials from that variable before config.json.
func (m *ConfigManager) WriteAuth(registry, username, password string) error {
	defer perf.Track(nil, "docker.ConfigManager.WriteAuth")()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store under the bare hostname, as `docker login https://host/` does, so a
	// scheme or trailing slash cannot create a key Docker never looks up.
	registry = convertToHostname(registry)

	if err := m.withConfigLock(func() error {
		return m.writeAuthLocked(registry, username, password)
	}); err != nil {
		return err
	}

	// The login succeeded, but Docker may still not use it in this shell.
	if msg := dockerAuthConfigWarning(registry); msg != "" {
		m.warner()(msg)
	}
	return nil
}

// writeAuthLocked is the body of WriteAuth; the caller holds the config lock.
func (m *ConfigManager) writeAuthLocked(registry, username, password string) error {
	// Load existing config or create new one.
	config, err := m.loadConfig()
	if err != nil {
		return err
	}

	if helper := config.credentialHelperFor(registry); helper != "" {
		// Docker ignores inline auths for this registry, so writing them would
		// make the login silently ineffective. Store through the helper.
		return m.writeHelperAuth(config, helper, registry, username, password)
	}

	// Encode credentials as base64(username:password).
	authValue := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	// Set auth for the registry.
	config.setAuth(registry, inlineAuthEntry(authValue))

	return m.saveConfig(config)
}

// writeHelperAuth stores the credentials in the helper Docker uses for registry
// and records the registry in config.json. If config.json cannot be saved, the
// helper is rolled back to what it held before.
func (m *ConfigManager) writeHelperAuth(config *rawConfig, helper, registry, username, password string) error {
	previous := m.snapshotHelperCredential(config, helper, registry)

	if err := m.credentialHelper().Store(helper, registry, username, password); err != nil {
		if errors.Is(err, errUtils.ErrDockerCredentialHelperStoreUnsupported) {
			// The helper gets credentials itself; see HelperManagedNotice.
			return &helperManagedError{registry: registry, helper: helper, err: err}
		}
		return withHelperHint(err, helper)
	}
	config.setAuth(registry, helperPlaceholderEntry)
	if err := m.saveConfig(config); err != nil {
		return m.rollbackHelperStore(err, helper, registry, previous)
	}
	return nil
}

// RemoveAuth removes ECR authorization from Docker config.
// It reverses WriteAuth: the registry's auths entry is deleted and, when Docker
// resolves the registry through a credential helper, the credentials are erased
// from that helper. Registries without an auths entry are left alone, so the
// helper is never asked to erase credentials Atmos (or `docker login`) did not
// record. If a helper erase fails, that registry's entry is kept so the logout
// can be retried, and the error is returned. Like `docker logout`, a registry is
// normalized to its hostname and every auths key Docker reads for that host is
// removed, including legacy "https://host" or "host/v2/" keys; each key is erased
// from the helper Docker resolves for that exact key.
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
			// Match every key Docker reads for this host, legacy variants included.
			for _, key := range config.authKeysFor(convertToHostname(registry)) {
				if helper := config.credentialHelperFor(key); helper != "" {
					if err := m.credentialHelper().Erase(helper, key); err != nil {
						helperErrs = append(helperErrs, withHelperHint(err, helper))
						continue
					}
				}
				config.removeAuth(key)
				changed = true
			}
		}

		if changed {
			if err := m.saveConfig(config); err != nil {
				helperErrs = append(helperErrs, err)
			}
		}
		return errors.Join(helperErrs...)
	})
}

// helperSnapshot is what a credential helper held for a registry before a store.
type helperSnapshot struct {
	// known is false when the previous credential could not be read, in which
	// case a rollback must leave the helper alone rather than guess.
	known bool
	// found reports whether the helper held a credential for the registry.
	found    bool
	username string
	secret   string
}

// snapshotHelperCredential reads the credential the helper holds for registry
// before it is replaced. Only a registry config.json already lists can have a
// credential Atmos must not lose (`docker login` records the registry there), so
// other registries are not read: that also avoids asking helpers such as
// docker-credential-ecr-login to fetch credentials just to take a snapshot.
func (m *ConfigManager) snapshotHelperCredential(config *rawConfig, helper, registry string) helperSnapshot {
	if !config.hasAuth(registry) {
		return helperSnapshot{known: true}
	}
	cred, err := m.credentialHelper().Get(helper, registry)
	if err != nil {
		return helperSnapshot{}
	}
	if cred == nil {
		return helperSnapshot{known: true}
	}
	return helperSnapshot{known: true, found: true, username: cred.Username, secret: cred.Secret}
}

// rollbackHelperStore undoes a helper store whose config.json save failed, on a
// best-effort basis. When the helper held a credential before, it is restored,
// so a failed save does not leave the registry without its working login.
// Otherwise the new credential is erased, because without the auths placeholder
// RemoveAuth would never erase it and it would be orphaned in the helper (for
// example the OS keychain). When the previous credential could not be read the
// helper is left as is. The save error is always returned, joined with the
// rollback error when the rollback also fails.
func (m *ConfigManager) rollbackHelperStore(saveErr error, helper, registry string, previous helperSnapshot) error {
	var err error
	switch {
	case !previous.known:
		return saveErr
	case previous.found:
		err = m.credentialHelper().Store(helper, registry, previous.username, previous.secret)
	default:
		err = m.credentialHelper().Erase(helper, registry)
	}
	if err != nil {
		return errors.Join(saveErr, withHelperHint(err, helper))
	}
	return saveErr
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

	target := resolveConfigTarget(m.configPath)

	perm := os.FileMode(dockerConfigFilePerm)
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() {
		perm = info.Mode().Perm()
	}

	if err := filesystem.NewOSFileSystem().WriteFileAtomic(target, data, perm); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	return nil
}
