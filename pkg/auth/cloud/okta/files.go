package okta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// File and directory permissions.
	PermissionRWX = 0o700
	PermissionRW  = 0o600

	// File locking timeouts.
	fileLockTimeout = 10 * time.Second
	fileLockRetry   = 50 * time.Millisecond

	// File names.
	tokensFileName = "tokens.json"

	// Logging keys.
	logKeyProvider = "provider"
	logKeyIdentity = "identity"
)

var (
	ErrGetHomeDir                    = errors.New("failed to get home directory")
	ErrCreateTokensFile              = errors.New("failed to create tokens file")
	ErrLoadTokensFile                = errors.New("failed to load tokens file")
	ErrWriteTokensFile               = errors.New("failed to write tokens file")
	ErrSetTokensFilePermissions      = errors.New("failed to set tokens file permissions")
	ErrCleanupOktaFiles              = errors.New("failed to cleanup Okta files")
	ErrFileLockTimeout               = errors.New("failed to acquire file lock within timeout")
	ErrRemoveIdentity                = errors.New("failed to remove identity")
	ErrMarshalTokens                 = errors.New("failed to marshal tokens")
	ErrUnmarshalTokens               = errors.New("failed to unmarshal tokens")
	ErrTokensFileNotFound            = errors.New("tokens file not found")
)

// AcquireFileLock attempts to acquire an exclusive file lock with timeout and retries.
// Exported for use by provider-side code.
func AcquireFileLock(lockPath string) (*flock.Flock, error) {
	lock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	ticker := time.NewTicker(fileLockRetry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %s", ErrFileLockTimeout, lockPath)
		case <-ticker.C:
			locked, err := lock.TryLock()
			if err != nil {
				return nil, fmt.Errorf("failed to acquire lock: %w", err)
			}
			if locked {
				log.Debug("Acquired file lock", "lock_file", lockPath)
				return lock, nil
			}
			log.Debug("Waiting for file lock", "lock_file", lockPath)
		}
	}
}

// OktaFileManager provides helpers to manage Okta token files.
type OktaFileManager struct {
	baseDir string
	mu      sync.Mutex
}

// NewOktaFileManager creates a new Okta file manager.
// If basePath is empty, uses default ~/.config/atmos/okta/{realm} path.
// The realm parameter provides credential isolation between different repositories.
func NewOktaFileManager(basePath string, realm string) (*OktaFileManager, error) {
	defer perf.Track(nil, "okta.NewOktaFileManager")()

	var baseDir string
	if basePath != "" {
		baseDir = basePath
	} else {
		homeDir, err := homedir.Dir()
		if err != nil {
			return nil, errors.Join(ErrGetHomeDir, err)
		}
		// Include realm in path for credential isolation.
		if realm != "" {
			baseDir = filepath.Join(homeDir, ".config", "atmos", realm, "okta")
		} else {
			baseDir = filepath.Join(homeDir, ".config", "atmos", "okta")
		}
	}

	return &OktaFileManager{
		baseDir: baseDir,
	}, nil
}

// GetBaseDir returns the base directory for Okta files.
func (m *OktaFileManager) GetBaseDir() string {
	return m.baseDir
}

// GetDisplayPath returns the display path with ~ for home directory.
func (m *OktaFileManager) GetDisplayPath() string {
	homeDir, err := homedir.Dir()
	if err != nil {
		return m.baseDir
	}

	if len(m.baseDir) >= len(homeDir) && m.baseDir[:len(homeDir)] == homeDir {
		return "~" + m.baseDir[len(homeDir):]
	}
	return m.baseDir
}

// GetProviderDir returns the directory for a specific provider.
func (m *OktaFileManager) GetProviderDir(providerName string) string {
	return filepath.Join(m.baseDir, providerName)
}

// GetTokensPath returns the path to the tokens file for the given provider.
func (m *OktaFileManager) GetTokensPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, tokensFileName)
}

// WriteTokens writes Okta tokens to a JSON file.
func (m *OktaFileManager) WriteTokens(providerName string, tokens *OktaTokens) error {
	defer perf.Track(nil, "okta.WriteTokens")()

	if tokens == nil {
		return fmt.Errorf("%w: Okta tokens cannot be nil", ErrWriteTokensFile)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tokensPath := m.GetTokensPath(providerName)
	tokensDir := filepath.Dir(tokensPath)

	// Create directory.
	if err := os.MkdirAll(tokensDir, PermissionRWX); err != nil {
		return errors.Join(ErrCreateTokensFile, err)
	}

	// Acquire file lock.
	lockPath := tokensPath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock tokens file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Marshal tokens to JSON.
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMarshalTokens, err)
	}

	// Write tokens file.
	if err := os.WriteFile(tokensPath, data, PermissionRW); err != nil {
		return errors.Join(ErrWriteTokensFile, err)
	}

	log.Debug("Wrote Okta tokens",
		logKeyProvider, providerName,
		"tokens_path", tokensPath,
		"expires_at", tokens.ExpiresAt.Format(time.RFC3339),
	)

	return nil
}

