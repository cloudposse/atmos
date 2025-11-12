package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrepareEnvironment(t *testing.T) {
	tests := []struct {
		name             string
		inputEnv         map[string]string
		subscriptionID   string
		tenantID         string
		location         string
		credentialsFile  string
		accessToken      string
		expectedContains map[string]string
		expectedMissing  []string
	}{
		{
			name: "basic environment preparation",
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			subscriptionID:  "12345678-1234-1234-1234-123456789012",
			tenantID:        "87654321-4321-4321-4321-210987654321",
			location:        "eastus",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"PATH":                  "/usr/bin",
				"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
				"ARM_SUBSCRIPTION_ID":   "12345678-1234-1234-1234-123456789012",
				"AZURE_TENANT_ID":       "87654321-4321-4321-4321-210987654321",
				"ARM_TENANT_ID":         "87654321-4321-4321-4321-210987654321",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"ARM_USE_CLI":           "true",
			},
		},
		{
			name: "clears conflicting Azure credential environment variables",
			inputEnv: map[string]string{
				"AZURE_CLIENT_ID":               "conflicting-client-id",
				"AZURE_CLIENT_SECRET":           "conflicting-secret",
				"AZURE_CLIENT_CERTIFICATE_PATH": "/path/to/cert",
				"ARM_CLIENT_ID":                 "conflicting-arm-client",
				"ARM_CLIENT_SECRET":             "conflicting-arm-secret",
				"HOME":                          "/home/user",
			},
			subscriptionID:  "12345678-1234-1234-1234-123456789012",
			tenantID:        "87654321-4321-4321-4321-210987654321",
			location:        "",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
				"ARM_SUBSCRIPTION_ID":   "12345678-1234-1234-1234-123456789012",
				"AZURE_TENANT_ID":       "87654321-4321-4321-4321-210987654321",
				"ARM_TENANT_ID":         "87654321-4321-4321-4321-210987654321",
				"ARM_USE_CLI":           "true",
			},
			expectedMissing: []string{
				"AZURE_CLIENT_ID",
				"AZURE_CLIENT_SECRET",
				"AZURE_CLIENT_CERTIFICATE_PATH",
				"ARM_CLIENT_ID",
				"ARM_CLIENT_SECRET",
			},
		},
		{
			name: "preserves other cloud provider credentials for multi-cloud scenarios",
			inputEnv: map[string]string{
				"AWS_ACCESS_KEY_ID":              "AKIAIOSFODNN7EXAMPLE",
				"AWS_SECRET_ACCESS_KEY":          "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"AWS_SESSION_TOKEN":              "session-token",
				"AWS_PROFILE":                    "aws-profile",
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp/creds.json",
				"GOOGLE_CLOUD_PROJECT":           "my-gcp-project",
				"HOME":                           "/home/user",
			},
			subscriptionID:  "12345678-1234-1234-1234-123456789012",
			tenantID:        "87654321-4321-4321-4321-210987654321",
			location:        "westus",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"HOME":                           "/home/user",
				"AZURE_SUBSCRIPTION_ID":          "12345678-1234-1234-1234-123456789012",
				"ARM_SUBSCRIPTION_ID":            "12345678-1234-1234-1234-123456789012",
				"AZURE_TENANT_ID":                "87654321-4321-4321-4321-210987654321",
				"ARM_TENANT_ID":                  "87654321-4321-4321-4321-210987654321",
				"AZURE_LOCATION":                 "westus",
				"ARM_LOCATION":                   "westus",
				"ARM_USE_CLI":                    "true",
				"AWS_ACCESS_KEY_ID":              "AKIAIOSFODNN7EXAMPLE",
				"AWS_SECRET_ACCESS_KEY":          "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"AWS_SESSION_TOKEN":              "session-token",
				"AWS_PROFILE":                    "aws-profile",
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp/creds.json",
				"GOOGLE_CLOUD_PROJECT":           "my-gcp-project",
			},
			expectedMissing: []string{},
		},
		{
			name:            "with empty input environment",
			inputEnv:        map[string]string{},
			subscriptionID:  "12345678-1234-1234-1234-123456789012",
			tenantID:        "87654321-4321-4321-4321-210987654321",
			location:        "northeurope",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
				"ARM_SUBSCRIPTION_ID":   "12345678-1234-1234-1234-123456789012",
				"AZURE_TENANT_ID":       "87654321-4321-4321-4321-210987654321",
				"ARM_TENANT_ID":         "87654321-4321-4321-4321-210987654321",
				"AZURE_LOCATION":        "northeurope",
				"ARM_LOCATION":          "northeurope",
				"ARM_USE_CLI":           "true",
			},
		},
		{
			name: "without location",
			inputEnv: map[string]string{
				"HOME": "/home/user",
			},
			subscriptionID:  "12345678-1234-1234-1234-123456789012",
			tenantID:        "87654321-4321-4321-4321-210987654321",
			location:        "",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
				"ARM_SUBSCRIPTION_ID":   "12345678-1234-1234-1234-123456789012",
				"AZURE_TENANT_ID":       "87654321-4321-4321-4321-210987654321",
				"ARM_TENANT_ID":         "87654321-4321-4321-4321-210987654321",
				"ARM_USE_CLI":           "true",
			},
			expectedMissing: []string{
				"AZURE_LOCATION",
				"ARM_LOCATION",
			},
		},
		{
			name: "empty subscription and tenant",
			inputEnv: map[string]string{
				"HOME": "/home/user",
			},
			subscriptionID:  "",
			tenantID:        "",
			location:        "eastus",
			credentialsFile: "",
			accessToken:     "",
			expectedContains: map[string]string{
				"HOME":           "/home/user",
				"AZURE_LOCATION": "eastus",
				"ARM_LOCATION":   "eastus",
				"ARM_USE_CLI":    "true",
			},
			expectedMissing: []string{
				"AZURE_SUBSCRIPTION_ID",
				"ARM_SUBSCRIPTION_ID",
				"AZURE_TENANT_ID",
				"ARM_TENANT_ID",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrepareEnvironment(
				tt.inputEnv,
				tt.subscriptionID,
				tt.tenantID,
				tt.location,
				tt.credentialsFile,
				tt.accessToken,
			)

			// Check that expected variables are present with correct values.
			for key, expectedValue := range tt.expectedContains {
				assert.Equal(t, expectedValue, result[key], "Expected %s=%s", key, expectedValue)
			}

			// Check that unwanted variables are not present.
			for _, key := range tt.expectedMissing {
				_, exists := result[key]
				assert.False(t, exists, "Expected %s to be missing", key)
			}

			// Verify ARM_USE_CLI is always set to "true".
			assert.Equal(t, "true", result["ARM_USE_CLI"], "ARM_USE_CLI should always be true")
		})
	}
}

func TestPrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	original := map[string]string{
		"HOME":              "/home/user",
		"AZURE_CLIENT_ID":   "should-be-preserved-in-original",
		"AWS_ACCESS_KEY_ID": "should-be-preserved-in-original",
	}

	// Make a copy to compare later.
	expected := make(map[string]string)
	for k, v := range original {
		expected[k] = v
	}

	_ = PrepareEnvironment(
		original,
		"sub-id",
		"tenant-id",
		"eastus",
		"",
		"",
	)

	// Original should be unchanged.
	assert.Equal(t, expected, original, "PrepareEnvironment should not mutate input map")
}
