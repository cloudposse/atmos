package testutils

import "os"

// ciEnvVar defines the generic CI environment variable used by many providers.
const ciEnvVar = "CI"

// ciProvidersEnvVarsExists lists environment variables that identify a CI provider when present.
var ciProvidersEnvVarsExists = map[string]string{
	"CODEBUILD":          "CODEBUILD_BUILD_ARN",
	"AZURE_PIPELINES":    "TF_BUILD",
	"BAMBOO":             "bamboo_planKey",
	"BITBUCKET":          "BITBUCKET_COMMIT",
	"BITRISE":            "BITRISE_IO",
	"BUILDKITE":          "BUILDKITE",
	"CIRCLE":             "CIRCLECI",
	"CIRRUS":             "CIRRUS_CI",
	"CODEFRESH":          "CF_BUILD_ID",
	"DRONE":              "DRONE",
	"GERRIT":             "GERRIT_PROJECT",
	"GITEA_ACTIONS":      "GITEA_ACTIONS",
	"GITHUB_ACTIONS":     "GITHUB_ACTIONS",
	"GITLAB":             "GITLAB_CI",
	"GOCD":               "GO_PIPELINE_LABEL",
	"GOOGLE_CLOUD_BUILD": "BUILDER_OUTPUT",
	"HARNESS":            "HARNESS_BUILD_ID",
	"HUDSON":             "HUDSON_URL",
	"JENKINS":            "JENKINS_URL",
	"PROW":               "PROW_JOB_ID",
	"SEMAPHORE":          "SEMAPHORE",
	"TEAMCITY":           "TEAMCITY_VERSION",
	"TRAVIS":             "TRAVIS",
}

// ciProvidersEnvVarsEquals lists environment variables whose value identifies a CI provider.
var ciProvidersEnvVarsEquals = map[string]map[string]string{
	"CODESHIP": {
		"CI_NAME": "codeship",
	},
	"SOURCEHUT": {
		"CI_NAME": "sourcehut",
	},
}

// PreserveCIEnvVars temporarily removes CI-related environment variables from the current process
// and returns a map containing the original values. This is useful for testing scenarios
// where you want to ensure a clean environment without CI detection.
//
// The function handles three categories of CI environment variables:
// 1. Variables from ciProvidersEnvVarsExists (existence-based detection)
// 2. Variables from ciProvidersEnvVarsEquals (value-based detection)
// 3. The general "CI" environment variable
//
// Returns a map of preserved environment variable names to their original values.
func PreserveCIEnvVars() map[string]string {
	envVars := make(map[string]string)

	// Preserve and unset CI provider variables that are detected by existence
	for key := range ciProvidersEnvVarsExists {
		if val, ok := os.LookupEnv(key); ok {
			envVars[key] = val
			os.Unsetenv(key)
		}
	}

	// Preserve and unset CI provider variables that are detected by specific values
	for _, values := range ciProvidersEnvVarsEquals {
		for valueKey := range values {
			if val, ok := os.LookupEnv(valueKey); ok {
				envVars[valueKey] = val
				os.Unsetenv(valueKey)
			}
		}
	}

	// Preserve and unset the general CI environment variable
	if val, ok := os.LookupEnv(ciEnvVar); ok {
		envVars[ciEnvVar] = val
		os.Unsetenv(ciEnvVar)
	}

	return envVars
}

// RestoreCIEnvVars restores previously preserved CI environment variables back to the system.
// This function is typically called in a defer statement after PreserveCIEnvVars to ensure
// the original environment is restored, even if the calling function panics or returns early.
//
// Parameters:
//   - envVars: A map of environment variable names to their original values, typically
//     returned from a previous call to PreserveCIEnvVars
func RestoreCIEnvVars(envVars map[string]string) {
	for key, value := range envVars {
		os.Setenv(key, value)
	}
}