// LoadTokens loads Okta tokens from a JSON file.
func (m *OktaFileManager) LoadTokens(providerName string) (*OktaTokens, error) {
	defer perf.Track(nil, "okta.LoadTokens")()

	m.mu.Lock()
	defer m.mu.Unlock()

	tokensPath := m.GetTokensPath(providerName)

	// Check if file exists.
	if _, err := os.Stat(tokensPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrTokensFileNotFound, tokensPath)
		}
		return nil, errors.Join(ErrLoadTokensFile, err)
	}

	// Acquire file lock for reading.
	lockPath := tokensPath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock tokens file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Read tokens file.
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		return nil, errors.Join(ErrLoadTokensFile, err)
	}

	// Unmarshal tokens.
	var tokens OktaTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnmarshalTokens, err)
	}

	log.Debug("Loaded Okta tokens",
		logKeyProvider, providerName,
		"tokens_path", tokensPath,
		"expires_at", tokens.ExpiresAt.Format(time.RFC3339),
	)

	return &tokens, nil
}

// Cleanup removes Okta files for the given provider.
func (m *OktaFileManager) Cleanup(providerName string) error {
	defer perf.Track(nil, "okta.Cleanup")()

	m.mu.Lock()
	defer m.mu.Unlock()

	providerDir := filepath.Join(m.baseDir, providerName)

	// Check if provider directory exists.
	if _, err := os.Stat(providerDir); err != nil {
		if os.IsNotExist(err) {
			log.Debug("Okta files directory does not exist, nothing to cleanup",
				logKeyProvider, providerName,
				"dir", providerDir,
			)
			return nil
		}
		return errors.Join(ErrCleanupOktaFiles, err)
	}

	// Remove provider directory.
	if err := os.RemoveAll(providerDir); err != nil {
		return errors.Join(ErrCleanupOktaFiles, err)
	}

	log.Debug("Cleaned up Okta files",
		logKeyProvider, providerName,
		"dir", providerDir,
	)

	return nil
}

// CleanupAll removes all Okta files.
func (m *OktaFileManager) CleanupAll() error {
	defer perf.Track(nil, "okta.CleanupAll")()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if base directory exists.
	if _, err := os.Stat(m.baseDir); err != nil {
		if os.IsNotExist(err) {
			log.Debug("Okta base directory does not exist, nothing to cleanup",
				"dir", m.baseDir,
			)
			return nil
		}
		return errors.Join(ErrCleanupOktaFiles, err)
	}

	// Remove base directory.
	if err := os.RemoveAll(m.baseDir); err != nil {
		return errors.Join(ErrCleanupOktaFiles, err)
	}

	log.Debug("Cleaned up all Okta files", "dir", m.baseDir)

	return nil
}

// TokensExist checks if tokens file exists for the given provider.
func (m *OktaFileManager) TokensExist(providerName string) bool {
	tokensPath := m.GetTokensPath(providerName)
	_, err := os.Stat(tokensPath)
	return err == nil
}

// DeleteIdentity removes files for a specific identity.
// For Okta, the provider and identity share the same directory.
func (m *OktaFileManager) DeleteIdentity(ctx context.Context, providerName, identityName string) error {
	defer perf.Track(nil, "okta.DeleteIdentity")()

	_ = ctx // Context available for future use.

	m.mu.Lock()
	defer m.mu.Unlock()

	// For Okta, identity files are stored in the provider directory.
	providerDir := filepath.Join(m.baseDir, providerName)

	// Check if provider directory exists.
	if _, err := os.Stat(providerDir); err != nil {
		if os.IsNotExist(err) {
			log.Debug("Okta identity directory does not exist, nothing to delete",
				logKeyProvider, providerName,
				logKeyIdentity, identityName,
				"dir", providerDir,
			)
			return nil
		}
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	// Remove provider directory (contains identity tokens).
	if err := os.RemoveAll(providerDir); err != nil {
		return errors.Join(ErrRemoveIdentity, err)
	}

	log.Debug("Deleted Okta identity files",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"dir", providerDir,
	)

	return nil
}
