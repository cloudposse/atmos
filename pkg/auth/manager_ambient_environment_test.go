package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_AWSAmbientEnvironmentOverrides(t *testing.T) {
	config := &schema.AuthConfig{
		Realm: "test-realm",
		Identities: map[string]schema.Identity{
			"localstack": {
				Kind: "aws/ambient",
				Env: []schema.EnvironmentVariable{
					{Key: "FOO", Value: "bar"},
					{Key: "AWS_ACCESS_KEY_ID", Value: "123456789012"},
				},
			},
		},
	}
	authManager, err := NewAuthManager(config, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)

	t.Run("auth env exports configured variables without authentication", func(t *testing.T) {
		env, err := authManager.GetEnvironmentVariables("localstack")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"FOO": "bar", "AWS_ACCESS_KEY_ID": "123456789012",
		}, env)
	})

	t.Run("subprocess environment overrides inherited credentials", func(t *testing.T) {
		input := []string{
			"FOO=inherited", "AWS_ACCESS_KEY_ID=test", "AWS_SECRET_ACCESS_KEY=test",
			"AWS_ENDPOINT_URL=http://shell:4566", "SHELL_ONLY=preserved",
		}
		// Match auth exec and auth shell: global Atmos env overrides the shell first.
		base := envpkg.MergeGlobalEnv(input, map[string]string{
			"FOO": "global", "AWS_ACCESS_KEY_ID": "global",
			"AWS_ENDPOINT_URL": "http://localhost:4566", "GLOBAL_ONLY": "preserved",
		})
		env, err := authManager.PrepareShellEnvironment(context.Background(), "localstack", base)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{
			"FOO=bar", "AWS_ACCESS_KEY_ID=123456789012", "AWS_SECRET_ACCESS_KEY=test",
			"AWS_ENDPOINT_URL=http://localhost:4566", "SHELL_ONLY=preserved", "GLOBAL_ONLY=preserved",
		}, env)
		assert.Equal(t, "AWS_ACCESS_KEY_ID=test", input[1])
	})
}

func TestAWSAmbientHookEnvironmentPrecedence(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("FOO", "shell")
	config := &schema.AuthConfig{
		Realm: "test-ambient-env-hook",
		Identities: map[string]schema.Identity{
			"localstack": {
				Kind: "aws/ambient",
				Env: []schema.EnvironmentVariable{
					{Key: "FOO", Value: "identity"},
					{Key: "EMPTY", Value: ""},
					{Key: "AWS_ACCESS_KEY_ID", Value: "123456789012"},
				},
			},
		},
	}
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	authManager, err := NewAuthManager(config, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)
	atmosConfig := &schema.AtmosConfiguration{Env: map[string]string{"FOO": "global", "EMPTY": "global"}}
	// Stack processing has already merged global env into the component section.
	stack := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: schema.AtmosSectionMapType{"FOO": "global", "EMPTY": "global"},
	}
	err = authenticateAndWriteEnv(context.Background(), authManager, "localstack", atmosConfig, stack)
	require.NoError(t, err)
	assert.Equal(t, "identity", stack.ComponentEnvSection["FOO"])
	assert.Equal(t, "", stack.ComponentEnvSection["EMPTY"])
	assert.Equal(t, "123456789012", stack.ComponentEnvSection["AWS_ACCESS_KEY_ID"])
	assert.Contains(t, stack.SanitizedEnv, "FOO=identity")
	assert.Contains(t, stack.SanitizedEnv, "EMPTY=")
}
