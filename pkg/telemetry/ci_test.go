package telemetry

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCiProvider tests the ciProvider function to ensure it correctly identifies
// various CI/CD platforms based on environment variables.
func TestCiProvider(t *testing.T) {
	// Define test cases for different CI providers
	testCases := []struct {
		name           string
		envVars        map[string]string
		expectedResult string
	}{
		// Test providers that check for environment variable existence.
		{
			name: "CODEBUILD",
			envVars: map[string]string{
				"CODEBUILD_BUILD_ARN": "arn:aws:codebuild:us-east-1:123456789012:build/test-project:12345678-1234-1234-1234-123456789012",
			},
			expectedResult: "CODEBUILD",
		},
		{
			name: "AZURE_PIPELINES",
			envVars: map[string]string{
				"TF_BUILD": "true",
			},
			expectedResult: "AZURE_PIPELINES",
		},
		{
			name: "BAMBOO",
			envVars: map[string]string{
				"bamboo_planKey": "TEST-PROJECT",
			},
			expectedResult: "BAMBOO",
		},
		{
			name: "BITBUCKET",
			envVars: map[string]string{
				"BITBUCKET_COMMIT": "abc123def456",
			},
			expectedResult: "BITBUCKET",
		},
		{
			name: "BITRISE",
			envVars: map[string]string{
				"BITRISE_IO": "true",
			},
			expectedResult: "BITRISE",
		},
		{
			name: "BUILDKITE",
			envVars: map[string]string{
				"BUILDKITE": "true",
			},
			expectedResult: "BUILDKITE",
		},
		{
			name: "CIRCLE",
			envVars: map[string]string{
				"CIRCLECI": "true",
			},
			expectedResult: "CIRCLE",
		},
		{
			name: "CIRRUS",
			envVars: map[string]string{
				"CIRRUS_CI": "true",
			},
			expectedResult: "CIRRUS",
		},
		{
			name: "CODEFRESH",
			envVars: map[string]string{
				"CF_BUILD_ID": "12345",
			},
			expectedResult: "CODEFRESH",
		},
		{
			name: "DRONE",
			envVars: map[string]string{
				"DRONE": "true",
			},
			expectedResult: "DRONE",
		},
		{
			name: "GERRIT",
			envVars: map[string]string{
				"GERRIT_PROJECT": "test-project",
			},
			expectedResult: "GERRIT",
		},
		{
			name: "GITEA_ACTIONS",
			envVars: map[string]string{
				"GITEA_ACTIONS": "true",
			},
			expectedResult: "GITEA_ACTIONS",
		},
		{
			name: "GITHUB_ACTIONS",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectedResult: "GITHUB_ACTIONS",
		},
		{
			name: "GITLAB",
			envVars: map[string]string{
				"GITLAB_CI": "true",
			},
			expectedResult: "GITLAB",
		},
		{
			name: "GOCD",
			envVars: map[string]string{
				"GO_PIPELINE_LABEL": "1.0.0",
			},
			expectedResult: "GOCD",
		},
		{
			name: "GOOGLE_CLOUD_BUILD",
			envVars: map[string]string{
				"BUILDER_OUTPUT": "/workspace/output",
			},
			expectedResult: "GOOGLE_CLOUD_BUILD",
		},
		{
			name: "HARNESS",
			envVars: map[string]string{
				"HARNESS_BUILD_ID": "12345",
			},
			expectedResult: "HARNESS",
		},
		{
			name: "HUDSON",
			envVars: map[string]string{
				"HUDSON_URL": "http://hudson.example.com",
			},
			expectedResult: "HUDSON",
		},
		{
			name: "JENKINS",
			envVars: map[string]string{
				"JENKINS_URL": "http://jenkins.example.com",
			},
			expectedResult: "JENKINS",
		},
		{
			name: "PROW",
			envVars: map[string]string{
				"PROW_JOB_ID": "12345",
			},
			expectedResult: "PROW",
		},
		{
			name: "SEMAPHORE",
			envVars: map[string]string{
				"SEMAPHORE": "true",
			},
			expectedResult: "SEMAPHORE",
		},
		{
			name: "TEAMCITY",
			envVars: map[string]string{
				"TEAMCITY_VERSION": "2023.1",
			},
			expectedResult: "TEAMCITY",
		},
		{
			name: "TRAVIS",
			envVars: map[string]string{
				"TRAVIS": "true",
			},
			expectedResult: "TRAVIS",
		},
		// Test providers that check for environment variable equality.
		{
			name: "CODESHIP",
			envVars: map[string]string{
				"CI_NAME": "codeship",
			},
			expectedResult: "CODESHIP",
		},
		{
			name: "SOURCEHUT",
			envVars: map[string]string{
				"CI_NAME": "sourcehut",
			},
			expectedResult: "SOURCEHUT",
		},
		// Test no CI provider detected.
		{
			name:           "No CI provider",
			envVars:        map[string]string{},
			expectedResult: "",
		},
		// Test that first matching provider is returned (priority order).
		{
			name: "Multiple providers - first one wins",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"GITLAB_CI":      "true",
				"TRAVIS":         "true",
			},
			expectedResult: "GITHUB_ACTIONS", // Should return the first one in the map iteration order.
		},
		// Test Spacelift CI
		{
			name: "SPACELIFT",
			envVars: map[string]string{
				"TF_VAR_spacelift_run_id": "12345",
			},
			expectedResult: "SPACELIFT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentEnvVars := PreserveCIEnvVars()
			defer RestoreCIEnvVars(currentEnvVars)

			// Save original environment variables.
			originalEnv := make(map[string]string)
			for key := range tc.envVars {
				if val := os.Getenv(key); val != "" {
					originalEnv[key] = val
				}
			}

			// Set test environment variables.
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			// Clean up environment variables after test.
			defer func() {
				// Clear test environment variables.
				for key := range tc.envVars {
					os.Unsetenv(key)
				}
				// Restore original environment variables.
				for key, value := range originalEnv {
					t.Setenv(key, value)
				}
			}()

			result := ciProvider()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestIsCI(t *testing.T) {
	testCases := []struct {
		name           string
		envVars        map[string]string
		expectedResult bool
	}{
		{
			name: "CI=true",
			envVars: map[string]string{
				"CI": "true",
			},
			expectedResult: true,
		},
		{
			name: "CI=false",
			envVars: map[string]string{
				"CI": "false",
			},
			expectedResult: false,
		},
		{
			name: "CI provider detected",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectedResult: true,
		},
		{
			name:           "No CI environment",
			envVars:        map[string]string{},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentEnvVars := PreserveCIEnvVars()
			defer RestoreCIEnvVars(currentEnvVars)

			// Save original environment variables.
			originalEnv := make(map[string]string)
			for key := range tc.envVars {
				if val := os.Getenv(key); val != "" {
					originalEnv[key] = val
				}
			}

			var envVarsOrdered []string
			for key := range tc.envVars {
				envVarsOrdered = append(envVarsOrdered, key)
			}
			sort.Strings(envVarsOrdered)
			// Set test environment variables.
			for _, key := range envVarsOrdered {
				t.Setenv(key, tc.envVars[key])
			}

			// Clean up environment variables after test.
			defer func() {
				// Clear test environment variables.
				for key := range tc.envVars {
					os.Unsetenv(key)
				}
				// Restore original environment variables
				for key, value := range originalEnv {
					t.Setenv(key, value)
				}
			}()

			result := IsCI()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("notEmpty", func(t *testing.T) {
		// Test with existing environment variable.
		t.Setenv("TEST_VAR", "value")
		defer os.Unsetenv("TEST_VAR")

		assert.True(t, isEnvVarExists("TEST_VAR"))
		assert.False(t, isEnvVarExists("NON_EXISTENT_VAR"))
	})

	t.Run("isTrue", func(t *testing.T) {
		// Test with "true" value.
		t.Setenv("TRUE_VAR", "true")
		defer os.Unsetenv("TRUE_VAR")

		assert.True(t, isEnvVarTrue("TRUE_VAR"))
		assert.False(t, isEnvVarTrue("FALSE_VAR"))
		assert.False(t, isEnvVarTrue("NON_EXISTENT_VAR"))

		// Test with "false" value.
		t.Setenv("FALSE_VAR", "false")
		defer os.Unsetenv("FALSE_VAR")

		assert.True(t, isEnvVarTrue("TRUE_VAR"))
		assert.False(t, isEnvVarTrue("FALSE_VAR"))
		assert.False(t, isEnvVarTrue("NON_EXISTENT_VAR"))
	})

	t.Run("isEquals", func(t *testing.T) {
		// Test with matching value.
		t.Setenv("MATCH_VAR", "expected")
		defer os.Unsetenv("MATCH_VAR")

		assert.True(t, isEnvVarEquals("MATCH_VAR", "expected"))
		assert.False(t, isEnvVarEquals("MATCH_VAR", "unexpected"))
		assert.False(t, isEnvVarEquals("NON_EXISTENT_VAR", "any"))
	})
}
