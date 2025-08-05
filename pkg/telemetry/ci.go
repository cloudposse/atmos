package telemetry

import (
	"os"
	"sort"
)

const (
	ciEnvVar = "CI"
)

var (
	// Inspired by https://github.com/watson/ci-info .

	// Map of CI providers that can be detected by checking if specific environment variables exist.
	ciProvidersEnvVarsExists = map[string]string{
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
		"JENKINS":            "JENKINS_URL", // "JENKINS_URL" and "BUILD_ID"
		"PROW":               "PROW_JOB_ID",
		"SEMAPHORE":          "SEMAPHORE",
		"TEAMCITY":           "TEAMCITY_VERSION",
		"TRAVIS":             "TRAVIS",
		"SPACELIFT":          "TF_VAR_spacelift_run_id",
	}

	// Map of CI providers that can be detected by checking if environment variables equal specific values.
	ciProvidersEnvVarsEquals = map[string]map[string]string{
		"CODESHIP": {
			"CI_NAME": "codeship",
		},
		"SOURCEHUT": {
			"CI_NAME": "sourcehut",
		},
	}
)

// isEnvVarExists checks if an environment variable exists and is not empty.
func isEnvVarExists(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

// isEnvVarEquals checks if an environment variable exists and equals the specified value.
func isEnvVarEquals(key string, value string) bool {
	return isEnvVarExists(key) && os.Getenv(key) == value
}

// isEnvVarTrue checks if an environment variable exists and equals "true".
func isEnvVarTrue(key string) bool {
	return isEnvVarEquals(key, "true")
}

// isCI determines if the current environment is a CI/CD environment.
// Returns true if CI=true or if a specific CI provider is detected.
func isCI() bool {
	return isEnvVarTrue(ciEnvVar) || ciProvider() != ""
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
	// Initialize map to store original environment variable values
	envVars := make(map[string]string)

	// Preserve and unset CI provider variables that are detected by existence
	for key := range ciProvidersEnvVarsExists {
		if isEnvVarExists(key) {
			envVars[key] = os.Getenv(key)
			os.Unsetenv(key)
		}
	}

	// Preserve and unset CI provider variables that are detected by specific values
	for _, values := range ciProvidersEnvVarsEquals {
		for valueKey := range values {
			if isEnvVarExists(valueKey) {
				envVars[valueKey] = os.Getenv(valueKey)
				os.Unsetenv(valueKey)
			}
		}
	}

	// Preserve and unset the general CI environment variable
	if isEnvVarExists(ciEnvVar) {
		envVars[ciEnvVar] = os.Getenv(ciEnvVar)
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
	// Restore each environment variable to its original value
	for key, value := range envVars {
		os.Setenv(key, value)
	}
}

// applyAlphabeticalOrder is a generic function that processes a map in alphabetical order.
// It applies a filter function to each value and returns the first key where the filter returns true.
// V can be either string or map[string]string.
func applyAlphabeticalOrder[V string | map[string]string](table map[string]V, filter func(V) bool) string {
	// Extract all keys from the map
	var keys []string
	for key := range table {
		keys = append(keys, key)
	}
	// Sort keys alphabetically for consistent ordering.
	sort.Strings(keys)
	// Apply filter to each value in alphabetical order.
	for _, key := range keys {
		if filter(table[key]) {
			return key
		}
	}
	return ""
}

// ciProvider detects which CI/CD provider is currently running.
// Returns the name of the detected provider or empty string if none found.
func ciProvider() string {
	// First, check providers that can be detected by environment variable existence.
	// Process in alphabetical order for consistent results.
	if result := applyAlphabeticalOrder(ciProvidersEnvVarsExists, isEnvVarExists); result != "" {
		return result
	}

	// Helper function to check if any environment variable in the map equals its expected value.
	checkEnvVarsEquals := func(key map[string]string) bool {
		for envName, envValue := range key {
			if isEnvVarEquals(envName, envValue) {
				return true
			}
		}
		return false
	}

	// Then, check providers that require specific environment variable values.
	// Process in alphabetical order for consistent results.
	if result := applyAlphabeticalOrder(ciProvidersEnvVarsEquals, checkEnvVarsEquals); result != "" {
		return result
	}

	// No CI provider detected.
	return ""
}
