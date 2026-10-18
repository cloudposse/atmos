package ci

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGitHubPlanfilePriority checks upload prerequisites without excluding REST downloads.
func TestGitHubPlanfilePriority(t *testing.T) {
	for _, tokenEnv := range []string{"ATMOS_CI_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		t.Run(tokenEnv, func(t *testing.T) {
			for _, name := range []string{"ATMOS_CI_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
				t.Setenv(name, "")
			}
			t.Setenv(tokenEnv, "fixture-token")
			for _, tc := range []struct {
				name, event, runtimeToken, resultsURL, want string
			}{
				{"upload without runtime credentials", "after.terraform.plan", "", "", "local/dir"},
				{"upload without runtime token", "after.terraform.plan", "", "https://results.example.com", "local/dir"},
				{"upload without results URL", "after.terraform.plan", "fixture-runtime", "", "local/dir"},
				{"upload with runtime credentials", "after.terraform.plan", "fixture-runtime", "https://results.example.com", "github/artifacts"},
				{"aggregate upload without runtime credentials", "after.terraform.plan.aggregate", "", "", "local/dir"},
				{"download with API token only", "before.terraform.apply", "", "", "github/artifacts"},
				{"verification with API token only", "before.terraform.deploy", "", "", "github/artifacts"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv("ACTIONS_RUNTIME_TOKEN", tc.runtimeToken)
					t.Setenv("ACTIONS_RESULTS_URL", tc.resultsURL)
					config := &schema.AtmosConfiguration{}
					config.Components.Terraform.Planfiles = schema.PlanfilesConfig{
						Priority: []string{"github", "local"},
						Stores: map[string]schema.PlanfileStoreSpec{
							"github": {Type: "github/artifacts", Options: map[string]any{"owner": "example", "repo": "repo"}},
							"local":  {Type: "local/dir", Options: map[string]any{"path": filepath.Join(t.TempDir(), "plans")}},
						},
					}
					selected, err := createPlanfileStore(ExecuteOptions{AtmosConfig: config, Event: tc.event})
					require.NoError(t, err)
					assert.Equal(t, tc.want, selected.Name())
				})
			}
		})
	}
}
