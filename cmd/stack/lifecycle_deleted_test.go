package stack

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// Deleted definitions must retain their own count in both the empty-plan and execution paths.
func TestLifecycleReportsDeletedDefinitionsSeparately(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "with live components"
		if empty {
			name = "only deleted component"
		}
		t.Run(name, func(t *testing.T) {
			run, outputFile, summaryFile := ciRun(t)
			selection := &lifecycle.Selection{Stack: "platform", Affected: []schema.Affected{
				{ComponentType: "terraform", Stack: "platform", Component: "registry"},
				{ComponentType: "helmfile", Stack: "platform", Component: "ingress"},
				{ComponentType: "packer", Stack: "platform", Component: "image"},
				{ComponentType: "terraform", Stack: "platform", Component: "gone", Deleted: true},
				{ComponentType: "terraform", Stack: "dev", Component: "gone", Deleted: true},
				{ComponentType: "terraform", Stack: "platform", Component: "other-gone", Deleted: true},
			}}
			if empty {
				selection.Components = []string{"gone"}
			}
			plan, err := lifecycleFixturePlan(t, "deploy", selection)
			require.NoError(t, err)
			run.plan = plan
			output := captureLifecycle(t)
			require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))

			written, err := os.ReadFile(outputFile)
			require.NoError(t, err)
			outputs := parseCIOutputs(string(written))
			summary, err := os.ReadFile(summaryFile)
			require.NoError(t, err)
			want := "2 succeeded, 0 failed, 0 skipped, 1 excluded, 2 deleted awaiting teardown"
			if empty {
				want = "0 succeeded, 0 failed, 0 skipped, 0 excluded, 1 deleted awaiting teardown"
				require.Equal(t, "0", outputs["components_total"])
				require.Equal(t, "0", outputs["components_excluded"])
				require.Equal(t, "1", outputs["components_deleted"])
			} else {
				require.Equal(t, "2", outputs["components_total"])
				require.Equal(t, "1", outputs["components_excluded"])
				require.Equal(t, "2", outputs["components_deleted"])
			}
			require.Equal(t, "true", outputs["success"])
			require.Contains(t, string(summary), want)
			require.Contains(t, output.String(), want)
		})
	}
}
