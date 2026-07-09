package profile

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProfileInfo contains information about a discovered profile.
type ProfileInfo struct {
	Name         string                 // Profile name (directory name).
	Path         string                 // Absolute path to profile directory.
	LocationType string                 // Location type: "configurable", "project-hidden", "xdg", "project".
	Files        []string               // List of configuration files in the profile.
	Metadata     *schema.ConfigMetadata // Optional metadata from the profile.
}

// ProfileManager defines the interface for profile operations.
// This interface allows for testing with mocks and provides a clean API for profile management.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=profile.go -destination=mock_profile_test.go -package=profile
type ProfileManager interface {
	// ListProfiles returns all available profiles across all locations.
	ListProfiles(atmosConfig *schema.AtmosConfiguration) ([]ProfileInfo, error)

	// GetProfile returns detailed information about a specific profile.
	GetProfile(atmosConfig *schema.AtmosConfiguration, profileName string) (*ProfileInfo, error)

	// GetProfileLocations returns all possible profile locations in precedence order.
	GetProfileLocations(atmosConfig *schema.AtmosConfiguration) ([]ProfileLocation, error)
}

// ProfileLocation represents a location where profiles can be stored.
type ProfileLocation struct {
	Path       string // Absolute path to profile directory.
	Type       string // "configurable", "project-hidden", "xdg", "project".
	Precedence int    // Lower number = higher precedence.
	Exists     bool   // Whether the directory exists.
}

// DefaultProfileManager is the concrete implementation of ProfileManager.
type DefaultProfileManager struct{}

// NewProfileManager creates a new DefaultProfileManager.
func NewProfileManager() ProfileManager {
	defer perf.Track(nil, "profile.NewProfileManager")()

	return &DefaultProfileManager{}
}

// DisplayPath returns a compact, user-facing path for profile output.
// Project-local paths are shown relative to the current command directory when
// that does not require "../" traversal. Home-local paths use "~"; everything
// else stays absolute so the location remains unambiguous.
func DisplayPath(path string) string {
	defer perf.Track(nil, "profile.DisplayPath")()

	if path == "" {
		return path
	}

	cleaned := filepath.Clean(path)
	if cwd, err := os.Getwd(); err == nil {
		if rel, ok := relativeDisplayPath(cwd, cleaned); ok {
			return rel
		}
	}

	if home, err := homedir.Dir(); err == nil && home != "" {
		if rel, ok := relativeDisplayPath(home, cleaned); ok {
			if rel == "." {
				return "~"
			}
			return filepath.Join("~", rel)
		}
	}

	return cleaned
}

func relativeDisplayPath(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
