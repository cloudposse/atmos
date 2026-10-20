package lifecycle

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestComponentSelection(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	app := []string{"terraform", "dev", "services/app"}
	helm := []string{"helm", "dev", "services/app"}
	prod := []string{"terraform", "prod", "services/app"}
	stacks := inventory(network, app, helm, prod, []string{"terraform", "dev", "services/app-extra"})
	depends(stacks, app, network, true)
	section(stacks, "helm", "dev", "services/app")["metadata"] = map[string]any{"tags": []any{"release"}}
	tests := []struct {
		name string
		pick Selection
		want []string
	}{
		{"exact name across kinds", Selection{Stack: "dev", Component: "services/app"}, []string{"helm/dev/services/app", "terraform/dev/services/app"}},
		{"across stacks", Selection{Component: "services/app"}, []string{"helm/dev/services/app", "terraform/dev/services/app", "terraform/prod/services/app"}},
		{"affected intersection", Selection{Component: "services/app", Affected: affected(app, network)}, []string{"terraform/dev/services/app"}},
		{"unaffected name", Selection{Component: "services/app", Affected: affected(network)}, nil},
		{"tag intersection", Selection{Component: "services/app", Filter: &Filter{Tags: []string{"release"}}}, []string{"helm/dev/services/app"}},
		{"dependencies may have other names", Selection{Stack: "dev", Component: "services/app", Dependencies: 1}, []string{"helm/dev/services/app", "terraform/dev/network", "terraform/dev/services/app"}},
		{"dependents may have other names", Selection{Stack: "dev", Component: "network", Dependents: 1}, []string{"terraform/dev/network", "terraform/dev/services/app"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := BuildPlan(stacks, capabilities("terraform", "helm"), tt.pick, "{{")
			require.NoError(t, err)
			var got []string
			for _, node := range plan.Graph.Nodes {
				got = append(got, Label(node))
			}
			slices.Sort(got)
			require.Equal(t, tt.want, got)
		})
	}
	_, err := BuildPlan(stacks, capabilities("terraform", "helm"), Selection{Stack: "dev", Component: "network", Reverse: true}, "{{")
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
}

func TestNamedAffectedSelectionFiltersDeletedNotices(t *testing.T) {
	plan, err := BuildPlan(nil, capabilities("terraform"), Selection{
		Component: "app",
		Affected: []schema.Affected{
			{ComponentType: "terraform", Stack: "dev", Component: "app", Deleted: true},
			{ComponentType: "terraform", Stack: "dev", Component: "network", Deleted: true},
		},
	}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	require.Equal(t, Notice{Kind: "terraform", Stack: "dev", Component: "app", Reason: deletedReason(false)}, plan.Notices[0])
}
