package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultEnvironmentSetup_SetupEnvironment(t *testing.T) {
	tests := []struct {
		name                  string
		config                *ComponentConfig
		authContext           *schema.AuthContext
		envSetup              map[string]string
		expectError           bool
		expectedEnvContains   map[string]string
		expectedEnvNotContain []string
	}{
		{
			name: "basic environment with no auth context",
			config: &ComponentConfig{
				Env: nil,
			},
			authContext:         nil,
			expectError:         false,
			expectedEnvContains: nil,
		},
		{
			name: "component env variables added",
			config: &ComponentConfig{
				Env: map[string]any{
					"MY_VAR":      "my-value",
					"ANOTHER_VAR": "another-value",
					"NUMERIC_VAR": 123,
					"BOOL_VAR":    true,
				},
			},
			authContext: nil,
			expectError: false,
			expectedEnvContains: map[string]string{
				"MY_VAR":      "my-value",
				"ANOTHER_VAR": "another-value",
				"NUMERIC_VAR": "123",
				"BOOL_VAR":    "true",
			},
		},
		{
			name: "with AWS auth context",
			config: &ComponentConfig{
				Env: map[string]any{
					"APP_ENV": "production",
				},
			},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile:         "my-profile",
					Region:          "us-west-2",
					CredentialsFile: "/path/to/credentials",
					ConfigFile:      "/path/to/config",
				},
			},
			expectError: false,
			expectedEnvContains: map[string]string{
				"APP_ENV":                     "production",
				"AWS_PROFILE":                 "my-profile",
				"AWS_DEFAULT_REGION":          "us-west-2",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_CONFIG_FILE":             "/path/to/config",
			},
		},
		{
			name: "with partial AWS auth context",
			config: &ComponentConfig{
				Env: nil,
			},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "minimal-profile",
				},
			},
			expectError: false,
			expectedEnvContains: map[string]string{
				"AWS_PROFILE": "minimal-profile",
			},
		},
		{
			name: "nil AWS in auth context",
			config: &ComponentConfig{
				Env: map[string]any{
					"TEST_VAR": "test-value",
				},
			},
			authContext: &schema.AuthContext{
				AWS: nil,
			},
			expectError: false,
			expectedEnvContains: map[string]string{
				"TEST_VAR": "test-value",
			},
		},
		{
			name: "empty env map",
			config: &ComponentConfig{
				Env: map[string]any{},
			},
			authContext: nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := &defaultEnvironmentSetup{}
			result, err := setup.SetupEnvironment(tt.config, tt.authContext)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify expected env vars are present.
			for key, expectedValue := range tt.expectedEnvContains {
				actualValue, ok := result[key]
				assert.True(t, ok, "expected key %s to be present", key)
				assert.Equal(t, expectedValue, actualValue, "value mismatch for key %s", key)
			}

			// Verify prohibited env vars are not present.
			for _, key := range tt.expectedEnvNotContain {
				_, ok := result[key]
				assert.False(t, ok, "key %s should not be present", key)
			}
		})
	}
}

func TestDefaultEnvironmentSetup_ProhibitedVarsFiltered(t *testing.T) {
	// Set prohibited environment variables.
	for _, key := range []string{
		"TF_CLI_ARGS",
		"TF_INPUT",
		"TF_WORKSPACE",
		"TF_VAR_test_var",
		"TF_CLI_ARGS_init",
	} {
		t.Setenv(key, "should-be-filtered")
	}

	// Set a normal variable that should pass through.
	t.Setenv("ALLOWED_VAR", "allowed-value")

	setup := &defaultEnvironmentSetup{}
	config := &ComponentConfig{
		Env: nil,
	}

	result, err := setup.SetupEnvironment(config, nil)
	require.NoError(t, err)

	// Verify prohibited vars are filtered.
	prohibitedVars := []string{
		"TF_CLI_ARGS",
		"TF_INPUT",
		"TF_WORKSPACE",
		"TF_VAR_test_var",
		"TF_CLI_ARGS_init",
	}
	for _, key := range prohibitedVars {
		_, ok := result[key]
		assert.False(t, ok, "prohibited var %s should be filtered", key)
	}

	// Verify allowed var passes through.
	val, ok := result["ALLOWED_VAR"]
	assert.True(t, ok, "ALLOWED_VAR should be present")
	assert.Equal(t, "allowed-value", val)
}

func TestDefaultEnvironmentSetup_ComponentEnvOverridesParent(t *testing.T) {
	// Set a parent environment variable.
	t.Setenv("MY_VAR", "parent-value")

	setup := &defaultEnvironmentSetup{}
	config := &ComponentConfig{
		Env: map[string]any{
			"MY_VAR": "component-value",
		},
	}

	result, err := setup.SetupEnvironment(config, nil)
	require.NoError(t, err)

	// Component env should override parent.
	val, ok := result["MY_VAR"]
	assert.True(t, ok, "MY_VAR should be present")
	assert.Equal(t, "component-value", val, "component env should override parent env")
}
