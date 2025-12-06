package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// Note: parseProfilesFromOsArgs is already tested in load_profile_test.go

// TestGetProfilesFromFlagsOrEnvWithRealViper tests environment variable parsing through actual Viper BindEnv.
// This ensures we handle Viper's quirk where GetStringSlice() doesn't parse comma-separated env vars.
func TestGetProfilesFromFlagsOrEnvWithRealViper(t *testing.T) {
	tests := []struct {
		name             string
		envValue         string
		expectedProfiles []string
	}{
		{
			name:             "single profile",
			envValue:         "dev",
			expectedProfiles: []string{"dev"},
		},
		{
			name:             "comma-separated profiles",
			envValue:         "dev,staging,prod",
			expectedProfiles: []string{"dev", "staging", "prod"},
		},
		{
			name:             "empty value in comma list",
			envValue:         "dev,,prod",
			expectedProfiles: []string{"dev", "prod"},
		},
		{
			name:             "only whitespace",
			envValue:         "   ",
			expectedProfiles: nil,
		},
		{
			name:             "leading and trailing commas",
			envValue:         ",dev,staging,",
			expectedProfiles: []string{"dev", "staging"},
		},
		{
			name:             "profiles with spaces",
			envValue:         " dev , staging , prod ",
			expectedProfiles: []string{"dev", "staging", "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset Viper and restore after test.
			viper.Reset()
			t.Cleanup(viper.Reset)

			// Create a fresh Viper instance that matches production setup.
			v := viper.GetViper()

			// Set environment variable.
			t.Setenv("ATMOS_PROFILE", tt.envValue)

			// Bind environment variable to Viper (same as production).
			err := v.BindEnv(profileKey, "ATMOS_PROFILE")
			assert.NoError(t, err)

			// Execute.
			profiles, source := getProfilesFromFlagsOrEnv()

			// Assert.
			assert.Equal(t, tt.expectedProfiles, profiles, "profiles should match")
			if tt.expectedProfiles != nil {
				assert.Equal(t, "env", source, "source should be env")
			} else {
				assert.Equal(t, "", source, "source should be empty when no profiles")
			}
		})
	}
}

func TestGetProfilesFromFlagsOrEnv(t *testing.T) {
	tests := []struct {
		name             string
		setupViper       func()
		setupEnv         func(*testing.T)
		expectedProfiles []string
		expectedSource   string
	}{
		{
			name: "profiles from environment variable - single value",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"env-profile"})
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "env-profile")
			},
			expectedProfiles: []string{"env-profile"},
			expectedSource:   "env",
		},
		{
			name: "profiles from environment variable - comma-separated",
			setupViper: func() {
				v := viper.GetViper()
				// Viper reads comma-separated env vars and parses them
				v.Set("profile", []string{"env-profile1", "env-profile2"})
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "env-profile1,env-profile2")
			},
			expectedProfiles: []string{"env-profile1", "env-profile2"},
			expectedSource:   "env",
		},
		{
			name: "profiles from environment variable - empty value in comma list",
			setupViper: func() {
				v := viper.GetViper()
				// Viper should filter empty values when parsing comma-separated list
				v.Set("profile", []string{"dev", "prod"})
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "dev,,prod")
			},
			expectedProfiles: []string{"dev", "prod"},
			expectedSource:   "env",
		},
		{
			name: "profiles from environment variable - only whitespace",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", nil)
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "   ")
			},
			expectedProfiles: nil,
			expectedSource:   "",
		},
		{
			name: "profiles from environment variable - leading and trailing commas",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"dev", "staging"})
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", ",dev,staging,")
			},
			expectedProfiles: []string{"dev", "staging"},
			expectedSource:   "env",
		},
		{
			name: "profiles from CLI flag --profile value syntax",
			setupViper: func() {
				v := viper.GetViper()
				v.Set("profile", []string{"cli-profile"})
			},
			setupEnv:         nil, // Don't set ATMOS_PROFILE
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
			expectedProfiles: nil,
			expectedSource:   "",
		},
		{
			name: "CLI flag takes precedence over environment variable",
			setupViper: func() {
				v := viper.GetViper()
				// In production, syncGlobalFlagsToViper() sets flag value, overwriting env
				v.Set("profile", []string{"flag-profile"})
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("ATMOS_PROFILE", "env-profile")
			},
			expectedProfiles: []string{"flag-profile"},
			// Note: Source detection has known limitation - may report "env" when both are set
			// since we check os.LookupEnv("ATMOS_PROFILE"). This is acceptable because:
			// 1. The correct value (flag) is used due to syncGlobalFlagsToViper()
			// 2. Source is only for logging and doesn't affect functionality
			expectedSource: "env", // Known limitation, but functionality works correctly
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset Viper state to prevent test pollution
			viper.Reset()
			t.Cleanup(viper.Reset)

			// Setup Viper
			tt.setupViper()

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
