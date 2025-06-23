package testutils

import (
	"os"
	"testing"
)

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

func TestPreserveRestoreCIEnvVars(t *testing.T) {
	// Set a couple of CI environment variables
	os.Setenv("CI", "true")
	os.Setenv("GITHUB_ACTIONS", "true")

	env := PreserveCIEnvVars()
	if _, ok := os.LookupEnv("CI"); ok {
		t.Errorf("CI should be unset after PreserveCIEnvVars")
	}
	if _, ok := os.LookupEnv("GITHUB_ACTIONS"); ok {
		t.Errorf("GITHUB_ACTIONS should be unset after PreserveCIEnvVars")
	}
	if env["CI"] != "true" || env["GITHUB_ACTIONS"] != "true" {
		t.Errorf("preserved values incorrect: %+v", env)
	}

	RestoreCIEnvVars(env)
	if v := os.Getenv("CI"); v != "true" {
		t.Errorf("CI not restored, got %s", v)
	}
	if v := os.Getenv("GITHUB_ACTIONS"); v != "true" {
		t.Errorf("GITHUB_ACTIONS not restored, got %s", v)
	}
}

func TestPreserveCIEnvVarsNoVars(t *testing.T) {
	os.Unsetenv("CI")
	os.Unsetenv("GITHUB_ACTIONS")

	env := PreserveCIEnvVars()
	if len(env) != 0 {
		t.Errorf("expected no env vars preserved, got %d", len(env))
	}
}
