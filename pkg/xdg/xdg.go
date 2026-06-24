package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

// DefaultCacheDirPerm is the default permission for cache directories.
const DefaultCacheDirPerm = 0o755

// Environment variable names for the XDG base directories. Each standard XDG
// variable has an Atmos-specific override that takes precedence over it.
const (
	// EnvXDGCacheHome is the standard XDG cache base directory variable.
	EnvXDGCacheHome = "XDG_CACHE_HOME"
	// EnvAtmosXDGCacheHome is the Atmos-specific cache base override.
	EnvAtmosXDGCacheHome = "ATMOS_XDG_CACHE_HOME"
	// EnvXDGDataHome is the standard XDG data base directory variable.
	EnvXDGDataHome = "XDG_DATA_HOME"
	// EnvAtmosXDGDataHome is the Atmos-specific data base override.
	EnvAtmosXDGDataHome = "ATMOS_XDG_DATA_HOME"
	// EnvXDGConfigHome is the standard XDG config base directory variable.
	EnvXDGConfigHome = "XDG_CONFIG_HOME"
	// EnvAtmosXDGConfigHome is the Atmos-specific config base override.
	EnvAtmosXDGConfigHome = "ATMOS_XDG_CONFIG_HOME"
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
	return getXDGDir(EnvXDGCacheHome, EnvAtmosXDGCacheHome, xdg.CacheHome, subpath, perm)
}

// LookupXDGCacheDir resolves the Atmos cache directory path without creating it.
// Use this for read-only checks where directory creation is not desired.
func LookupXDGCacheDir(subpath string) string {
	return lookupXDGDir(EnvXDGCacheHome, EnvAtmosXDGCacheHome, xdg.CacheHome, subpath)
}

// LookupCacheHomeBase returns the cache base directory configured via the
// environment, plus the name of the variable that supplied it
// (ATMOS_XDG_CACHE_HOME wins over XDG_CACHE_HOME). Both values are empty when
// neither variable is set, meaning the platform default applies. Use this for
// safety checks that need to report which variable configured the base.
func LookupCacheHomeBase() (value, source string) {
	v := viper.New()
	// Best-effort bind; fall through to empty on error.
	_ = v.BindEnv(EnvAtmosXDGCacheHome, EnvAtmosXDGCacheHome)
	_ = v.BindEnv(EnvXDGCacheHome, EnvXDGCacheHome)

	if dir := v.GetString(EnvAtmosXDGCacheHome); dir != "" {
		return dir, EnvAtmosXDGCacheHome
	}
	if dir := v.GetString(EnvXDGCacheHome); dir != "" {
		return dir, EnvXDGCacheHome
	}
	return "", ""
}

// GetXDGDataDir returns the Atmos data directory following XDG Base Directory Specification.
// It respects ATMOS_XDG_DATA_HOME and XDG_DATA_HOME environment variables.
// The directory is created if it doesn't exist.
func GetXDGDataDir(subpath string, perm os.FileMode) (string, error) {
	return getXDGDir(EnvXDGDataHome, EnvAtmosXDGDataHome, xdg.DataHome, subpath, perm)
}

// GetXDGConfigDir returns the Atmos config directory following XDG Base Directory Specification.
// It respects ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
// The directory is created if it doesn't exist.
func GetXDGConfigDir(subpath string, perm os.FileMode) (string, error) {
	return getXDGDir(EnvXDGConfigHome, EnvAtmosXDGConfigHome, xdg.ConfigHome, subpath, perm)
}

// LookupXDGConfigDir resolves the Atmos config directory path without creating it.
// Use this for read-only checks where directory creation is not desired.
func LookupXDGConfigDir(subpath string) string {
	return lookupXDGDir(EnvXDGConfigHome, EnvAtmosXDGConfigHome, xdg.ConfigHome, subpath)
}

// lookupXDGDir resolves an XDG directory path without creating it.
func lookupXDGDir(xdgVar, atmosVar string, defaultDir string, subpath string) string {
	v := viper.New()
	// Best-effort bind; fall through to default on error.
	_ = v.BindEnv(atmosVar, atmosVar)
	_ = v.BindEnv(xdgVar, xdgVar)

	if dir := v.GetString(atmosVar); dir != "" {
		return filepath.Join(dir, "atmos", subpath)
	}
	if dir := v.GetString(xdgVar); dir != "" {
		return filepath.Join(dir, "atmos", subpath)
	}
	return filepath.Join(defaultDir, "atmos", subpath)
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
