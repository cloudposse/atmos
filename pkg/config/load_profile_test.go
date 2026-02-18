package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProfilesFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "--profile value syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "managers"},
			expected: []string{"managers"},
		},
		{
			name:     "--profile=value syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=managers"},
			expected: []string{"managers"},
		},
		{
			name:     "comma-separated values",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "multiple --profile flags",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev", AtmosProfileFlag, "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "no profile flag",
			args:     []string{"atmos", "describe", "config"},
			expected: nil,
		},
		{
			name:     "--profile at end without value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag},
			expected: nil,
		},
		{
			name:     "comma-separated with spaces",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev, staging , prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "comma-separated with --profile value syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev,staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "mixed syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev", AtmosProfileFlag + "=staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "empty value in comma list",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,,prod"},
			expected: []string{"dev", "prod"},
		},
		{
			name:     "only whitespace in profile value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=   "},
			expected: nil,
		},
		{
			name:     "leading and trailing commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=,dev,staging,"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "multiple consecutive commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,,,staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profile value with only commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=,,,"},
			expected: nil,
		},
		{
			name:     "mixed whitespace and empty values",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,  , , staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profile flag with equals but no value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "="},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromOsArgs(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetProfilesFromFlagsOrEnv_AwsProfileDoesNotAffect is a regression test
// for issue #2076. Verifies that AWS_PROFILE env var does not cause
// getProfilesFromFlagsOrEnv() to return the AWS profile name as an Atmos
// configuration profile.
func TestGetProfilesFromFlagsOrEnv_AwsProfileDoesNotAffect(t *testing.T) {
	// Save and reset the global Viper instance to prevent test interference.
	viper.Reset()

	// Step 1: Simulate global flag registration (what cmd/root.go init() does).
	// Bind "profile" key to ATMOS_PROFILE only.
	v := viper.GetViper()
	v.SetDefault("profile", nil)
	require.NoError(t, v.BindEnv("profile", "ATMOS_PROFILE"))

	// Step 2: Simulate EKS flag registration WITH prefix (the fix from #2076).
	// The prefix ensures "eks.profile" is bound to AWS_PROFILE, not "profile".
	v.SetDefault("eks.profile", "")
	require.NoError(t, v.BindEnv("eks.profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"))

	// Step 3: Set AWS_PROFILE but NOT ATMOS_PROFILE.
	t.Setenv("AWS_PROFILE", "my-aws-profile")
	// Ensure ATMOS_PROFILE is NOT set.
	os.Unsetenv("ATMOS_PROFILE")
	t.Cleanup(func() {
		os.Unsetenv("ATMOS_PROFILE")
	})

	// Step 4: Call getProfilesFromFlagsOrEnv - should NOT return AWS_PROFILE value.
	profiles, source := getProfilesFromFlagsOrEnv()
	assert.Empty(t, profiles,
		"getProfilesFromFlagsOrEnv should return no profiles when only AWS_PROFILE is set")
	assert.Empty(t, source,
		"source should be empty when no Atmos profiles are found")
}

// TestGetProfilesFromFlagsOrEnv_AtmosProfileStillWorks verifies that the fix
// for issue #2076 doesn't break ATMOS_PROFILE functionality.
func TestGetProfilesFromFlagsOrEnv_AtmosProfileStillWorks(t *testing.T) {
	// Save and reset.
	viper.Reset()

	// Simulate global flag registration.
	v := viper.GetViper()
	v.SetDefault("profile", nil)
	require.NoError(t, v.BindEnv("profile", "ATMOS_PROFILE"))

	// Simulate EKS parser with prefix.
	v.SetDefault("eks.profile", "")
	require.NoError(t, v.BindEnv("eks.profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"))

	// Set ATMOS_PROFILE.
	t.Setenv("ATMOS_PROFILE", "dev-config")

	// getProfilesFromFlagsOrEnv should return the ATMOS_PROFILE value.
	profiles, source := getProfilesFromFlagsOrEnv()
	assert.Contains(t, profiles, "dev-config",
		"getProfilesFromFlagsOrEnv should return ATMOS_PROFILE value")
	assert.Equal(t, "env", source,
		"source should be 'env' when profile comes from ATMOS_PROFILE")
}

// TestGetProfilesFromFallbacks_EnvVarFallback tests the getProfilesFromFallbacks
// function when ATMOS_PROFILE env var is set but os.Args has no --profile flag.
func TestGetProfilesFromFallbacks_EnvVarFallback(t *testing.T) {
	// Set ATMOS_PROFILE.
	t.Setenv("ATMOS_PROFILE", "fallback-profile")

	profiles, source := getProfilesFromFallbacks()
	assert.Contains(t, profiles, "fallback-profile",
		"getProfilesFromFallbacks should return ATMOS_PROFILE value")
	assert.Equal(t, "env", source,
		"source should be 'env' when profile comes from ATMOS_PROFILE fallback")
}

// TestGetProfilesFromFallbacks_NoProfile tests getProfilesFromFallbacks
// when neither os.Args nor ATMOS_PROFILE env var has a profile.
func TestGetProfilesFromFallbacks_NoProfile(t *testing.T) {
	// Ensure ATMOS_PROFILE is not set.
	os.Unsetenv("ATMOS_PROFILE")
	t.Cleanup(func() {
		os.Unsetenv("ATMOS_PROFILE")
	})

	profiles, source := getProfilesFromFallbacks()
	assert.Nil(t, profiles,
		"getProfilesFromFallbacks should return nil when no profile is available")
	assert.Empty(t, source,
		"source should be empty when no profile is available")
}

// TestGetProfilesFromFallbacks_EmptyEnvVar tests that an empty ATMOS_PROFILE
// env var is treated as unset.
func TestGetProfilesFromFallbacks_EmptyEnvVar(t *testing.T) {
	// Set ATMOS_PROFILE to empty string.
	t.Setenv("ATMOS_PROFILE", "")

	profiles, source := getProfilesFromFallbacks()
	assert.Nil(t, profiles,
		"getProfilesFromFallbacks should return nil for empty ATMOS_PROFILE")
	assert.Empty(t, source,
		"source should be empty for empty ATMOS_PROFILE")
}

// TestParseViperProfilesFromEnv_Quirks tests the parseViperProfilesFromEnv function
// with various Viper-quirk inputs.
func TestParseViperProfilesFromEnv_Quirks(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "single value",
			input:    []string{"dev"},
			expected: []string{"dev"},
		},
		{
			name:     "comma-separated in single element",
			input:    []string{"dev,staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "whitespace elements from Viper split",
			input:    []string{"dev", " ", "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "standalone commas from Viper split",
			input:    []string{"dev", ",", "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "empty strings filtered",
			input:    []string{"", "dev", ""},
			expected: []string{"dev"},
		},
		{
			name:     "all empty",
			input:    []string{"", " ", ","},
			expected: nil,
		},
		{
			name:     "mixed commas and values",
			input:    []string{"dev,staging", ",", "prod"},
			expected: []string{"dev", "staging", "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseViperProfilesFromEnv(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseProfilesFromEnvString_Delimiters tests the parseProfilesFromEnvString function
// with various delimiter patterns.
func TestParseProfilesFromEnvString_Delimiters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single value",
			input:    "dev",
			expected: []string{"dev"},
		},
		{
			name:     "comma-separated",
			input:    "dev,staging,prod",
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "with spaces",
			input:    "dev , staging , prod",
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "only commas",
			input:    ",,,",
			expected: nil,
		},
		{
			name:     "trailing comma",
			input:    "dev,staging,",
			expected: []string{"dev", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromEnvString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
