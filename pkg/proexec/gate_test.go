package proexec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// withCIEnv isolates CI-detection env vars for the duration of the test,
// using telemetry's own preserve/restore helpers so this test is robust to
// running inside real CI. When ci is true, only the generic CI=true var is
// set (provider-specific vars stay cleared).
func withCIEnv(t *testing.T, ci bool) {
	t.Helper()
	preserved := telemetry.PreserveCIEnvVars()
	// Jenkins detection requires both JENKINS_URL and BUILD_ID; neither is
	// covered by PreserveCIEnvVars' existence/equals maps.
	jenkinsURL, hadJenkinsURL := os.LookupEnv("JENKINS_URL")
	buildID, hadBuildID := os.LookupEnv("BUILD_ID")
	os.Unsetenv("JENKINS_URL")
	os.Unsetenv("BUILD_ID")

	t.Cleanup(func() {
		telemetry.RestoreCIEnvVars(preserved)
		if hadJenkinsURL {
			os.Setenv("JENKINS_URL", jenkinsURL)
		}
		if hadBuildID {
			os.Setenv("BUILD_ID", buildID)
		}
	})

	if ci {
		t.Setenv("CI", "true")
	}
}

func TestGateOpen(t *testing.T) {
	tests := []struct {
		name         string
		ci           bool
		proSettings  schema.ProSettings
		expectedOpen bool
	}{
		{
			name:         "CI and static token configured",
			ci:           true,
			proSettings:  schema.ProSettings{Token: "abc"},
			expectedOpen: true,
		},
		{
			name: "CI and full OIDC configured",
			ci:   true,
			proSettings: schema.ProSettings{
				WorkspaceID: "ws",
				GithubOIDC: schema.GithubOIDCSettings{
					RequestURL:   "https://example.com",
					RequestToken: "token",
				},
			},
			expectedOpen: true,
		},
		{
			name:         "CI but not Pro configured",
			ci:           true,
			proSettings:  schema.ProSettings{},
			expectedOpen: false,
		},
		{
			name:         "Not CI but Pro configured",
			ci:           false,
			proSettings:  schema.ProSettings{Token: "abc"},
			expectedOpen: false,
		},
		{
			name:         "Not CI and not Pro configured",
			ci:           false,
			proSettings:  schema.ProSettings{},
			expectedOpen: false,
		},
		{
			name: "CI with partial OIDC (missing workspace)",
			ci:   true,
			proSettings: schema.ProSettings{
				GithubOIDC: schema.GithubOIDCSettings{
					RequestURL:   "https://example.com",
					RequestToken: "token",
				},
			},
			expectedOpen: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCIEnv(t, tt.ci)
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Pro: tt.proSettings},
			}
			assert.Equal(t, tt.expectedOpen, gateOpen(atmosConfig))
		})
	}
}

func TestGateOpen_NilConfig(t *testing.T) {
	withCIEnv(t, true)
	assert.False(t, gateOpen(nil))
}
