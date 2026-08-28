package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

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

		// Ensure a var that doesn't exist (register for cleanup first).
		t.Setenv("TEST_NEW_VAR", "")
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

// TestProcessTmplWithDatasources_DelimitersFromCLIConfig tests that custom delimiters
// configured only at the CLI (atmos.yaml) level are honored when the stack manifest doesn't
// set any of its own. Regression test: merge.Merge encodes both sides of the merge through
// mapstructure first, so an unset stack-level Delimiters (nil) becomes an explicit
// "delimiters: []" key that -- without a fallback -- wins the merge over the populated
// CLI-level value and silently reverts rendering to Go's default "{{"/"}}".
func TestProcessTmplWithDatasources_DelimitersFromCLIConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:     true,
				Delimiters:  []string{"[[", "]]"},
				Evaluations: 1,
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{} // No stack-level delimiters override.

	tmplValue := `
config:
  value: '[[ .name ]]'
`
	tmplData := map[string]any{"name": "test"}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-delimiters-cli",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test")
}

// TestProcessTmplWithDatasources_DelimitersFromStackManifest tests that stack manifest
// settings can override the CLI config's delimiters, per templates.settings.delimiters
// precedence (same override relationship already covered for Env above).
func TestProcessTmplWithDatasources_DelimitersFromStackManifest(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:    true,
				Delimiters: []string{"[[", "]]"},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Stack manifest overrides the CLI config's delimiters with a different pair.
	settingsSection := schema.Settings{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Delimiters: []string{"<<", ">>"},
			},
		},
	}

	tmplValue := `
config:
  value: '<< .name >>'
`
	tmplData := map[string]any{"name": "test"}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-delimiters-stack-override",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test")
}

// TestProcessTmplWithDatasources_DelimitersExplicitEmptyResetsToDefault tests that a stack
// manifest explicitly setting `delimiters: []` resets to Go's default "{{"/"}}" rather than
// falling back to the CLI config's custom delimiters. Loads the empty value through real YAML
// unmarshaling (not a Go literal) because yaml.v3 decodes `delimiters: []` into a non-nil,
// zero-length slice distinct from an absent key (which stays nil) -- the two must be
// distinguishable for "stack didn't set it, defer to CLI" vs "stack explicitly reset it" to work.
func TestProcessTmplWithDatasources_DelimitersExplicitEmptyResetsToDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:    true,
				Delimiters: []string{"[[", "]]"},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	var settingsSection schema.Settings
	err := yaml.Unmarshal([]byte(`
templates:
  settings:
    delimiters: []
`), &settingsSection)
	require.NoError(t, err)
	require.NotNil(t, settingsSection.Templates.Settings.Delimiters, "delimiters: [] must decode to a non-nil empty slice, not nil")
	require.Empty(t, settingsSection.Templates.Settings.Delimiters)

	tmplValue := `
config:
  value: '{{ .name }}'
`
	tmplData := map[string]any{"name": "test"}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-delimiters-explicit-reset",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test", "explicit delimiters: [] should render with Go's default {{ }}, not the CLI's [[ ]]")
}

// TestProcessTmplWithDatasources_DelimitersChangedMidEvaluationLoop tests that delimiters a
// first evaluation pass introduces via its own rendered "settings.templates.settings.delimiters"
// section are honored on the next pass, not silently reset to the original config. Regression
// test: effectiveDelimiters previously reset from settingsSection (a fixed, pre-loop value) on
// every pass instead of carrying a pass's own rendered delimiters forward.
func TestProcessTmplWithDatasources_DelimitersChangedMidEvaluationLoop(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:     true,
				Evaluations: 2, // Multiple evaluations: pass 1 introduces new delimiters, pass 2 must use them.
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	// Pass 1 uses the default "{{"/"}}" (nothing configured yet) to resolve `.name` and to
	// render the literal `delimiters: ["<<", ">>"]` section itself (no templating needed on
	// that section -- it's already the desired literal value). `<< .other >>` is untouched by
	// pass 1 (its delimiters don't match "<<"/">>") and must be resolved by pass 2 using the
	// newly-declared delimiters.
	tmplValue := `
settings:
  templates:
    settings:
      delimiters: ["<<", ">>"]
config:
  first: '{{ .name }}'
  second: '<< .other >>'
`
	tmplData := map[string]any{"name": "test-name", "other": "test-other"}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-delimiters-mid-loop-change",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)
	assert.Contains(t, result, "test-name", "pass 1 should resolve .name with the default {{ }}")
	assert.Contains(t, result, "test-other", "pass 2 should resolve << .other >> using the delimiters pass 1 declared")
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
