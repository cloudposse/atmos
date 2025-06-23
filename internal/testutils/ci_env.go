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

// PreserveCIEnvVars removes CI related environment variables and returns their previous values.
// Use in tests to ensure a clean environment without CI detection.
func PreserveCIEnvVars() map[string]string {
	envVars := make(map[string]string)

	for key := range ciProvidersEnvVarsExists {
		if val, ok := os.LookupEnv(key); ok {
			envVars[key] = val
			os.Unsetenv(key)
		}
	}

	for _, values := range ciProvidersEnvVarsEquals {
		for valueKey := range values {
			if val, ok := os.LookupEnv(valueKey); ok {
				envVars[valueKey] = val
				os.Unsetenv(valueKey)
			}
		}
	}

	if val, ok := os.LookupEnv(ciEnvVar); ok {
		envVars[ciEnvVar] = val
		os.Unsetenv(ciEnvVar)
	}

	return envVars
}

// RestoreCIEnvVars restores environment variables previously removed by PreserveCIEnvVars.
func RestoreCIEnvVars(envVars map[string]string) {
	for key, value := range envVars {
		os.Setenv(key, value)
	}
}
