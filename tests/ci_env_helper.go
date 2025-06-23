package tests

import (
	"os"
	"testing"
)

var ciProvidersEnvVarsExists = map[string]string{
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
	"JENKINS":            "JENKINS_URL",
	"MAGNUM":             "MAGNUM",
	"NEVERCODE":          "NEVERCODE",
	"PROW":               "PROW_JOB_ID",
	"SAIL":               "SAILCI",
	"SEMAPHORE":          "SEMAPHORE",
	"STRIDER":            "STRIDER",
	"TASKCLUSTER":        "TASK_ID",
	"TEAMCITY":           "TEAMCITY_VERSION",
	"TRAVIS":             "TRAVIS",
	"VELA":               "VELA",
}

var ciProvidersEnvVarsEquals = map[string]map[string]string{
	"CODESHIP": {
		"CI_NAME": "codeship",
	},
	"SOURCEHUT": {
		"CI_NAME": "sourcehut",
	},
	"WOODPECKER": {
		"CI": "woodpecker",
	},
}

const ciEnvVar = "CI"

func DisableCIEnvVars(t testing.TB) {
	t.Helper()
	unset := func(key string) {
		if val, ok := os.LookupEnv(key); ok {
			t.Setenv(key, val)
		} else {
			t.Setenv(key, "")
		}
		os.Unsetenv(key)
	}
	for key := range ciProvidersEnvVarsExists {
		unset(key)
	}
	for _, values := range ciProvidersEnvVarsEquals {
		for key := range values {
			unset(key)
		}
	}
	unset(ciEnvVar)
}
