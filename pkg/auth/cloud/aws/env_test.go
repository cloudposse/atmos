package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrepareEnvironment(t *testing.T) {
	tests := []struct {
		name            string
		inputEnv        map[string]string
		profile         string
		credentialsFile string
		configFile      string
		region          string
		expectedEnv     map[string]string
	}{
		{
			name: "basic environment preparation",
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "us-west-2",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"PATH":                        "/usr/bin",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "us-west-2",
				"AWS_DEFAULT_REGION":          "us-west-2",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "clears conflicting credential environment variables",
			inputEnv: map[string]string{
				"AWS_ACCESS_KEY_ID":           "AKIAIOSFODNN7EXAMPLE",
				"AWS_SECRET_ACCESS_KEY":       "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"AWS_SESSION_TOKEN":           "session-token-value",
				"AWS_SECURITY_TOKEN":          "security-token-value",
				"AWS_WEB_IDENTITY_TOKEN_FILE": "/path/to/token",
				"AWS_ROLE_ARN":                "arn:aws:iam::123456789012:role/MyRole",
				"AWS_ROLE_SESSION_NAME":       "my-session",
				"HOME":                        "/home/user",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "without region",
			inputEnv: map[string]string{
				"HOME": "/home/user",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name:            "with empty input environment",
			inputEnv:        map[string]string{},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "eu-central-1",
			expectedEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "eu-central-1",
				"AWS_DEFAULT_REGION":          "eu-central-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "preserves non-AWS environment variables",
			inputEnv: map[string]string{
				"HOME":       "/home/user",
				"PATH":       "/usr/bin",
				"USER":       "testuser",
				"LANG":       "en_US.UTF-8",
				"CUSTOM_VAR": "custom-value",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "ap-southeast-1",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"PATH":                        "/usr/bin",
				"USER":                        "testuser",
				"LANG":                        "en_US.UTF-8",
				"CUSTOM_VAR":                  "custom-value",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "ap-southeast-1",
				"AWS_DEFAULT_REGION":          "ap-southeast-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call PrepareEnvironment.
			result := PrepareEnvironment(tt.inputEnv, tt.profile, tt.credentialsFile, tt.configFile, tt.region)

			// Verify result matches expected environment.
			assert.Equal(t, tt.expectedEnv, result, "environment should match expected")

			// Verify input environment was not mutated.
			for key, value := range tt.inputEnv {
				// Check that original values are still there (unless they should be cleared).
				shouldBeCleared := false
				for _, clearVar := range environmentVarsToClear {
					if key == clearVar {
						shouldBeCleared = true
						break
					}
				}
				if !shouldBeCleared {
					// Non-cleared variables should remain in input.
					assert.Equal(t, value, tt.inputEnv[key], "input environment should not be mutated for %s", key)
				}
			}
		})
	}
}

func TestPrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	// Create input environment.
	inputEnv := map[string]string{
		"HOME":                  "/home/user",
		"AWS_ACCESS_KEY_ID":     "AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	// Save original values.
	originalHome := inputEnv["HOME"]
	originalAccessKey := inputEnv["AWS_ACCESS_KEY_ID"]
	originalSecretKey := inputEnv["AWS_SECRET_ACCESS_KEY"]

	// Call PrepareEnvironment.
	result := PrepareEnvironment(inputEnv, "test-profile", "/creds", "/config", "us-east-1")

	// Verify input was not mutated.
	assert.Equal(t, originalHome, inputEnv["HOME"], "HOME should not be modified in input")
	assert.Equal(t, originalAccessKey, inputEnv["AWS_ACCESS_KEY_ID"], "AWS_ACCESS_KEY_ID should not be modified in input")
	assert.Equal(t, originalSecretKey, inputEnv["AWS_SECRET_ACCESS_KEY"], "AWS_SECRET_ACCESS_KEY should not be modified in input")

	// Verify result does not contain credentials.
	_, hasAccessKey := result["AWS_ACCESS_KEY_ID"]
	_, hasSecretKey := result["AWS_SECRET_ACCESS_KEY"]
	assert.False(t, hasAccessKey, "result should not contain AWS_ACCESS_KEY_ID")
	assert.False(t, hasSecretKey, "result should not contain AWS_SECRET_ACCESS_KEY")

	// Verify result contains expected values.
	assert.Equal(t, "test-profile", result["AWS_PROFILE"])
	assert.Equal(t, "/creds", result["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, "/config", result["AWS_CONFIG_FILE"])
}
