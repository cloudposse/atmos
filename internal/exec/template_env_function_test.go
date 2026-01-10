package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessTmplWithDatasources_EnvFunction tests that the Sprig `env` template function
// correctly retrieves environment variables when they are set.
//
// The `env` function is provided by Sprig (github.com/Masterminds/sprig/v3) and simply
// calls os.Getenv() to retrieve environment variable values.
//
// This test verifies that templates like:
//
//	role_arn: 'arn:aws:iam::{{ env "ACCOUNT_ID" }}:role/{{ .vars.namespace }}-{{ env "ROLE_SUFFIX" }}'
//
// Work correctly when the environment variables are set.
func TestProcessTmplWithDatasources_EnvFunction(t *testing.T) {
	// Set up test environment variable
	t.Setenv("TEST_ACCOUNT_ID", "123456789012")
	t.Setenv("TEST_ROLE_SUFFIX", "terraform-backend")

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
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	// Template that uses env function similar to user's use case
	tmplValue := `
terraform:
  backend:
    s3:
      assume_role:
        role_arn: 'arn:aws:iam::{{ env "TEST_ACCOUNT_ID" }}:role/{{ .vars.namespace }}-{{ env "TEST_ROLE_SUFFIX" }}'
`

	tmplData := map[string]any{
		"vars": map[string]any{
			"namespace": "acme",
		},
	}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-template",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)

	// Check that env values are populated
	assert.Contains(t, result, "123456789012", "env TEST_ACCOUNT_ID should be populated")
	assert.Contains(t, result, "terraform-backend", "env TEST_ROLE_SUFFIX should be populated")
	assert.Contains(t, result, "acme", ".vars.namespace should be populated")

	// The full ARN should be correct
	assert.Contains(t, result, "arn:aws:iam::123456789012:role/acme-terraform-backend")

	t.Log("Result:", result)
}

// TestProcessTmplWithDatasources_EnvFunction_UnsetEnvVar tests the expected behavior when
// environment variables are NOT set.
//
// IMPORTANT: When an environment variable is not set, Sprig's `env` function returns an
// empty string. This is the standard behavior of os.Getenv() and is NOT a bug.
//
// This test documents the behavior reported in GitHub issue where users see templates like:
//
//	arn:aws:iam::{{ env "ACCOUNT_ID" }}:role/...
//
// Rendered as:
//
//	arn:aws:iam:::role/...
//
// The solution is to ensure the required environment variables are set before running
// atmos commands. The `env` template function reads from the OS environment at the time
// the template is processed.
func TestProcessTmplWithDatasources_EnvFunction_UnsetEnvVar(t *testing.T) {
	// Intentionally NOT setting the env vars to test what happens when they're missing.
	// This demonstrates the expected behavior - NOT a bug.

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
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	settingsSection := schema.Settings{}

	// Template that uses env function - env vars NOT set
	tmplValue := `
terraform:
  backend:
    s3:
      assume_role:
        role_arn: 'arn:aws:iam::{{ env "UNSET_ACCOUNT_ID" }}:role/{{ .vars.namespace }}-{{ env "UNSET_ROLE_SUFFIX" }}'
`

	tmplData := map[string]any{
		"vars": map[string]any{
			"namespace": "acme",
		},
	}

	result, err := ProcessTmplWithDatasources(
		atmosConfig,
		configAndStacksInfo,
		settingsSection,
		"test-template",
		tmplValue,
		tmplData,
		true,
	)

	require.NoError(t, err)

	// When env var is not set, Sprig's env function returns empty string
	// This is the EXPECTED behavior - but matches the user's issue!
	// The result will be: 'arn:aws:iam:::role/acme-'
	t.Log("Result with unset env vars:", result)

	// This is what the user is experiencing:
	assert.Contains(t, result, "arn:aws:iam:::role/acme-", "env returns empty string when env var not set")
}
