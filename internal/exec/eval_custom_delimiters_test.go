package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func customDelimiterEvaluationConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	writeMinimalStackFixture(t, `base_path: .
stacks:
  base_path: stacks
  included_paths: ["*"]
components:
  terraform:
    base_path: components/terraform
templates:
  settings:
    enabled: true
    evaluations: 2
    delimiters: ["[[", "]]"]
`, `components:
  terraform:
    mock:
      vars:
        selected: '[[ .settings.owner ]]'
        unused: '!aws.account_id'
      settings:
        owner: '[[ printf "%s" "TEAM" ]]'
`)
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	authdeferred.ConfigureAuth(&ac, "")
	setDeferredAuthFactory(&ac, authdeferred.NewMockAuthFactory(gomock.NewController(t)))
	return &ac
}

func TestDescribeStacksCustomDelimiterEvaluation(t *testing.T) {
	for _, inventory := range []bool{true, false} {
		name := "selected field and dependency"
		if inventory {
			name = "inventory has no evaluation"
		}
		t.Run(name, func(t *testing.T) {
			ac := customDelimiterEvaluationConfig(t)
			paths, sections := [][]string{{"vars", "selected"}}, []string{"vars"}
			if inventory {
				paths, sections = [][]string{}, []string{}
			}
			result, err := ExecuteDescribeStacksWithEvalSections(ac, "dev", nil, nil, nil, false, true, true, false, nil, nil, nil, nil,
				DescribeStacksErrorOptions{EvaluationPaths: paths}, sections)
			require.NoError(t, err)
			component := result["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["mock"].(map[string]any)
			if inventory {
				require.Equal(t, "[[ .settings.owner ]]", component["vars"].(map[string]any)["selected"])
			} else {
				require.Equal(t, "TEAM", component["vars"].(map[string]any)["selected"])
				require.Equal(t, "TEAM", component["settings"].(map[string]any)["owner"])
			}
			require.Equal(t, "!aws.account_id", component["vars"].(map[string]any)["unused"])
		})
	}
}

func TestProcessStacksCustomDelimiterDependencies(t *testing.T) {
	ac := customDelimiterEvaluationConfig(t)
	info, err := ProcessStacks(ac, schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentType: "terraform", ComponentFromArg: "mock",
		EvaluationPaths: [][]string{{"vars", "selected"}},
	}, true, true, true, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "TEAM", info.ComponentSection["vars"].(map[string]any)["selected"])
	require.Equal(t, "TEAM", info.ComponentSection["settings"].(map[string]any)["owner"])
	require.Equal(t, "!aws.account_id", info.ComponentSection["vars"].(map[string]any)["unused"])
}
