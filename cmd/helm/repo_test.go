package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRepoCommandStructure(t *testing.T) {
	repoCmd := findSubcommand(helmCmd, "repo")
	require.NotNil(t, repoCmd)

	listCmd := findSubcommand(repoCmd, "list")
	require.NotNil(t, listCmd)
	for _, name := range []string{"format", "columns", "process-templates", "process-functions", "skip"} {
		assert.NotNil(t, listCmd.Flag(name), "expected repo list flag %q", name)
	}
}

func TestHelmRepositoryRows(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Helm.Repositories = []schema.HelmRepository{
		{Name: "global", URL: "https://global.example.com"},
		{Name: "shared", URL: "https://old.example.com"},
	}
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"helm": map[string]any{
					"app": map[string]any{
						"chart":      "shared/nginx",
						"repository": "https://direct.example.com",
						"repositories": []any{
							map[string]any{"name": "shared", "url": "https://new.example.com"},
							map[string]any{"name": "extra", "url": "https://extra.example.com"},
						},
					},
					"other": map[string]any{
						"chart": "global/redis",
					},
				},
			},
		},
	}

	rows := helmRepositoryRows(atmosConfig, stacks, "app")
	require.Len(t, rows, 4)
	assert.Equal(t, directRepositoryName, rows[0]["name"])
	assert.Equal(t, "direct", rows[0]["source"])
	assert.Equal(t, true, rows[0]["used"])
	assert.Equal(t, "global", rows[1]["name"])
	assert.Equal(t, "global", rows[1]["source"])
	assert.Equal(t, false, rows[1]["used"])
	assert.Equal(t, "shared", rows[2]["name"])
	assert.Equal(t, "component", rows[2]["source"])
	assert.Equal(t, "https://new.example.com", rows[2]["url"])
	assert.Equal(t, true, rows[2]["used"])
	assert.Equal(t, "extra", rows[3]["name"])
	assert.Equal(t, false, rows[3]["used"])
}

func TestRepoListAuthManagerSkipsImplicitIdentity(t *testing.T) {
	for _, identity := range []string{"", cfg.IdentityFlagDisabledValue} {
		t.Run(identity, func(t *testing.T) {
			manager, err := repoListAuthManager(
				&schema.ConfigAndStacksInfo{Identity: identity},
				&schema.AtmosConfiguration{},
			)

			require.NoError(t, err)
			assert.Nil(t, manager)
		})
	}
}

func TestChartUsesRepository(t *testing.T) {
	assert.True(t, chartUsesRepository("bitnami/nginx", "bitnami"))
	assert.False(t, chartUsesRepository("other/nginx", "bitnami"))
	assert.False(t, chartUsesRepository("./chart", "chart"))
	assert.False(t, chartUsesRepository("oci://example.com/chart", "example"))
}
