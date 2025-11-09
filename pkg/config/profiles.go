package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// ProfileLocation represents a location where profiles can be stored.
type ProfileLocation struct {
	Path       string // Absolute path to profile directory.
	Type       string // "configurable", "project-hidden", "xdg", "project".
	Precedence int    // Lower number = higher precedence.
}

// discoverProfileLocations returns all possible profile locations in precedence order.
// Precedence (highest to lowest):
// 1. Configurable (profiles.base_path in atmos.yaml)
// 2. Project-local hidden (.atmos/profiles/)
// 3. XDG user profiles ($XDG_CONFIG_HOME/atmos/profiles/)
// 4. Project-local non-hidden (profiles/)
func discoverProfileLocations(atmosConfig *schema.AtmosConfiguration) ([]ProfileLocation, error) {
	var locations []ProfileLocation

	// Derive base directory from CliConfigPath (which may be a file path to atmos.yaml).
	baseDir := filepath.Dir(atmosConfig.CliConfigPath)

	// 1. Configurable base_path (highest precedence).
	if atmosConfig.Profiles.BasePath != "" {
		basePath := atmosConfig.Profiles.BasePath

		// If relative, resolve from atmos.yaml directory.
		if !filepath.IsAbs(basePath) {
			basePath = filepath.Join(baseDir, basePath)
		}

		locations = append(locations, ProfileLocation{
			Path:       basePath,
			Type:       "configurable",
			Precedence: 1,
		})
	}

	// 2. Project-local hidden profiles.
	projectHiddenPath := filepath.Join(baseDir, ".atmos", "profiles")
	locations = append(locations, ProfileLocation{
		Path:       projectHiddenPath,
		Type:       "project-hidden",
		Precedence: 2,
	})

	// 3. XDG user profiles.
	xdgPath, err := xdg.GetXDGConfigDir("profiles", 0o755)
	if err == nil && xdgPath != "" {
		locations = append(locations, ProfileLocation{
			Path:       xdgPath,
			Type:       "xdg",
			Precedence: 3,
		})
	}

	// 4. Project-local non-hidden profiles (lowest precedence).
	projectPath := filepath.Join(baseDir, "profiles")
	locations = append(locations, ProfileLocation{
		Path:       projectPath,
		Type:       "project",
		Precedence: 4,
	})

	return locations, nil
}

// findProfileDirectory searches for a profile across all locations.
// Returns the path to the profile directory and the location type.
func findProfileDirectory(profileName string, locations []ProfileLocation) (string, string, error) {
	// Sort by precedence (lower number = higher precedence).
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Precedence < locations[j].Precedence
	})

	for _, loc := range locations {
		profilePath := filepath.Join(loc.Path, profileName)

		// Check if directory exists.
		info, err := os.Stat(profilePath)
		if err == nil && info.IsDir() {
			return profilePath, loc.Type, nil
		}
	}

	// Build list of searched paths for error message.
	var searchedPaths []string
	for _, loc := range locations {
		searchedPaths = append(searchedPaths, filepath.Join(loc.Path, profileName))
	}

	return "", "", fmt.Errorf("%w: '%s' (searched: %s)", errUtils.ErrProfileNotFound, profileName, strings.Join(searchedPaths, ", "))
}

// listAvailableProfiles returns all profiles found across all locations.
func listAvailableProfiles(locations []ProfileLocation) (map[string][]string, error) {
	// Map: profile name -> list of locations where found.
	profiles := make(map[string][]string)

	for _, loc := range locations {
		// Check if location exists.
		if _, err := os.Stat(loc.Path); os.IsNotExist(err) {
			continue
		}

		// List directories in location.
		entries, err := os.ReadDir(loc.Path)
		if err != nil {
			continue // Skip inaccessible locations.
		}

		for _, entry := range entries {
			if entry.IsDir() {
				profileName := entry.Name()
				profiles[profileName] = append(profiles[profileName], loc.Path)
			}
		}
	}

	return profiles, nil
}

// loadProfileFiles loads all YAML files from a profile directory.
// Uses the shared loadAtmosConfigsFromDirectory function for consistent behavior.
// See .scratch/profiles-loading-refactor.md for details on this refactoring.
func loadProfileFiles(v *viper.Viper, profileDir string, profileName string) error {
	// Validate directory exists.
	info, err := os.Stat(profileDir)
	if err != nil {
		return fmt.Errorf("%w: profile '%s' directory does not exist: %s (run 'atmos profile list' to see available profiles)", errUtils.ErrProfileDirNotExist, profileName, profileDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: profile '%s' path is not a directory: %s", errUtils.ErrProfileDirNotExist, profileName, profileDir)
	}

	// Use shared loading function (see .scratch/profiles-loading-refactor.md).
	// This reuses SearchAtmosConfig() and mergeConfigFile() for consistency with .atmos.d/
	// Benefits:
	// - Recursive directory support
	// - Priority file handling (atmos.yaml first)
	// - Depth-based sorting
	// - Command array merging
	// - Atmos YAML function processing
	searchPattern := filepath.Join(profileDir, "**", "*")
	source := fmt.Sprintf("profile '%s'", profileName)

	return loadAtmosConfigsFromDirectory(searchPattern, v, source)
}

// loadProfiles loads the specified profiles in order (left-to-right precedence).
func loadProfiles(v *viper.Viper, profileNames []string, atmosConfig *schema.AtmosConfiguration) error {
	if len(profileNames) == 0 {
		return nil // No profiles to load.
	}

	// Discover profile locations.
	locations, err := discoverProfileLocations(atmosConfig)
	if err != nil {
		return fmt.Errorf("%w: %s (check profiles.base_path in atmos.yaml)", errUtils.ErrProfileDiscovery, err)
	}

	// Load each profile in order (left-to-right).
	for _, profileName := range profileNames {
		// Find profile directory.
		profileDir, locType, err := findProfileDirectory(profileName, locations)
		if err != nil {
			// Add list of available profiles to error.
			available, _ := listAvailableProfiles(locations)
			var availableNames []string
			for name := range available {
				availableNames = append(availableNames, name)
			}
			sort.Strings(availableNames)

			return fmt.Errorf("%w (available profiles: %s - run 'atmos profile list' for details)", err, strings.Join(availableNames, ", "))
		}

		log.Info("Loading profile",
			"profile", profileName,
			"location", locType,
			"path", profileDir)

		// Load all files from profile directory.
		if err := loadProfileFiles(v, profileDir, profileName); err != nil {
			return err // Error already enriched.
		}
	}

	return nil
}
