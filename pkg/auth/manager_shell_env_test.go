package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// TestEnvironListToMap tests conversion from environment variable list to map.
func TestEnvironListToMap(t *testing.T) {
	tests := []struct {
		name     string
		envList  []string
		expected map[string]string
	}{
		{
			name:     "empty list",
			envList:  []string{},
			expected: map[string]string{},
		},
		{
			name:    "single variable",
			envList: []string{"KEY=value"},
			expected: map[string]string{
				"KEY": "value",
			},
		},
		{
			name:    "multiple variables",
			envList: []string{"FOO=bar", "BAZ=qux", "NUM=123"},
			expected: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
				"NUM": "123",
			},
		},
		{
			name:    "value with equals sign",
			envList: []string{"KEY=value=with=equals"},
			expected: map[string]string{
				"KEY": "value=with=equals",
			},
		},
		{
			name:    "empty value",
			envList: []string{"EMPTY="},
			expected: map[string]string{
				"EMPTY": "",
			},
		},
		{
			name:    "value with spaces",
			envList: []string{"SPACED=value with spaces"},
			expected: map[string]string{
				"SPACED": "value with spaces",
			},
		},
		{
			name:     "malformed entry without equals",
			envList:  []string{"MALFORMED"},
			expected: map[string]string{},
		},
		{
			name:    "mixed valid and invalid",
			envList: []string{"VALID=value", "INVALID", "ALSO_VALID=123"},
			expected: map[string]string{
				"VALID":      "value",
				"ALSO_VALID": "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := environListToMap(tt.envList)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMapToEnvironList tests conversion from map to environment variable list.
func TestMapToEnvironList(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		validate func(t *testing.T, result []string)
	}{
		{
			name:   "empty map",
			envMap: map[string]string{},
			validate: func(t *testing.T, result []string) {
				assert.Empty(t, result)
			},
		},
		{
			name: "single entry",
			envMap: map[string]string{
				"KEY": "value",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 1)
				assert.Contains(t, result, "KEY=value")
			},
		},
		{
			name: "multiple entries",
			envMap: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
				"NUM": "123",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 3)
				assert.Contains(t, result, "FOO=bar")
				assert.Contains(t, result, "BAZ=qux")
				assert.Contains(t, result, "NUM=123")
			},
		},
		{
			name: "value with special characters",
			envMap: map[string]string{
				"PATH":    "/usr/bin:/usr/local/bin",
				"SPECIAL": "value=with=equals",
				"SPACED":  "value with spaces",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 3)
				assert.Contains(t, result, "PATH=/usr/bin:/usr/local/bin")
				assert.Contains(t, result, "SPECIAL=value=with=equals")
				assert.Contains(t, result, "SPACED=value with spaces")
			},
		},
		{
			name: "empty value",
			envMap: map[string]string{
				"EMPTY": "",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 1)
				assert.Contains(t, result, "EMPTY=")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToEnvironList(tt.envMap)
			tt.validate(t, result)
		})
	}
}

// TestEnvironListToMapRoundTrip tests that converting list -> map -> list preserves values.
func TestEnvironListToMapRoundTrip(t *testing.T) {
	original := []string{
		"FOO=bar",
		"BAZ=qux",
		"PATH=/usr/bin:/usr/local/bin",
		"SPECIAL=value=with=equals",
		"EMPTY=",
	}

	// Convert to map and back to list.
	envMap := environListToMap(original)
	result := mapToEnvironList(envMap)

	// Should have same number of entries.
	assert.Len(t, result, len(original))

	// All original entries should be present in result.
	for _, entry := range original {
		assert.Contains(t, result, entry)
	}
}

// mockIdentityForShellEnv is a test mock that implements types.Identity.
type mockIdentityForShellEnv struct {
	prepareEnvFunc func(ctx context.Context, environ map[string]string) (map[string]string, error)
}

func (m *mockIdentityForShellEnv) Kind() string { return "mock" }
func (m *mockIdentityForShellEnv) GetProviderName() (string, error) {
	return "mock-provider", nil
}

func (m *mockIdentityForShellEnv) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	return nil, errors.New("not implemented")
}

func (m *mockIdentityForShellEnv) Validate() error { return nil }

func (m *mockIdentityForShellEnv) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *mockIdentityForShellEnv) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	if m.prepareEnvFunc != nil {
		return m.prepareEnvFunc(ctx, environ)
	}
	return environ, nil
}

func (m *mockIdentityForShellEnv) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	return nil, errors.New("not implemented")
}

func (m *mockIdentityForShellEnv) Logout(ctx context.Context) error { return nil }

func (m *mockIdentityForShellEnv) GetFilesDisplayPath() string { return "" }

func (m *mockIdentityForShellEnv) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	return nil
}

func (m *mockIdentityForShellEnv) CredentialsExist() (bool, error) {
	return false, nil
}

