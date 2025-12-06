package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

func init() {
	// Override adrg/xdg defaults for macOS to follow CLI tool conventions.
	// This must happen in init() to ensure it runs before any code uses xdg.ConfigHome, etc.
	// CLI tools use ~/.config on all platforms for consistency, while GUI apps use
	// platform-specific locations like ~/Library/Application Support on macOS.
	if runtime.GOOS == "darwin" {
		//nolint:forbidigo // os.UserHomeDir() needed here to override adrg/xdg library defaults in init()
		homeDir, err := os.UserHomeDir()
		if err == nil {
			// Override macOS defaults to use CLI tool conventions.
			xdg.CacheHome = filepath.Join(homeDir, ".cache")
			xdg.DataHome = filepath.Join(homeDir, ".local", "share")
			xdg.ConfigHome = filepath.Join(homeDir, ".config")
		}
	}
}

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
	// Bind ATMOS_XDG_*_HOME and XDG_*_HOME separately to enforce explicit precedence.
	v := viper.New()
	if err := v.BindEnv(atmosVar, atmosVar); err != nil {
		return "", fmt.Errorf("error binding %s environment variable: %w", atmosVar, err)
	}
	if err := v.BindEnv(xdgVar, xdgVar); err != nil {
		return "", fmt.Errorf("error binding %s environment variable: %w", xdgVar, err)
	}

	var baseDir string
	// Check ATMOS_XDG_*_HOME first (highest priority).
	if atmosHome := v.GetString(atmosVar); atmosHome != "" {
		baseDir = atmosHome
	} else if xdgHome := v.GetString(xdgVar); xdgHome != "" {
		// Fall back to XDG_*_HOME if ATMOS override not set.
		baseDir = xdgHome
	} else {
		// Fall back to XDG library default.
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
