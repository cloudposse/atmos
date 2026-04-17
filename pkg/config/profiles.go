package config

import (
	"errors"
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
// 1. Configurable (profiles.base_path in atmos.yaml).
// 2. Project-local hidden (.atmos/profiles/).
// 3. XDG user profiles ($XDG_CONFIG_HOME/atmos/profiles/).
// 4. Project-local non-hidden (profiles/).
func discoverProfileLocations(atmosConfig *schema.AtmosConfiguration) ([]ProfileLocation, error) {
	var locations []ProfileLocation

	// Use CliConfigPath as base directory (it contains the directory of atmos.yaml).
	baseDir := atmosConfig.CliConfigPath

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

	// Build list of searched paths and location types for error message.
	var searchedPaths []string
	var locationTypes []string
	for _, loc := range locations {
		searchedPaths = append(searchedPaths, filepath.Join(loc.Path, profileName))
		locationTypes = append(locationTypes, loc.Type)
	}

	return "", "", errUtils.Build(errUtils.ErrProfileNotFound).
		WithExplanationf("Profile `%s` not found in any configured location", profileName).
		WithExplanationf("Searched in: `%s`", strings.Join(searchedPaths, ", ")).
		WithHint("Run `atmos profile list` to see all available profiles").
		WithHint("Create the profile directory in one of the search locations").
		WithHintf("To create: `mkdir -p <location>/profiles/%s`", profileName).
		WithContext("profile", profileName).
		WithContext("searched_paths", strings.Join(searchedPaths, ", ")).
		WithContext("location_types", strings.Join(locationTypes, ", ")).
		WithExitCode(2).
		Err()
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
		// Differentiate between "not found" and "not accessible".
		if os.IsNotExist(err) {
			return errUtils.Build(errUtils.ErrProfileDirNotExist).
				WithExplanationf("Profile `%s` directory does not exist", profileName).
				WithExplanationf("Expected path: `%s`", profileDir).
				WithHint("Run `atmos profile list` to see available profiles").
				WithHintf("Create directory: `mkdir -p %s`", profileDir).
				WithContext("profile", profileName).
				WithContext("path", profileDir).
				WithContext("error", err.Error()).
				WithExitCode(2).
				Err()
		}

		// Directory exists but is not accessible (permissions, etc.).
		return errUtils.Build(errUtils.ErrProfileDirNotAccessible).
			WithExplanationf("Profile `%s` directory exists but is not accessible", profileName).
			WithExplanationf("Expected path: `%s`", profileDir).
			WithHint("Check directory permissions and ownership").
			WithContext("profile", profileName).
			WithContext("path", profileDir).
			WithContext("error", err.Error()).
			WithExitCode(2).
			Err()
	}
	if !info.IsDir() {
		return errUtils.Build(errUtils.ErrProfileDirNotExist).
			WithExplanationf("Profile `%s` path exists but is not a directory", profileName).
			WithExplanationf("Found a file at: `%s`", profileDir).
			WithHint("Remove the file and create a directory instead").
			WithHintf("Run: `rm %s && mkdir -p %s`", profileDir, profileDir).
			WithContext("profile", profileName).
			WithContext("path", profileDir).
			WithContext("type", "file").
			WithExitCode(2).
			Err()
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
		return errUtils.Build(errUtils.ErrProfileDiscovery).
			WithExplanationf("Failed to discover profile locations: `%s`", err).
			WithExplanation("The system could not determine where to look for profiles").
			WithHint("Verify `profiles.base_path` in `atmos.yaml` exists and is accessible").
			WithHint("Check `XDG_CONFIG_HOME` environment variable if using XDG locations").
			WithHint("Run `atmos describe config` to view current configuration").
			WithContext("base_path", atmosConfig.Profiles.BasePath).
			WithContext("config_dir", atmosConfig.CliConfigPath).
			WithExitCode(2).
			Err()
	}

	// Load each profile in order (left-to-right).
	for _, profileName := range profileNames {
		// Find profile directory.
		profileDir, locType, err := findProfileDirectory(profileName, locations)
		if err != nil {
			// Check if this is a profile not found error.
			if errors.Is(err, errUtils.ErrProfileNotFound) {
				// Add list of available profiles to help user.
				available, _ := listAvailableProfiles(locations)
				var availableNames []string
				for name := range available {
					availableNames = append(availableNames, name)
				}
				sort.Strings(availableNames)

				builder := errUtils.Build(errUtils.ErrProfileNotFound).
					WithExplanationf("Profile `%s` does not exist in any configured location", profileName).
					WithExplanationf("Available profiles are: `%s`", strings.Join(availableNames, ", "))

				// Check if the profile name matches an auth identity name.
				// This helps users who confuse ATMOS_PROFILE (configuration profiles)
				// with ATMOS_IDENTITY (authentication identities).
				// See: https://github.com/cloudposse/atmos/issues/2071
				if isAuthIdentityName(profileName, atmosConfig) {
					builder = builder.
						WithHintf("`%s` is an auth identity, not a configuration profile", profileName).
						WithHint("If you want to select an auth identity, use `ATMOS_IDENTITY` or `--identity` instead of `ATMOS_PROFILE`").
						WithHintf("Example: `export ATMOS_IDENTITY=%s` or `atmos terraform plan --identity %s`", profileName, profileName)
				} else {
					builder = builder.
						WithHint("Check the spelling of the profile name")
				}

				return builder.
					WithHint("Run `atmos profile list` for detailed information about each profile").
					WithHint("Verify the profile directory exists if you expect to see it").
					WithContext("profile", profileName).
					WithContext("requested_profiles", strings.Join(profileNames, ", ")).
					WithContext("available_count", fmt.Sprintf("%d", len(availableNames))).
					WithExitCode(2).
					Err()
			}

			// For other errors, preserve original error chain.
			return err
		}

		log.Debug("Loading profile",
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

// isAuthIdentityName checks whether the given name matches an auth identity in the config.
// This is used to provide better error messages when users confuse ATMOS_PROFILE with ATMOS_IDENTITY.
func isAuthIdentityName(name string, atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil || len(atmosConfig.Auth.Identities) == 0 {
		return false
	}

	// Check case-insensitive match against identity names.
	nameLower := strings.ToLower(name)
	for identityName := range atmosConfig.Auth.Identities {
		if strings.ToLower(identityName) == nameLower {
			return true
		}
	}

	return false
}

// HasExplicitProfile reports whether the user explicitly selected a profile via
// the `--profile` flag or the `ATMOS_PROFILE` environment variable.
//
// An implicit default from `profiles.default` is NOT considered explicit — this
// distinction gates the interactive identity fallback (see PRD:
// interactive-profile-suggestion). We never override an explicit user choice,
// but we do prompt when the only reason a profile loaded was the implicit default.
func HasExplicitProfile() bool {
	profiles, _ := getProfilesFromFallbacks()
	return len(profiles) > 0
}

// ProfileDefinesIdentity reports whether the named profile defines the given
// identity in its auth.identities section. Match is case-insensitive.
//
// Uses a scoped Viper instance — does NOT mutate global config. Returns false +
// nil when the profile exists but does not define the identity. Returns false +
// error when the profile cannot be located or loaded.
func ProfileDefinesIdentity(atmosConfig *schema.AtmosConfiguration, profileName, identityName string) (bool, error) {
	if atmosConfig == nil {
		return false, fmt.Errorf("%w: atmosConfig is nil", errUtils.ErrInvalidAuthConfig)
	}
	if strings.TrimSpace(profileName) == "" || strings.TrimSpace(identityName) == "" {
		return false, nil
	}

	locations, err := discoverProfileLocations(atmosConfig)
	if err != nil {
		return false, err
	}

	profileDir, _, err := findProfileDirectory(profileName, locations)
	if err != nil {
		// Profile itself doesn't exist — treat as "does not define identity".
		if errors.Is(err, errUtils.ErrProfileNotFound) {
			return false, nil
		}
		return false, err
	}

	// Fresh Viper instance — scoped, no global state mutation.
	v := viper.New()
	v.SetConfigType("yaml")
	if err := loadProfileFiles(v, profileDir, profileName); err != nil {
		return false, err
	}

	identities := v.GetStringMap("auth.identities")
	wantLower := strings.ToLower(identityName)
	for name := range identities {
		if strings.ToLower(name) == wantLower {
			return true, nil
		}
	}
	return false, nil
}

// ProfilesWithIdentity returns the names of all profiles that define the given
// identity in their auth.identities section. The returned list is sorted
// alphabetically for deterministic output. Uses case-insensitive matching.
//
// Errors loading individual profiles are logged at debug level and that profile
// is skipped — a single broken profile should not hide the others.
func ProfilesWithIdentity(atmosConfig *schema.AtmosConfiguration, identityName string) ([]string, error) {
	if atmosConfig == nil || strings.TrimSpace(identityName) == "" {
		return nil, nil
	}

	locations, err := discoverProfileLocations(atmosConfig)
	if err != nil {
		return nil, err
	}

	available, err := listAvailableProfiles(locations)
	if err != nil {
		return nil, err
	}

	var matches []string
	for profileName := range available {
		defines, checkErr := ProfileDefinesIdentity(atmosConfig, profileName, identityName)
		if checkErr != nil {
			log.Debug("Skipping profile during identity search",
				"profile", profileName,
				"error", checkErr)
			continue
		}
		if defines {
			matches = append(matches, profileName)
		}
	}

	sort.Strings(matches)
	return matches, nil
}

// ProfileDefinesAuthConfig reports whether the named profile defines any auth
// configuration — either a non-empty auth.identities map or a non-empty
// auth.providers map. Used by the identity-agnostic fallback in auth commands
// (login, exec, shell, env, console, whoami) when the base atmos.yaml has no
// usable auth config at all.
//
// Uses a scoped Viper instance — does NOT mutate global config.
func ProfileDefinesAuthConfig(atmosConfig *schema.AtmosConfiguration, profileName string) (bool, error) {
	if atmosConfig == nil {
		return false, fmt.Errorf("%w: atmosConfig is nil", errUtils.ErrInvalidAuthConfig)
	}
	if strings.TrimSpace(profileName) == "" {
		return false, nil
	}

	locations, err := discoverProfileLocations(atmosConfig)
	if err != nil {
		return false, err
	}

	profileDir, _, err := findProfileDirectory(profileName, locations)
	if err != nil {
		if errors.Is(err, errUtils.ErrProfileNotFound) {
			return false, nil
		}
		return false, err
	}

	v := viper.New()
	v.SetConfigType("yaml")
	if err := loadProfileFiles(v, profileDir, profileName); err != nil {
		return false, err
	}

	if len(v.GetStringMap("auth.identities")) > 0 {
		return true, nil
	}
	if len(v.GetStringMap("auth.providers")) > 0 {
		return true, nil
	}
	return false, nil
}

// ProfilesWithAuthConfig returns the names of all profiles that define any auth
// configuration (identities or providers). The returned list is sorted
// alphabetically for deterministic output.
//
// Errors loading individual profiles are logged at debug level and that profile
// is skipped — a single broken profile should not hide the others.
func ProfilesWithAuthConfig(atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	if atmosConfig == nil {
		return nil, nil
	}

	locations, err := discoverProfileLocations(atmosConfig)
	if err != nil {
		return nil, err
	}

	available, err := listAvailableProfiles(locations)
	if err != nil {
		return nil, err
	}

	var matches []string
	for profileName := range available {
		defines, checkErr := ProfileDefinesAuthConfig(atmosConfig, profileName)
		if checkErr != nil {
			log.Debug("Skipping profile during auth-config search",
				"profile", profileName,
				"error", checkErr)
			continue
		}
		if defines {
			matches = append(matches, profileName)
		}
	}

	sort.Strings(matches)
	return matches, nil
}