func (m *mockIdentityForShellEnv) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// TestPrepareShellEnvironment_Success tests successful environment preparation.
func TestPrepareShellEnvironment_Success(t *testing.T) {
	// Create mock identity that implements PrepareEnvironment.
	mockIdentity := &mockIdentityForShellEnv{
		prepareEnvFunc: func(ctx context.Context, environ map[string]string) (map[string]string, error) {
			// Add auth-specific env vars.
			result := make(map[string]string)
			for k, v := range environ {
				result[k] = v
			}
			result["AWS_SHARED_CREDENTIALS_FILE"] = "/path/to/credentials"
			result["AWS_CONFIG_FILE"] = "/path/to/config"
			result["AWS_PROFILE"] = "test-profile"
			result["AWS_REGION"] = "us-east-1"
			// Clear conflicting vars.
			delete(result, "AWS_ACCESS_KEY_ID")
			delete(result, "AWS_SECRET_ACCESS_KEY")
			return result, nil
		},
	}

	m := &manager{
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	inputEnv := []string{
		"PATH=/usr/bin",
		"HOME=/home/user",
		"AWS_ACCESS_KEY_ID=old-key",        // Should be removed.
		"AWS_SECRET_ACCESS_KEY=old-secret", // Should be removed.
	}

	result, err := m.PrepareShellEnvironment(context.Background(), "test-identity", inputEnv)
	require.NoError(t, err)

	// Convert result to map for easier validation.
	resultMap := environListToMap(result)

	// Verify auth vars were added.
	assert.Equal(t, "/path/to/credentials", resultMap["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, "/path/to/config", resultMap["AWS_CONFIG_FILE"])
	assert.Equal(t, "test-profile", resultMap["AWS_PROFILE"])
	assert.Equal(t, "us-east-1", resultMap["AWS_REGION"])

	// Verify original vars preserved.
	assert.Equal(t, "/usr/bin", resultMap["PATH"])
	assert.Equal(t, "/home/user", resultMap["HOME"])

	// Verify conflicting vars removed.
	assert.NotContains(t, resultMap, "AWS_ACCESS_KEY_ID")
	assert.NotContains(t, resultMap, "AWS_SECRET_ACCESS_KEY")
}

// TestPrepareShellEnvironment_IdentityNotFound tests error when identity doesn't exist.
func TestPrepareShellEnvironment_IdentityNotFound(t *testing.T) {
	m := &manager{
		identities: map[string]types.Identity{},
	}

	_, err := m.PrepareShellEnvironment(context.Background(), "nonexistent", []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestPrepareShellEnvironment_PrepareEnvironmentError tests error propagation.
func TestPrepareShellEnvironment_PrepareEnvironmentError(t *testing.T) {
	mockIdentity := &mockIdentityForShellEnv{
		prepareEnvFunc: func(ctx context.Context, environ map[string]string) (map[string]string, error) {
			return nil, errors.New("prepare failed")
		},
	}

	m := &manager{
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	_, err := m.PrepareShellEnvironment(context.Background(), "test-identity", []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prepare failed")
}

// TestPrepareShellEnvironment_EmptyInput tests handling of empty input.
func TestPrepareShellEnvironment_EmptyInput(t *testing.T) {
	mockIdentity := &mockIdentityForShellEnv{
		prepareEnvFunc: func(ctx context.Context, environ map[string]string) (map[string]string, error) {
			result := make(map[string]string)
			result["AWS_PROFILE"] = "test"
			return result, nil
		},
	}

	m := &manager{
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	result, err := m.PrepareShellEnvironment(context.Background(), "test-identity", []string{})
	require.NoError(t, err)

	resultMap := environListToMap(result)
	assert.Equal(t, "test", resultMap["AWS_PROFILE"])
}

// TestPrepareShellEnvironment_PreservesValues tests that environment variables are preserved.
func TestPrepareShellEnvironment_PreservesValues(t *testing.T) {
	mockIdentity := &mockIdentityForShellEnv{
		prepareEnvFunc: func(ctx context.Context, environ map[string]string) (map[string]string, error) {
			// Just pass through with one addition.
			result := make(map[string]string)
			for k, v := range environ {
				result[k] = v
			}
			result["NEW_VAR"] = "new_value"
			return result, nil
		},
	}

	m := &manager{
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	inputEnv := []string{
		"EXISTING1=value1",
		"EXISTING2=value2",
		"EXISTING3=value3",
	}

	result, err := m.PrepareShellEnvironment(context.Background(), "test-identity", inputEnv)
	require.NoError(t, err)

	resultMap := environListToMap(result)

	// All existing vars should be present.
	assert.Equal(t, "value1", resultMap["EXISTING1"])
	assert.Equal(t, "value2", resultMap["EXISTING2"])
	assert.Equal(t, "value3", resultMap["EXISTING3"])
	assert.Equal(t, "new_value", resultMap["NEW_VAR"])
	assert.Len(t, resultMap, 4)
}
