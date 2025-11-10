package provisioning

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultCacheDir is the default cache directory relative to XDG_CACHE_HOME.
	DefaultCacheDir = "atmos/auth"

	// ProvisionedFileName is the filename for provisioned identities.
	ProvisionedFileName = "provisioned-identities.yaml"
)

// Writer handles writing provisioned identities to disk.
type Writer struct {
	// CacheDir is the base cache directory (e.g., ~/.cache/atmos/auth).
	CacheDir string
}

// NewWriter creates a new provisioner writer.
func NewWriter() (*Writer, error) {
	defer perf.Track(nil, "provisioning.NewWriter")()

	cacheDir, err := getDefaultCacheDir()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get cache directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	return &Writer{
		CacheDir: cacheDir,
	}, nil
}

// Write writes provisioned identities to the cache directory for the specified provider.
func (w *Writer) Write(result *Result) (string, error) {
	defer perf.Track(nil, "provisioning.Writer.Write")()

	if result == nil {
		return "", fmt.Errorf("%w: result cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	if result.Provider == "" {
		return "", fmt.Errorf("%w: provider name cannot be empty", errUtils.ErrInvalidAuthConfig)
	}

	// Create provider-specific cache directory.
	providerDir := filepath.Join(w.CacheDir, result.Provider)
	if err := os.MkdirAll(providerDir, 0700); err != nil {
		return "", fmt.Errorf("%w: failed to create cache directory %s: %w", errUtils.ErrInvalidAuthConfig, providerDir, err)
	}

	// Build config structure.
	config := buildConfig(result)

	// Marshal to YAML.
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal provisioned identities: %w", errUtils.ErrParseFile, err)
	}

	// Write to file.
	filePath := filepath.Join(providerDir, ProvisionedFileName)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("%w: failed to write provisioned identities to %s: %w", errUtils.ErrParseFile, filePath, err)
	}

	return filePath, nil
}

// buildConfig constructs the auth configuration structure for provisioned identities.
func buildConfig(result *Result) map[string]interface{} {
	config := map[string]interface{}{
		"auth": map[string]interface{}{
			"identities": result.Identities,
			"_metadata": map[string]interface{}{
				"provisioned_at": result.ProvisionedAt.Format(time.RFC3339),
				"source":         result.Metadata.Source,
				"provider":       result.Provider,
			},
		},
	}

	// Add counts if available.
	if result.Metadata.Counts != nil {
		metadata := config["auth"].(map[string]interface{})["_metadata"].(map[string]interface{})
		metadata["counts"] = map[string]int{
			"accounts":   result.Metadata.Counts.Accounts,
			"roles":      result.Metadata.Counts.Roles,
			"identities": result.Metadata.Counts.Identities,
		}
	}

	// Add extra metadata if available.
	if len(result.Metadata.Extra) > 0 {
		metadata := config["auth"].(map[string]interface{})["_metadata"].(map[string]interface{})
		metadata["extra"] = result.Metadata.Extra
	}

	return config
}

// getDefaultCacheDir returns the default cache directory.
// Uses XDG_CACHE_HOME if set, otherwise ~/.cache/atmos/auth.
func getDefaultCacheDir() (string, error) {
	defer perf.Track(nil, "provisioning.getDefaultCacheDir")()

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		cacheHome = filepath.Join(homeDir, ".cache")
	}

	return filepath.Join(cacheHome, DefaultCacheDir), nil
}

// GetProvisionedIdentitiesPath returns the path to the provisioned identities file for a provider.
func (w *Writer) GetProvisionedIdentitiesPath(providerName string) string {
	defer perf.Track(nil, "provisioning.Writer.GetProvisionedIdentitiesPath")()

	return filepath.Join(w.CacheDir, providerName, ProvisionedFileName)
}

// Remove removes the provisioned identities file for a provider.
func (w *Writer) Remove(providerName string) error {
	defer perf.Track(nil, "provisioning.Writer.Remove")()

	filePath := w.GetProvisionedIdentitiesPath(providerName)

	// Check if file exists.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // Already removed.
	}

	// Remove file.
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("%w: failed to remove provisioned identities file %s: %w", errUtils.ErrParseFile, filePath, err)
	}

	return nil
}
