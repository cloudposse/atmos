package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// Note: parseProfilesFromArgs is already tested in load_profile_test.go

func TestGetProfilesFromFlagsOrEnv(t *testing.T) {
	tests := []struct {
		name             string
		setupViper       func()
		osArgs           []string
		expectedProfiles []string
		expectedSource   string
	}{
		{
			name: "profiles from environment variable",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"env-profile1", "env-profile2"})
			},
			osArgs:           []string{"atmos", "describe", "config"},
			expectedProfiles: []string{"env-profile1", "env-profile2"},
			expectedSource:   "env",
		},
		{
			name: "profiles from CLI flag --profile value syntax",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", nil) // Ensure viper doesn't have profile set
			},
			osArgs:           []string{"atmos", "describe", "config", "--profile", "cli-profile"},
			expectedProfiles: []string{"cli-profile"},
			expectedSource:   "flag",
		},
		{
			name: "profiles from CLI flag --profile=value syntax",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", nil)
			},
			osArgs:           []string{"atmos", "describe", "config", "--profile=cli-profile"},
			expectedProfiles: []string{"cli-profile"},
			expectedSource:   "flag",
		},
		{
			name: "no profiles specified",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", nil)
			},
			osArgs:           []string{"atmos", "describe", "config"},
			expectedProfiles: nil,
			expectedSource:   "",
		},
		{
			name: "viper has empty slice",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{})
			},
			osArgs:           []string{"atmos", "describe", "config"},
			expectedProfiles: nil,
			expectedSource:   "",
		},
		{
			name: "environment variable takes precedence over CLI flag",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"env-profile"})
			},
			osArgs:           []string{"atmos", "describe", "config", "--profile", "cli-profile"},
			expectedProfiles: []string{"env-profile"},
			expectedSource:   "env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupViper()

			// Save original os.Args and restore after test
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()
			os.Args = tt.osArgs

			// Execute
			profiles, source := getProfilesFromFlagsOrEnv()

			// Assert
			assert.Equal(t, tt.expectedProfiles, profiles, "profiles should match")
			assert.Equal(t, tt.expectedSource, source, "source should match")
		})
	}
}
