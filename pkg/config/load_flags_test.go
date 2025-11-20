package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetProfilesFromFlagsOrEnv(t *testing.T) {
	tests := []struct {
		name             string
		setupViper       func()
		setupEnv         func(*testing.T)
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
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "env-profile1,env-profile2")
			},
			osArgs:           []string{"atmos", "describe", "config"},
			expectedProfiles: []string{"env-profile1", "env-profile2"},
			expectedSource:   "env",
		},
		{
			name: "profiles from CLI flag --profile value syntax",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"cli-profile"})
			},
			setupEnv:         nil, // Don't set ATMOS_PROFILE
			osArgs:           []string{"atmos", "describe", "config", "--profile", "cli-profile"},
			expectedProfiles: []string{"cli-profile"},
			expectedSource:   "flag",
		},
		{
			name: "profiles from CLI flag --profile=value syntax",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"cli-profile"})
			},
			setupEnv:         nil, // Don't set ATMOS_PROFILE
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
			setupEnv:         nil, // Don't set ATMOS_PROFILE
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
			setupEnv:         nil, // Don't set ATMOS_PROFILE
			osArgs:           []string{"atmos", "describe", "config"},
			expectedProfiles: nil,
			expectedSource:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupViper()

			// Save original os.Args and restore after test
			originalArgs := os.Args
			t.Cleanup(func() {
				os.Args = originalArgs
			})
			os.Args = tt.osArgs

			// Setup environment variables using t.Setenv for automatic cleanup
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			// Execute
			profiles, source := getProfilesFromFlagsOrEnv()

			// Assert
			assert.Equal(t, tt.expectedProfiles, profiles, "profiles should match")
			assert.Equal(t, tt.expectedSource, source, "source should match")
		})
	}
}
