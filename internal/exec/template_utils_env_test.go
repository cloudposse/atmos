package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExtractEnvFromRawMap tests extracting env vars from raw map[string]any.
func TestExtractEnvFromRawMap(t *testing.T) {
	t.Run("nil map returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(nil)
		assert.Nil(t, result)
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{})
		assert.Nil(t, result)
	})

	t.Run("missing templates key returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"other": "value",
		})
		assert.Nil(t, result)
	})

	t.Run("missing settings key returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"other": "value",
			},
		})
		assert.Nil(t, result)
	})

	t.Run("missing env key returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"enabled": true,
				},
			},
		})
		assert.Nil(t, result)
	})

	t.Run("extracts env from map[string]any", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": map[string]any{
						"AWS_PROFILE": "production",
						"AWS_REGION":  "us-east-1",
					},
				},
			},
		})
		require.NotNil(t, result)
		assert.Equal(t, "production", result["AWS_PROFILE"])
		assert.Equal(t, "us-east-1", result["AWS_REGION"])
	})

	t.Run("extracts env from map[string]string", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": map[string]string{
						"AWS_PROFILE": "staging",
					},
				},
			},
		})
		require.NotNil(t, result)
		assert.Equal(t, "staging", result["AWS_PROFILE"])
	})

	t.Run("skips non-string values in map[string]any", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": map[string]any{
						"VALID_KEY":   "valid",
						"INVALID_KEY": 123,
					},
				},
			},
		})
		require.NotNil(t, result)
		assert.Equal(t, "valid", result["VALID_KEY"])
		assert.NotContains(t, result, "INVALID_KEY")
	})

	t.Run("unsupported env type returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": []string{"not", "a", "map"},
				},
			},
		})
		assert.Nil(t, result, "unsupported type should return nil")
	})

	t.Run("empty env map returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": map[string]any{},
				},
			},
		})
		assert.Nil(t, result, "empty env map should return nil")
	})

	t.Run("all non-string values returns nil", func(t *testing.T) {
		result := extractEnvFromRawMap(map[string]any{
			"templates": map[string]any{
				"settings": map[string]any{
					"env": map[string]any{
						"KEY1": 42,
						"KEY2": true,
					},
				},
			},
		})
		assert.Nil(t, result, "env map with only non-string values should return nil")
	})
}

// TestSetEnvVarsWithRestore tests that env vars are set and restored correctly.
func TestSetEnvVarsWithRestore(t *testing.T) {
	t.Run("sets env vars and restores on cleanup", func(t *testing.T) {
		// Set an existing var that will be overwritten.
		t.Setenv("TEST_EXISTING_VAR", "original_value")

		// Ensure a var that doesn't exist.
		os.Unsetenv("TEST_NEW_VAR")

		envVars := map[string]string{
			"TEST_EXISTING_VAR": "new_value",
			"TEST_NEW_VAR":      "created_value",
		}

		cleanup, err := setEnvVarsWithRestore(envVars)
		require.NoError(t, err)

		// Verify vars are set.
		assert.Equal(t, "new_value", os.Getenv("TEST_EXISTING_VAR"))
		assert.Equal(t, "created_value", os.Getenv("TEST_NEW_VAR"))

		// Run cleanup.
		cleanup()

		// Verify original var is restored.
		assert.Equal(t, "original_value", os.Getenv("TEST_EXISTING_VAR"))

		// Verify new var is removed.
		_, exists := os.LookupEnv("TEST_NEW_VAR")
		assert.False(t, exists, "TEST_NEW_VAR should be unset after cleanup")
	})

	t.Run("empty map produces no-op cleanup", func(t *testing.T) {
		cleanup, err := setEnvVarsWithRestore(map[string]string{})
		require.NoError(t, err)
		cleanup() // Should not panic.
	})
}

// TestProcessTmplWithDatasources_EnvVarsFromConfig tests that env vars configured
// in atmosConfig.Templates.Settings.Env are properly set during template processing.
func TestProcessTmplWithDatasources_EnvVarsFromConfig(t *testing.T) {
	// Use t.Setenv for automatic restore on test cleanup, then unset for clean state.
	t.Setenv("TEST_GOMPLATE_AWS_PROFILE", "")
	os.Unsetenv("TEST_GOMPLATE_AWS_PROFILE")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				Env: map[string]string{
					"TEST_GOMPLATE_AWS_PROFILE": "my-profile",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	// Template that reads the env var using Sprig's env function.
	tmplValue := `
config:
  profile: '{{ env "TEST_GOMPLATE_AWS_PROFILE" }}'
`

	tmplData := map[string]any{}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-env-from-config",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "my-profile", "env var from atmosConfig should be available in template")

	// Verify cleanup: the env var should be restored to its original state (unset).
	_, exists := os.LookupEnv("TEST_GOMPLATE_AWS_PROFILE")
	assert.False(t, exists, "TEST_GOMPLATE_AWS_PROFILE should be unset after template processing")
}

// TestProcessTmplWithDatasources_EnvVarsFromStackManifest tests that env vars from
// stack manifest settings override those from CLI config.
func TestProcessTmplWithDatasources_EnvVarsFromStackManifest(t *testing.T) {
	t.Setenv("TEST_GOMPLATE_REGION", "")
	os.Unsetenv("TEST_GOMPLATE_REGION")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				Env: map[string]string{
					"TEST_GOMPLATE_REGION": "us-east-1",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Stack manifest overrides the CLI config env var.
	settingsSection := schema.Settings{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Env: map[string]string{
					"TEST_GOMPLATE_REGION": "eu-west-1",
				},
			},
		},
	}

	tmplValue := `
config:
  region: '{{ env "TEST_GOMPLATE_REGION" }}'
`

	tmplData := map[string]any{}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-env-stack-override",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "eu-west-1", "stack manifest env should override CLI config env")
	assert.NotContains(t, result, "us-east-1", "CLI config env should be overridden by stack manifest")
}

