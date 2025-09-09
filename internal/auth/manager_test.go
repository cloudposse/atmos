package auth

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_GetDefaultIdentity(t *testing.T) {
	tests := []struct {
		name            string
		identities      map[string]schema.Identity
		isCI            bool
		expectedResult  string
		expectedError   string
		skipInteractive bool // Skip tests that require user interaction
	}{
		{
			name: "no default identities - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: false},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:          true,
			expectedError: "no default identity configured",
		},
		{
			name: "no default identities - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: false},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:            false,
			skipInteractive: true, // Skip because it requires user interaction
		},
		{
			name: "single default identity - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:           true,
			expectedResult: "identity1",
		},
		{
			name: "single default identity - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:           false,
			expectedResult: "identity1",
		},
		{
			name: "multiple default identities - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: true},
				"identity3": {Kind: "aws/user", Default: false},
			},
			isCI:          true,
			expectedError: "multiple default identities found: [identity1 identity2]",
		},
		{
			name: "multiple default identities - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: true},
				"identity3": {Kind: "aws/user", Default: false},
			},
			isCI:            false,
			skipInteractive: true, // Skip because it requires user interaction
		},
		{
			name:          "no identities at all - CI mode",
			identities:    map[string]schema.Identity{},
			isCI:          true,
			expectedError: "no default identity configured",
		},
		{
			name:            "no identities at all - interactive mode",
			identities:      map[string]schema.Identity{},
			isCI:            false,
			skipInteractive: true, // Skip because it requires user interaction
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipInteractive {
				t.Skipf("Skipping interactive test - requires user input.")
			}

			// Set up CI environment variable
			originalCI := os.Getenv("CI")
			if tt.isCI {
				os.Setenv("CI", "true")
			} else {
				os.Unsetenv("CI")
			}
			defer func() {
				if originalCI != "" {
					os.Setenv("CI", originalCI)
				} else {
					os.Unsetenv("CI")
				}
			}()

			// Create manager with test identities
			manager := &manager{
				config: &schema.AuthConfig{
					Identities: tt.identities,
				},
			}

			// Call the function
			result, err := manager.GetDefaultIdentity()

			// Assert results
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestManager_GetDefaultIdentity_MultipleDefaultsOrder(t *testing.T) {
	// Test that multiple defaults are returned in a consistent order
	identities := map[string]schema.Identity{
		"zebra":   {Kind: "aws/user", Default: true},
		"alpha":   {Kind: "aws/user", Default: true},
		"beta":    {Kind: "aws/user", Default: false},
		"charlie": {Kind: "aws/user", Default: true},
	}

	// Set CI mode to get deterministic error message
	os.Setenv("CI", "true")
	defer os.Unsetenv("CI")

	manager := &manager{
		config: &schema.AuthConfig{
			Identities: identities,
		},
	}

	_, err := manager.GetDefaultIdentity()
	require.Error(t, err)

	// The error should contain all three default identities
	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "multiple default identities found:")
	assert.Contains(t, errorMsg, "alpha")
	assert.Contains(t, errorMsg, "charlie")
	assert.Contains(t, errorMsg, "zebra")
	// Should not contain the non-default identity
	assert.NotContains(t, errorMsg, "beta")
}

func TestManager_ListIdentities(t *testing.T) {
	identities := map[string]schema.Identity{
		"identity1": {Kind: "aws/user", Default: true},
		"identity2": {Kind: "aws/user", Default: false},
		"identity3": {Kind: "aws/assume-role", Default: false},
	}

	manager := &manager{
		config: &schema.AuthConfig{
			Identities: identities,
		},
	}

	result := manager.ListIdentities()

	// Should return all identity names
	assert.Len(t, result, 3)
	assert.Contains(t, result, "identity1")
	assert.Contains(t, result, "identity2")
	assert.Contains(t, result, "identity3")
}

func TestManager_promptForIdentity(t *testing.T) {
	manager := &manager{}

	// Test with empty identities list
	_, err := manager.promptForIdentity("Choose identity:", []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no identities available")

	// Note: We can't easily test the interactive prompt without mocking huh.Form
	// In a real test environment, you might want to use dependency injection
	// to mock the form interaction
}
