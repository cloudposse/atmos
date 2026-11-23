package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubAmbientEnvironment(t *testing.T) {
	env := map[string]string{
		"PATH":                               "/usr/bin",
		"HOME":                               "/home/me",
		"GOOGLE_APPLICATION_CREDENTIALS":     "/home/me/key.json",
		"CLOUDSDK_COMPUTE_REGION":            "caller-region",
		"GOOGLE_OAUTH_ACCESS_TOKEN":          "caller-token",
		"GOOGLE_CREDENTIALS":                 "{}",
		"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": "other@example.iam.gserviceaccount.com",
		"CLOUDSDK_AUTH_ACCESS_TOKEN":         "caller-sdk-token",
		"CLOUDSDK_AUTH_ACCESS_TOKEN_FILE":    "/home/me/token",
	}

	ScrubAmbientEnvironment(env)

	assert.Equal(t, map[string]string{"PATH": "/usr/bin", "HOME": "/home/me"}, env)
}

func TestScrubAmbientEnvironment_RemovesEveryManagedVariable(t *testing.T) {
	env := map[string]string{}
	for _, key := range GCPEnvironmentVariables {
		env[key] = "caller"
	}
	for _, key := range gcpCredentialOverrideEnvironmentVariables {
		env[key] = "caller"
	}

	ScrubAmbientEnvironment(env)

	assert.Empty(t, env)
}
