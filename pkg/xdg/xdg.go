package xdg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

// GetXDGCacheDir returns the Atmos cache directory following XDG Base Directory Specification.
// It respects ATMOS_XDG_CACHE_HOME and XDG_CACHE_HOME environment variables.
// The directory is created if it doesn't exist.
func GetXDGCacheDir(subpath string, perm os.FileMode) (string, error) {
	return getXDGDir("XDG_CACHE_HOME", "ATMOS_XDG_CACHE_HOME", xdg.CacheHome, subpath, perm)
}

// GetXDGDataDir returns the Atmos data directory following XDG Base Directory Specification.
// It respects ATMOS_XDG_DATA_HOME and XDG_DATA_HOME environment variables.
// The directory is created if it doesn't exist.
func GetXDGDataDir(subpath string, perm os.FileMode) (string, error) {
	return getXDGDir("XDG_DATA_HOME", "ATMOS_XDG_DATA_HOME", xdg.DataHome, subpath, perm)
}

// GetXDGConfigDir returns the Atmos config directory following XDG Base Directory Specification.
// It respects ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
// The directory is created if it doesn't exist.
func GetXDGConfigDir(subpath string, perm os.FileMode) (string, error) {
	return getXDGDir("XDG_CONFIG_HOME", "ATMOS_XDG_CONFIG_HOME", xdg.ConfigHome, subpath, perm)
}

// getXDGDir is the internal implementation for getting XDG directories.
// It follows this precedence:
// 1. ATMOS_XDG_*_HOME environment variable (Atmos-specific override).
// 2. XDG_*_HOME environment variable (standard XDG variable).
// 3. XDG library default (platform-specific defaults from github.com/adrg/xdg).
func getXDGDir(xdgVar, atmosVar string, defaultDir string, subpath string, perm os.FileMode) (string, error) {
	// Bind both ATMOS_XDG_*_HOME and XDG_*_HOME to support ATMOS override.
	// This allows operators to use ATMOS_XDG_*_HOME to override the standard XDG_*_HOME.
	v := viper.New()
	if err := v.BindEnv(xdgVar, atmosVar, xdgVar); err != nil {
		return "", fmt.Errorf("error binding %s environment variables: %w", xdgVar, err)
	}

	var baseDir string
	if customHome := v.GetString(xdgVar); customHome != "" {
		// Use the custom home from either ATMOS_XDG_*_HOME or XDG_*_HOME.
		baseDir = customHome
	} else {
		// Fall back to XDG library default behavior.
		baseDir = defaultDir
	}

	// Construct the full path.
	fullPath := filepath.Join(baseDir, "atmos", subpath)

	// Create the directory if it doesn't exist.
	if err := os.MkdirAll(fullPath, perm); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", fullPath, err)
	}

	return fullPath, nil
}
