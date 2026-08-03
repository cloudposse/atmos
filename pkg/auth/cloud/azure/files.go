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

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	PermissionRWX = 0o700
	PermissionRW  = 0o600

	// File locking timeouts.
	fileLockTimeout = 10 * time.Second

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

func withFileLock(path string, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	if err := cache.NewFileLock(path).WithLockContext(ctx, fn); err != nil {
		if errors.Is(err, errUtils.ErrCacheLocked) {
			return fmt.Errorf("%w: %w", ErrFileLockTimeout, err)
		}
		return err
	}
	return nil
}

// AzureFileManager provides helpers to manage Azure credentials files.
type AzureFileManager struct {
	baseDir string
	mu      sync.Mutex
}

// NewAzureFileManager creates a new Azure file manager.
// If basePath is empty, uses default ~/.azure/atmos/{realm} path.
// The realm parameter provides credential isolation between different repositories.
func NewAzureFileManager(basePath string, realm string) (*AzureFileManager, error) {
	defer perf.Track(nil, "azure.NewAzureFileManager")()

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
			baseDir = filepath.Join(homeDir, ".azure", "atmos", realm)
		} else {
			baseDir = filepath.Join(homeDir, ".azure", "atmos")
		}
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

	if creds == nil {
		return fmt.Errorf("%w: Azure credentials cannot be nil", ErrWriteCredentialsFile)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	credPath := m.GetCredentialsPath(providerName)
	credDir := filepath.Dir(credPath)

	// Create directory.
	if err := os.MkdirAll(credDir, PermissionRWX); err != nil {
		return errors.Join(ErrCreateCredentialsFile, err)
	}

	return withFileLock(credPath, func() error {
		data, err := json.MarshalIndent(creds, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal credentials: %w", ErrWriteCredentialsFile, err)
		}
		if err := os.WriteFile(credPath, data, PermissionRW); err != nil {
			return errors.Join(ErrWriteCredentialsFile, err)
		}

		log.Debug(
			"Wrote Azure credentials",
			logKeyProvider, providerName,
			logKeyIdentity, identityName,
			"credentials_path", credPath,
			"has_graph_token", creds.GraphAPIToken != "",
			"has_graph_expiration", creds.GraphAPIExpiration != "",
		)
		return nil
	})
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

	var creds *types.AzureCredentials
	if err := withFileLock(credPath, func() error {
		data, err := os.ReadFile(credPath)
		if err != nil {
			return errors.Join(ErrLoadCredentialsFile, err)
		}
		value := &types.AzureCredentials{}
		if err := json.Unmarshal(data, value); err != nil {
			return fmt.Errorf("%w: failed to unmarshal credentials: %w", ErrLoadCredentialsFile, err)
		}
		creds = value
		log.Debug("Loaded Azure credentials", logKeyProvider, providerName, "credentials_path", credPath)
		return nil
	}); err != nil {
		return nil, err
	}
	return creds, nil
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
			log.Debug(
				"Azure files directory does not exist, nothing to cleanup",
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

	log.Debug(
		"Cleaned up Azure files",
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
