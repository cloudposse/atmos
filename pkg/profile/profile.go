package profile

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProfileInfo contains information about a discovered profile.
type ProfileInfo struct {
	Name        string              // Profile name (directory name).
	Path        string              // Absolute path to profile directory.
	LocationType string             // Location type: "configurable", "project-hidden", "xdg", "project".
	Files       []string            // List of configuration files in the profile.
	Metadata    *schema.ConfigMetadata // Optional metadata from the profile.
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
	return &DefaultProfileManager{}
}
