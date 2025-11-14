package azure

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
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	PermissionRWX = 0o700
	PermissionRW  = 0o600

	// File locking timeouts.
	fileLockTimeout = 10 * time.Second
	fileLockRetry   = 50 * time.Millisecond

	// Logging keys.
	logKeyProvider = "provider"
	logKeyIdentity = "identity"
	logKeyProfile  = "profile"
)

var (
	ErrGetHomeDir                    = errors.New("failed to get home directory")
	ErrCreateCredentialsFile         = errors.New("failed to create credentials file")
	ErrLoadCredentialsFile           = errors.New("failed to load credentials file")
	ErrWriteCredentialsFile          = errors.New("failed to write credentials file")
	ErrSetCredentialsFilePermissions = errors.New("failed to set credentials file permissions")
	ErrCleanupAzureFiles             = errors.New("failed to cleanup Azure files")
	ErrFileLockTimeout               = errors.New("failed to acquire file lock within timeout")
	ErrRemoveProfile                 = errors.New("failed to remove profile")
)

// AcquireFileLock attempts to acquire an exclusive file lock with timeout and retries.
// Exported for use by provider-side code (device_code_cache.go).
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

// AzureFileManager provides helpers to manage Azure credentials files.
type AzureFileManager struct {
	baseDir string
	mu      sync.Mutex
}

// NewAzureFileManager creates a new Azure file manager.
// If basePath is empty, uses default ~/.azure/atmos path.
func NewAzureFileManager(basePath string) (*AzureFileManager, error) {
	defer perf.Track(nil, "azure.NewAzureFileManager")()

	var baseDir string
	if basePath != "" {
		baseDir = basePath
	} else {
		homeDir, err := homedir.Dir()
		if err != nil {
			return nil, errors.Join(ErrGetHomeDir, err)
		}
		baseDir = filepath.Join(homeDir, ".azure", "atmos")
	}

	return &AzureFileManager{
		baseDir: baseDir,
	}, nil
}

// GetCredentialsPath returns the path to the credentials file for the given provider.
func (m *AzureFileManager) GetCredentialsPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, "credentials.json")
}

// WriteCredentials writes Azure credentials to a JSON file.
func (m *AzureFileManager) WriteCredentials(providerName, identityName string, creds *types.AzureCredentials) error {
	defer perf.Track(nil, "azure.WriteCredentials")()

	m.mu.Lock()
	defer m.mu.Unlock()

	credPath := m.GetCredentialsPath(providerName)
	credDir := filepath.Dir(credPath)

	// Create directory.
	if err := os.MkdirAll(credDir, PermissionRWX); err != nil {
		return errors.Join(ErrCreateCredentialsFile, err)
	}

	// Acquire file lock.
	lockPath := credPath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock credentials file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Marshal credentials to JSON.
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to marshal credentials: %w", ErrWriteCredentialsFile, err)
	}

	// Write credentials file.
	if err := os.WriteFile(credPath, data, PermissionRW); err != nil {
		return errors.Join(ErrWriteCredentialsFile, err)
	}

	log.Debug("Wrote Azure credentials",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"credentials_path", credPath,
		"has_graph_token", creds.GraphAPIToken != "",
		"has_graph_expiration", creds.GraphAPIExpiration != "",
	)

	return nil
}

// LoadCredentials loads Azure credentials from a JSON file.
func (m *AzureFileManager) LoadCredentials(providerName string) (*types.AzureCredentials, error) {
	defer perf.Track(nil, "azure.LoadCredentials")()

	m.mu.Lock()
	defer m.mu.Unlock()

	credPath := m.GetCredentialsPath(providerName)

	// Check if file exists.
	if _, err := os.Stat(credPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: credentials file does not exist: %s", errUtils.ErrAuthenticationFailed, credPath)
		}
		return nil, errors.Join(ErrLoadCredentialsFile, err)
	}

	// Acquire file lock for reading.
	lockPath := credPath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock credentials file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Read credentials file.
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, errors.Join(ErrLoadCredentialsFile, err)
	}

	// Unmarshal credentials.
	var creds types.AzureCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal credentials: %w", ErrLoadCredentialsFile, err)
	}

	log.Debug("Loaded Azure credentials",
		logKeyProvider, providerName,
		"credentials_path", credPath,
	)

	return &creds, nil
}

// Cleanup removes Azure files for the given provider.
func (m *AzureFileManager) Cleanup(providerName string) error {
	defer perf.Track(nil, "azure.Cleanup")()

	m.mu.Lock()
	defer m.mu.Unlock()

	providerDir := filepath.Join(m.baseDir, providerName)

	// Check if provider directory exists.
	if _, err := os.Stat(providerDir); err != nil {
		if os.IsNotExist(err) {
			log.Debug("Azure files directory does not exist, nothing to cleanup",
				logKeyProvider, providerName,
				"dir", providerDir,
			)
			return nil
		}
		return errors.Join(ErrCleanupAzureFiles, err)
	}

	// Remove provider directory.
	if err := os.RemoveAll(providerDir); err != nil {
		return errors.Join(ErrCleanupAzureFiles, err)
	}

	log.Debug("Cleaned up Azure files",
		logKeyProvider, providerName,
		"dir", providerDir,
	)

	return nil
}

// CredentialsExist checks if credentials file exists for the given provider.
func (m *AzureFileManager) CredentialsExist(providerName string) bool {
	credPath := m.GetCredentialsPath(providerName)
	_, err := os.Stat(credPath)
	return err == nil
}
