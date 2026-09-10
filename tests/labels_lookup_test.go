package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
)

// TestLabelsLookupInWorkflowInputs verifies that defaults and inherited overrides
// resolve identically through label lookups and templates before workflow dispatch.
func TestLabelsLookupInWorkflowInputs(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")
	for _, tc := range []struct {
		component, runner string
		inherits          bool
	}{
		{"runner-default", "ubuntu-latest", false},
		{"runner-inherited", "self-hosted-large", true},
		{"runner-override", "self-hosted-gpu", true},
		{"runner-empty", "", true},
		{"runner-mapping-only", "ubuntu-latest", false},
	} {
		t.Run(tc.component, func(t *testing.T) {
			section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component: tc.component, Stack: "label-lookup",
				ProcessTemplates: true, ProcessYamlFunctions: true,
			})
			require.NoError(t, err)
			current := section["settings"].(map[string]any)
			for _, key := range []string{"pro", "pull_request", "merged", "workflows", "apply.yaml", "inputs"} {
				next, ok := current[key].(map[string]any)
				require.True(t, ok, "missing map %s", key)
				current = next
			}
			require.Equal(t, tc.runner, current["runner"])
			require.Equal(t, tc.runner, current["runner_template"])
			require.Equal(t, tc.component, current["component"])
			require.Equal(t, "label-lookup", current["stack"])
			if tc.inherits {
				vars := section["vars"].(map[string]any)
				require.Equal(t, tc.runner, vars["runner"])
				require.Equal(t, "default runner", vars["fallback"])
				require.Equal(t, "", vars["empty_fallback"])
				require.Equal(t, "platform", vars["cost_center"])
				require.Equal(t, "yaml-function-test", vars["implementation"])
			}
		})
	}
}
