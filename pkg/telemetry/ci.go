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
		"AGOLA":              "AGOLA_GIT_REF",
		"APPCIRCLE":          "AC_APPCIRCLE",
		"APPVEYOR":           "APPVEYOR",
		"CODEBUILD":          "CODEBUILD_BUILD_ARN",
		"AZURE_PIPELINES":    "TF_BUILD",
		"BAMBOO":             "bamboo_planKey",
		"BITBUCKET":          "BITBUCKET_COMMIT",
		"BITRISE":            "BITRISE_IO",
		"BUDDY":              "BUDDY_WORKSPACE_ID",
		"BUILDKITE":          "BUILDKITE",
		"CIRCLE":             "CIRCLECI",
		"CIRRUS":             "CIRRUS_CI",
		"CODEFRESH":          "CF_BUILD_ID",
		"DRONE":              "DRONE",
		"DSARI":              "DSARI",
		"EARTHLY":            "EARTHLY_CI",
		"GERRIT":             "GERRIT_PROJECT",
		"GITEA_ACTIONS":      "GITEA_ACTIONS",
		"GITHUB_ACTIONS":     "GITHUB_ACTIONS",
		"GITLAB":             "GITLAB_CI",
		"GOCD":               "GO_PIPELINE_LABEL",
		"GOOGLE_CLOUD_BUILD": "BUILDER_OUTPUT",
		"HARNESS":            "HARNESS_BUILD_ID",
		"HUDSON":             "HUDSON_URL",
		"JENKINS":            "JENKINS_URL", // "JENKINS_URL" and "BUILD_ID"
		"MAGNUM":             "MAGNUM",
		"NEVERCODE":          "NEVERCODE",
		"PROW":               "PROW_JOB_ID",
		"SAIL":               "SAILCI",
		"SEMAPHORE":          "SEMAPHORE",
		"STRIDER":            "STRIDER",
		"TASKCLUSTER":        "TASK_ID", // "TASK_ID"  &&  "RUN_ID"
		"TEAMCITY":           "TEAMCITY_VERSION",
		"TRAVIS":             "TRAVIS",
		"VELA":               "VELA",
	}

	// Map of CI providers that can be detected by checking if environment variables equal specific values.
	ciProvidersEnvVarsEquals = map[string]map[string]string{
		"CODESHIP": {
			"CI_NAME": "codeship",
		},
		"SOURCEHUT": {
			"CI_NAME": "sourcehut",
		},
		"WOODPECKER": {
			ciEnvVar: "woodpecker",
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