// TestProcessTmplWithDatasources_EnvVarsCleanedUp tests that env vars are properly
// restored after template processing, preventing pollution across components.
func TestProcessTmplWithDatasources_EnvVarsCleanedUp(t *testing.T) {
	// Set an env var that will be overridden during template processing.
	t.Setenv("TEST_GOMPLATE_CLEANUP", "original")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				Env: map[string]string{
					"TEST_GOMPLATE_CLEANUP": "overridden",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	tmplValue := `
config:
  value: '{{ env "TEST_GOMPLATE_CLEANUP" }}'
`

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-env-cleanup",
		tmplValue,
		map[string]any{},
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "overridden", "env var should be overridden during processing")

	// After processing, the original value should be restored.
	assert.Equal(t, "original", os.Getenv("TEST_GOMPLATE_CLEANUP"),
		"env var should be restored to original value after processing")
}

// TestProcessTmplWithDatasources_DisabledTemplating tests that when templating is
// disabled, env vars are not set and the template is returned unchanged.
func TestProcessTmplWithDatasources_DisabledTemplating(t *testing.T) {
	t.Setenv("TEST_GOMPLATE_DISABLED", "")
	os.Unsetenv("TEST_GOMPLATE_DISABLED")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: false, // Templating disabled.
				Env: map[string]string{
					"TEST_GOMPLATE_DISABLED": "should-not-be-set",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	tmplValue := `config: '{{ env "TEST_GOMPLATE_DISABLED" }}'`

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-disabled",
		tmplValue,
		map[string]any{},
		true,
	)

	require.NoError(t, err)
	// Template should be returned unchanged.
	assert.Equal(t, tmplValue, result)

	// Env var should NOT have been set.
	_, exists := os.LookupEnv("TEST_GOMPLATE_DISABLED")
	assert.False(t, exists, "env var should not be set when templating is disabled")
}

// TestProcessTmplWithDatasources_NoEnvVars tests that processing works correctly
// when no env vars are configured.
func TestProcessTmplWithDatasources_NoEnvVars(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				// No Env field set.
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	tmplValue := `
config:
  value: '{{ .name }}'
`
	tmplData := map[string]any{"name": "test"}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-no-env",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test")
}

// TestProcessTmplWithDatasources_EnvVarsInEvaluationLoop tests that env vars are
// properly re-extracted when template settings are re-decoded in the evaluation loop.
func TestProcessTmplWithDatasources_EnvVarsInEvaluationLoop(t *testing.T) {
	t.Setenv("TEST_EVAL_LOOP_VAR", "")
	os.Unsetenv("TEST_EVAL_LOOP_VAR")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:     true,
				Evaluations: 2, // Multiple evaluations.
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				Env: map[string]string{
					"TEST_EVAL_LOOP_VAR": "eval-value",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	// Template with settings section that will be re-decoded in the evaluation loop.
	tmplValue := `
settings:
  templates:
    settings:
      env:
        TEST_EVAL_LOOP_VAR: "eval-value"
config:
  value: '{{ env "TEST_EVAL_LOOP_VAR" }}'
`

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-eval-loop",
		tmplValue,
		map[string]any{},
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "eval-value", "env var should be available across evaluation passes")
}

// TestProcessTmplWithDatasources_EnvVarsCaseSensitive tests that env var keys
// preserve their original case (e.g., AWS_PROFILE stays uppercase).
func TestProcessTmplWithDatasources_EnvVarsCaseSensitive(t *testing.T) {
	t.Setenv("AWS_TEST_CASE_KEY", "")
	os.Unsetenv("AWS_TEST_CASE_KEY")

	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
				Env: map[string]string{
					"AWS_TEST_CASE_KEY": "test-value",
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	tmplValue := `
config:
  key: '{{ env "AWS_TEST_CASE_KEY" }}'
`

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-env-case-sensitive",
		tmplValue,
		map[string]any{},
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test-value", "case-sensitive env var key should work correctly")
}
