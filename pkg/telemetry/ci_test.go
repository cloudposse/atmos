package telemetry

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCiProvider(t *testing.T) {
	testCases := []struct {
		name           string
		envVars        map[string]string
		expectedResult string
	}{
		// Test providers that check for environment variable existence
		{
			name: "AGOLA",
			envVars: map[string]string{
				"AGOLA_GIT_REF": "refs/heads/main",
			},
			expectedResult: "AGOLA",
		},
		{
			name: "APPCIRCLE",
			envVars: map[string]string{
				"AC_APPCIRCLE": "true",
			},
			expectedResult: "APPCIRCLE",
		},
		{
			name: "APPVEYOR",
			envVars: map[string]string{
				"APPVEYOR": "true",
			},
			expectedResult: "APPVEYOR",
		},
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
			name: "BUDDY",
			envVars: map[string]string{
				"BUDDY_WORKSPACE_ID": "12345",
			},
			expectedResult: "BUDDY",
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
			name: "DSARI",
			envVars: map[string]string{
				"DSARI": "true",
			},
			expectedResult: "DSARI",
		},
		{
			name: "EARTHLY",
			envVars: map[string]string{
				"EARTHLY_CI": "true",
			},
			expectedResult: "EARTHLY",
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
			name: "MAGNUM",
			envVars: map[string]string{
				"MAGNUM": "true",
			},
			expectedResult: "MAGNUM",
		},
		{
			name: "NEVERCODE",
			envVars: map[string]string{
				"NEVERCODE": "true",
			},
			expectedResult: "NEVERCODE",
		},
		{
			name: "PROW",
			envVars: map[string]string{
				"PROW_JOB_ID": "12345",
			},
			expectedResult: "PROW",
		},
		{
			name: "SAIL",
			envVars: map[string]string{
				"SAILCI": "true",
			},
			expectedResult: "SAIL",
		},
		{
			name: "SEMAPHORE",
			envVars: map[string]string{
				"SEMAPHORE": "true",
			},
			expectedResult: "SEMAPHORE",
		},
		{
			name: "STRIDER",
			envVars: map[string]string{
				"STRIDER": "true",
			},
			expectedResult: "STRIDER",
		},
		{
			name: "TASKCLUSTER",
			envVars: map[string]string{
				"TASK_ID": "abc123",
			},
			expectedResult: "TASKCLUSTER",
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
		{
			name: "VELA",
			envVars: map[string]string{
				"VELA": "true",
			},
			expectedResult: "VELA",
		},
		// Test providers that check for environment variable equality
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
		{
			name: "WOODPECKER",
			envVars: map[string]string{
				"CI": "woodpecker",
			},
			expectedResult: "WOODPECKER",
		},
		// Test no CI provider detected
		{
			name:           "No CI provider",
			envVars:        map[string]string{},
			expectedResult: "",
		},
		// Test that first matching provider is returned (priority order)
		{
			name: "Multiple providers - first one wins",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"GITLAB_CI":      "true",
				"TRAVIS":         "true",
			},
			expectedResult: "GITHUB_ACTIONS", // Should return the first one in the map iteration order
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save original environment variables
			originalEnv := make(map[string]string)
			for key := range tc.envVars {
				if val := os.Getenv(key); val != "" {
					originalEnv[key] = val
					os.Unsetenv(key)
				}
			}

			// Set test environment variables
			for key, value := range tc.envVars {
				os.Setenv(key, value)
			}

			// Clean up environment variables after test
			defer func() {
				// Clear test environment variables
				for key := range tc.envVars {
					os.Unsetenv(key)
				}
				// Restore original environment variables
				for key, value := range originalEnv {
					os.Setenv(key, value)
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
			// Save original environment variables
			originalEnv := make(map[string]string)
			for key := range tc.envVars {
				if val := os.Getenv(key); val != "" {
					originalEnv[key] = val
				}
			}
			// https://bitfieldconsulting.com/posts/map-iteration
			var envVarsOrdered []string
			for key := range tc.envVars {
				envVarsOrdered = append(envVarsOrdered, key)
			}
			sort.Strings(envVarsOrdered)
			// Set test environment variables
			for _, key := range envVarsOrdered {
				os.Setenv(key, tc.envVars[key])
			}

			// Clean up environment variables after test
			defer func() {
				// Clear test environment variables
				for key := range tc.envVars {
					os.Unsetenv(key)
				}
				// Restore original environment variables
				for key, value := range originalEnv {
					os.Setenv(key, value)
				}
			}()

			result := isCI()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("notEmpty", func(t *testing.T) {
		// Test with existing environment variable
		os.Setenv("TEST_VAR", "value")
		defer os.Unsetenv("TEST_VAR")

		assert.True(t, isEnvVarExists("TEST_VAR"))
		assert.False(t, isEnvVarExists("NON_EXISTENT_VAR"))
	})

	t.Run("isTrue", func(t *testing.T) {
		// Test with "true" value
		os.Setenv("TRUE_VAR", "true")
		defer os.Unsetenv("TRUE_VAR")

		assert.True(t, isEnvVarTrue("TRUE_VAR"))
		assert.False(t, isEnvVarTrue("FALSE_VAR"))
		assert.False(t, isEnvVarTrue("NON_EXISTENT_VAR"))

		// Test with "false" value
		os.Setenv("FALSE_VAR", "false")
		defer os.Unsetenv("FALSE_VAR")

		assert.True(t, isEnvVarTrue("TRUE_VAR"))
		assert.False(t, isEnvVarTrue("FALSE_VAR"))
		assert.False(t, isEnvVarTrue("NON_EXISTENT_VAR"))
	})

	t.Run("isEquals", func(t *testing.T) {
		// Test with matching value
		os.Setenv("MATCH_VAR", "expected")
		defer os.Unsetenv("MATCH_VAR")

		assert.True(t, isEnvVarEquals("MATCH_VAR", "expected"))
		assert.False(t, isEnvVarEquals("MATCH_VAR", "unexpected"))
		assert.False(t, isEnvVarEquals("NON_EXISTENT_VAR", "any"))
	})
}
