package okta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrepareEnvironment_SetsOktaOrgURL(t *testing.T) {
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ: map[string]string{"OTHER": "value"},
		OrgURL:  "https://company.okta.com",
	})

	assert.Equal(t, "https://company.okta.com", result["OKTA_ORG_URL"])
	assert.Equal(t, "https://company.okta.com", result["OKTA_BASE_URL"])
	assert.Equal(t, "value", result["OTHER"])
}

func TestPrepareEnvironment_SetsAccessToken(t *testing.T) {
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ:     map[string]string{},
		OrgURL:      "https://company.okta.com",
		AccessToken: "test-access-token",
	})

	assert.Equal(t, "test-access-token", result["OKTA_OAUTH2_ACCESS_TOKEN"])
}

func TestPrepareEnvironment_SetsConfigDir(t *testing.T) {
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ:   map[string]string{},
		OrgURL:    "https://company.okta.com",
		ConfigDir: "/path/to/config",
	})

	assert.Equal(t, "/path/to/config", result["OKTA_CONFIG_DIR"])
}

func TestPrepareEnvironment_ClearsConflictingVars(t *testing.T) {
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ: map[string]string{
			"OKTA_API_TOKEN":           "should-be-cleared",
			"OKTA_CLIENT_ID":           "should-be-cleared",
			"OKTA_CLIENT_SECRET":       "should-be-cleared",
			"OKTA_PRIVATE_KEY":         "should-be-cleared",
			"OKTA_OAUTH2_ACCESS_TOKEN": "should-be-cleared",
			"OTHER":                    "should-remain",
		},
		OrgURL:      "https://company.okta.com",
		AccessToken: "new-access-token",
	})

	// Conflicting vars should be cleared (not present).
	assert.NotContains(t, result, "OKTA_API_TOKEN")
	assert.NotContains(t, result, "OKTA_CLIENT_ID")
	assert.NotContains(t, result, "OKTA_CLIENT_SECRET")
	assert.NotContains(t, result, "OKTA_PRIVATE_KEY")
	// Access token should be set to new value.
	assert.Equal(t, "new-access-token", result["OKTA_OAUTH2_ACCESS_TOKEN"])
	// Other vars should remain.
	assert.Equal(t, "should-remain", result["OTHER"])
}

func TestPrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	input := map[string]string{
		"EXISTING": "value",
	}
	inputCopy := make(map[string]string)
	for k, v := range input {
		inputCopy[k] = v
	}

	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ: input,
		OrgURL:  "https://company.okta.com",
	})

	// Result should have new values.
	assert.Equal(t, "https://company.okta.com", result["OKTA_ORG_URL"])
	assert.Equal(t, "value", result["EXISTING"])

	// Original input should not be modified.
	assert.Equal(t, inputCopy, input)
	assert.NotContains(t, input, "OKTA_ORG_URL")
}

func TestPrepareEnvironment_EmptyConfig(t *testing.T) {
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ: map[string]string{"EXISTING": "value"},
	})

	// Existing values preserved.
	assert.Equal(t, "value", result["EXISTING"])
	// No Okta-specific values set.
	assert.NotContains(t, result, "OKTA_ORG_URL")
	assert.NotContains(t, result, "OKTA_BASE_URL")
	assert.NotContains(t, result, "OKTA_OAUTH2_ACCESS_TOKEN")
}

func TestPrepareEnvironment_PreservesOtherCloudVars(t *testing.T) {
	// Ensure multi-cloud support: AWS/Azure/GCP credentials should NOT be cleared.
	result := PrepareEnvironment(PrepareEnvironmentConfig{
		Environ: map[string]string{
			"AWS_ACCESS_KEY_ID":     "aws-key",
			"AWS_SECRET_ACCESS_KEY": "aws-secret",
			"AZURE_SUBSCRIPTION_ID": "azure-sub",
			"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp.json",
		},
		OrgURL: "https://company.okta.com",
	})

	// Other cloud credentials should remain.
	assert.Equal(t, "aws-key", result["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "aws-secret", result["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "azure-sub", result["AZURE_SUBSCRIPTION_ID"])
	assert.Equal(t, "/path/to/gcp.json", result["GOOGLE_APPLICATION_CREDENTIALS"])
}
