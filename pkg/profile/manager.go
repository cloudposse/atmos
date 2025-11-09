package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config"
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// GetProfileLocations returns all possible profile locations in precedence order.
func (m *DefaultProfileManager) GetProfileLocations(atmosConfig *schema.AtmosConfiguration) ([]ProfileLocation, error) {
	defer perf.Track(atmosConfig, "profile.GetProfileLocations")()

	var locations []ProfileLocation
	cliConfigPath := atmosConfig.CliConfigPath

	// 1. Configurable base_path (highest precedence).
	if atmosConfig.Profiles.BasePath != "" {
		basePath := atmosConfig.Profiles.BasePath

		// If relative, resolve from atmos.yaml directory.
		if !filepath.IsAbs(basePath) {
			basePath = filepath.Join(cliConfigPath, basePath)
		}

		exists := dirExists(basePath)
		locations = append(locations, ProfileLocation{
			Path:       basePath,
			Type:       "configurable",
			Precedence: 1,
			Exists:     exists,
		})
	}

	// 2. Project-local hidden profiles.
	projectHiddenPath := filepath.Join(cliConfigPath, ".atmos", "profiles")
	locations = append(locations, ProfileLocation{
		Path:       projectHiddenPath,
		Type:       "project-hidden",
		Precedence: 2,
		Exists:     dirExists(projectHiddenPath),
	})

	// 3. XDG user profiles.
	xdgPath, err := xdg.GetXDGConfigDir("profiles", 0o755)
	if err == nil && xdgPath != "" {
		locations = append(locations, ProfileLocation{
			Path:       xdgPath,
			Type:       "xdg",
			Precedence: 3,
			Exists:     dirExists(xdgPath),
		})
	}

	// 4. Project-local non-hidden profiles (lowest precedence).
	projectPath := filepath.Join(cliConfigPath, "profiles")
	locations = append(locations, ProfileLocation{
		Path:       projectPath,
		Type:       "project",
		Precedence: 4,
		Exists:     dirExists(projectPath),
	})

	return locations, nil
}

// ListProfiles returns all available profiles across all locations.
func (m *DefaultProfileManager) ListProfiles(atmosConfig *schema.AtmosConfiguration) ([]ProfileInfo, error) {
	defer perf.Track(atmosConfig, "profile.ListProfiles")()

	locations, err := m.GetProfileLocations(atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrProfileDiscovery, err)
	}

	// Map to track profiles (key: profile name, value: ProfileInfo).
	// Higher precedence locations will override lower precedence.
	profileMap := make(map[string]ProfileInfo)

	// Sort locations by precedence (higher precedence first).
	// Process in reverse order so higher precedence overwrites lower.
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Precedence > locations[j].Precedence
	})

	for _, loc := range locations {
		if !loc.Exists {
			continue
		}

		// List directories in location.
		entries, err := os.ReadDir(loc.Path)
		if err != nil {
			log.Trace("Failed to read profile location", "path", loc.Path, "error", err)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			profileName := entry.Name()
			profilePath := filepath.Join(loc.Path, profileName)

			// Get list of files in profile directory.
			files, err := listProfileFiles(profilePath)
			if err != nil {
				log.Trace("Failed to list profile files", "profile", profileName, "error", err)
				files = []string{} // Continue with empty file list.
			}

			// Try to load metadata if atmos.yaml exists.
			var metadata *schema.ConfigMetadata
			atmosYamlPath := filepath.Join(profilePath, "atmos.yaml")
			if fileExists(atmosYamlPath) {
				metadata, _ = loadProfileMetadata(atmosYamlPath)
			}

			// Store profile info (higher precedence will overwrite).
			profileMap[profileName] = ProfileInfo{
				Name:         profileName,
				Path:         profilePath,
				LocationType: loc.Type,
				Files:        files,
				Metadata:     metadata,
			}
		}
	}

	// Convert map to sorted slice.
	var profiles []ProfileInfo
	for _, profile := range profileMap {
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

// GetProfile returns detailed information about a specific profile.
func (m *DefaultProfileManager) GetProfile(atmosConfig *schema.AtmosConfiguration, profileName string) (*ProfileInfo, error) {
	defer perf.Track(atmosConfig, "profile.GetProfile")()

	locations, err := m.GetProfileLocations(atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrProfileDiscovery, err)
	}

	// Sort by precedence (lower number = higher precedence).
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Precedence < locations[j].Precedence
	})

	// Find the first matching profile.
	for _, loc := range locations {
		if !loc.Exists {
			continue
		}

		profilePath := filepath.Join(loc.Path, profileName)
		if !dirExists(profilePath) {
			continue
		}

		// Found the profile - get details.
		files, err := listProfileFiles(profilePath)
		if err != nil {
			log.Trace("Failed to list profile files", "profile", profileName, "error", err)
			files = []string{}
		}

		// Try to load metadata.
		var metadata *schema.ConfigMetadata
		atmosYamlPath := filepath.Join(profilePath, "atmos.yaml")
		if fileExists(atmosYamlPath) {
			metadata, _ = loadProfileMetadata(atmosYamlPath)
		}

		return &ProfileInfo{
			Name:         profileName,
			Path:         profilePath,
			LocationType: loc.Type,
			Files:        files,
			Metadata:     metadata,
		}, nil
	}

	// Profile not found.
	return nil, fmt.Errorf("%w: '%s'", errUtils.ErrProfileNotFound, profileName)
}

// listProfileFiles returns all YAML files in a profile directory.
func listProfileFiles(profilePath string) ([]string, error) {
	var files []string

	// Use config.SearchAtmosConfig to find all YAML files.
	searchPattern := filepath.Join(profilePath, "**", "*")
	foundPaths, err := config.SearchAtmosConfig(searchPattern)
	if err != nil {
		return nil, err
	}

	// Convert absolute paths to relative paths from profile directory.
	for _, absPath := range foundPaths {
		relPath, err := filepath.Rel(profilePath, absPath)
		if err != nil {
			log.Trace("Failed to get relative path", "path", absPath, "error", err)
			continue
		}
		files = append(files, relPath)
	}

	sort.Strings(files)
	return files, nil
}

// loadProfileMetadata loads metadata from a profile's atmos.yaml file.
func loadProfileMetadata(atmosYamlPath string) (*schema.ConfigMetadata, error) {
	data, err := os.ReadFile(atmosYamlPath)
	if err != nil {
		return nil, err
	}

	var config struct {
		Metadata schema.ConfigMetadata `yaml:"metadata"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Only return metadata if at least one field is set.
	if config.Metadata.Name == "" &&
		config.Metadata.Description == "" &&
		config.Metadata.Version == "" &&
		len(config.Metadata.Tags) == 0 {
		return nil, nil
	}

	return &config.Metadata, nil
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
