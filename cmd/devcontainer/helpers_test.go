package devcontainer

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListAvailableDevcontainers(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    []string
		expectError bool
	}{
		{
			name:        "nil atmosConfig",
			atmosConfig: nil,
			expectError: true,
		},
		{
			name: "nil devcontainer map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: nil,
				},
			},
			expectError: true,
		},
		{
			name: "empty devcontainer map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]interface{}{},
				},
			},
			expected:    nil, // Empty map returns nil slice, not empty slice
			expectError: false,
		},
		{
			name: "single devcontainer",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]interface{}{
						"geodesic": map[string]interface{}{},
					},
				},
			},
			expected:    []string{"geodesic"},
			expectError: false,
		},
		{
			name: "multiple devcontainers sorted alphabetically",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]interface{}{
						"terraform": map[string]interface{}{},
						"geodesic":  map[string]interface{}{},
						"python":    map[string]interface{}{},
					},
				},
			},
			expected:    []string{"geodesic", "python", "terraform"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := listAvailableDevcontainers(tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetDevcontainerName_WithArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "single argument provided",
			args:     []string{"geodesic"},
			expected: "geodesic",
		},
		{
			name:     "multiple arguments - returns first",
			args:     []string{"geodesic", "extra"},
			expected: "geodesic",
		},
		{
			name:     "argument with special characters",
			args:     []string{"my-dev-container"},
			expected: "my-dev-container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getDevcontainerName(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDevcontainerName_EmptyArgs(t *testing.T) {
	// This test verifies error handling when no args provided in non-interactive mode.
	// We can't test the interactive prompt without mocking or integration tests.
	t.Run("empty args in non-TTY environment", func(t *testing.T) {
		// When run in non-TTY (like in test environment), should return error.
		result, err := getDevcontainerName([]string{})
		// In test environments (non-TTY), we expect an error.
		// In interactive terminal, this would prompt the user.
		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "required in non-interactive mode")
	})

	t.Run("empty string in args", func(t *testing.T) {
		result, err := getDevcontainerName([]string{""})
		// Empty string should trigger same behavior as no args.
		require.Error(t, err)
		require.Empty(t, result)
	})
}

func TestDevcontainerNameCompletion(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		toComplete         string
		expectedDirective  cobra.ShellCompDirective
		expectCompletions  bool
		minCompletionCount int
	}{
		{
			name:              "no args - should provide completions",
			args:              []string{},
			toComplete:        "",
			expectedDirective: cobra.ShellCompDirectiveError,
			// In test environment, atmos.yaml cannot be loaded, so we expect error directive.
			// In real usage with valid atmos.yaml, this would return ShellCompDirectiveNoFileComp with completions.
		},
		{
			name:               "already has one arg - no more completions",
			args:               []string{"geodesic"},
			toComplete:         "",
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
			expectCompletions:  false,
			minCompletionCount: 0,
		},
		{
			name:               "already has two args - no completions",
			args:               []string{"geodesic", "extra"},
			toComplete:         "",
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
			expectCompletions:  false,
			minCompletionCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completions, directive := devcontainerNameCompletion(nil, tt.args, tt.toComplete)

			assert.Equal(t, tt.expectedDirective, directive)

			if !tt.expectCompletions {
				// Should return nil or empty when already have arg.
				assert.True(t, len(completions) == 0 || completions == nil)
			}

			if tt.minCompletionCount > 0 {
				assert.GreaterOrEqual(t, len(completions), tt.minCompletionCount)
			}
		})
	}
}

func TestPromptForDevcontainer_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		devcontainers []string
		expectError   bool
	}{
		{
			name:          "empty devcontainers list",
			message:       "Select a devcontainer:",
			devcontainers: []string{},
			expectError:   true,
		},
		{
			name:          "nil devcontainers list",
			message:       "Select a devcontainer:",
			devcontainers: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := promptForDevcontainer(tt.message, tt.devcontainers)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Can't test actual prompting without interactive terminal.
				// This would require integration tests or mocking huh.Form.
				t.Skip("Cannot test interactive prompt without TTY")
			}
		})
	}
}

// TestPromptForDevcontainer_Sorting verifies that we would pass sorted list to prompt.
// Actual prompting cannot be tested without mocking or integration tests.
func TestPromptForDevcontainer_Sorting(t *testing.T) {
	t.Run("devcontainers should be sorted before prompting", func(t *testing.T) {
		// We test listAvailableDevcontainers which provides sorted output.
		// The promptForDevcontainer receives pre-sorted list.
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Devcontainer: map[string]interface{}{
					"zulu":    map[string]interface{}{},
					"alpha":   map[string]interface{}{},
					"charlie": map[string]interface{}{},
				},
			},
		}

		devcontainers, err := listAvailableDevcontainers(atmosConfig)
		require.NoError(t, err)

		// Verify sorting.
		expected := []string{"alpha", "charlie", "zulu"}
		assert.Equal(t, expected, devcontainers)
	})
}

func TestSetAtmosConfig(t *testing.T) {
	t.Run("sets config successfully", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Components: schema.Components{
				Devcontainer: map[string]interface{}{
					"test": map[string]interface{}{},
				},
			},
		}

		SetAtmosConfig(config)
		assert.Equal(t, config, atmosConfigPtr)
	})

	t.Run("handles nil config", func(t *testing.T) {
		SetAtmosConfig(nil)
		assert.Nil(t, atmosConfigPtr)
	})
}

func TestDevcontainerCommandProvider(t *testing.T) {
	provider := &DevcontainerCommandProvider{}

	t.Run("GetCommand returns devcontainer command", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "devcontainer", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		name := provider.GetName()
		assert.Equal(t, "devcontainer", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		group := provider.GetGroup()
		assert.Equal(t, "Workflow Commands", group)
	})
}

func TestIsAuthConfigured(t *testing.T) {
	tests := []struct {
		name       string
		authConfig *schema.AuthConfig
		expected   bool
	}{
		{
			name:       "nil auth config",
			authConfig: nil,
			expected:   false,
		},
		{
			name: "empty identities",
			authConfig: &schema.AuthConfig{
				Identities: nil,
			},
			expected: false,
		},
		{
			name: "empty identities map",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{},
			},
			expected: false,
		},
		{
			name: "has identities",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"default": {},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAuthConfigured(tt.authConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateUnauthenticatedAuthManager(t *testing.T) {
	tests := []struct {
		name        string
		authConfig  *schema.AuthConfig
		expectError bool
	}{
		{
			name:        "nil auth config",
			authConfig:  nil,
			expectError: true,
		},
		{
			name: "empty identities",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{},
			},
			// Empty identities map is valid - no error expected.
			expectError: false,
		},
		{
			name: "with identities configured",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind:    "mock",
						Default: true,
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authManager, err := createUnauthenticatedAuthManager(tt.authConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, authManager)
			} else {
				require.NoError(t, err)
				require.NotNil(t, authManager)
			}
		})
	}
}
