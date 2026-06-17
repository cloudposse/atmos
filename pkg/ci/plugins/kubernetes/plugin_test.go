package kubernetes

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginHookBindings(t *testing.T) {
	p := &Plugin{}
	var events []string
	for _, binding := range p.GetHookBindings() {
		events = append(events, binding.Event)
	}

	assert.Contains(t, events, "after.kubernetes.plan")
	assert.Contains(t, events, "after.kubernetes.apply")
	assert.Contains(t, events, "after.kubernetes.deploy")
	assert.Contains(t, events, "after.kubernetes.delete")
	assert.Contains(t, events, "after.kubernetes.validate")
}

func TestRenderSummaryIncludesCountsObjectsAndErrors(t *testing.T) {
	p := &Plugin{}
	ctx := &plugin.HookContext{
		Command:        "plan",
		TemplateLoader: templates.NewLoader(nil),
		Aggregate: &schema.KubernetesCIResult{
			Stack:        "dev",
			Component:    "api",
			Command:      "plan",
			ObjectsTotal: 2,
			ExitCode:     1,
			Error:        "server dry-run failed",
			ActionCounts: map[string]int{"changed": 1, "create": 1},
			Actions: []schema.KubernetesObjectCIResult{
				{Action: "create", Resource: "v1/ConfigMap", Namespace: "default", Name: "settings"},
				{Action: "changed", Resource: "apps/v1/Deployment", Namespace: "default", Name: "api"},
			},
		},
	}

	rendered, err := p.renderSummary(ctx)
	require.NoError(t, err)

	assert.Contains(t, rendered, "## Kubernetes plan Summary for `api` in `dev`")
	assert.Contains(t, rendered, "Status: **failed**")
	assert.Contains(t, rendered, "| changed | 1 |")
	assert.Contains(t, rendered, "| create | 1 |")
	assert.Contains(t, rendered, "| create | v1/ConfigMap | default | settings |")
	assert.Contains(t, rendered, "server dry-run failed")
}

func TestNormalizeResultFallsBackToHookContext(t *testing.T) {
	ctx := &plugin.HookContext{
		Command:      "validate",
		ExitCode:     1,
		CommandError: errors.New("validation failed"),
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "api",
		},
	}

	result := normalizeResult(ctx)

	assert.Equal(t, "validate", result.Command)
	assert.Equal(t, "dev", result.Stack)
	assert.Equal(t, "api", result.Component)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "validation failed", result.Error)
}

func TestSummaryStatus(t *testing.T) {
	tests := []struct {
		name   string
		result *schema.KubernetesCIResult
		want   string
	}{
		{name: "failed by error", result: &schema.KubernetesCIResult{Error: "boom"}, want: "failed"},
		{name: "changed", result: &schema.KubernetesCIResult{ActionCounts: map[string]int{"changed": 1}}, want: "changed"},
		{name: "created", result: &schema.KubernetesCIResult{ActionCounts: map[string]int{"create": 1}}, want: "changed"},
		{name: "no change", result: &schema.KubernetesCIResult{ActionCounts: map[string]int{"no-change": 2}}, want: "no changes"},
		{name: "success", result: &schema.KubernetesCIResult{ActionCounts: map[string]int{"applied": 2}}, want: "succeeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, summaryStatus(tt.result))
		})
	}
}

func TestRenderSummaryDoesNotEmitOutputsContract(t *testing.T) {
	p := &Plugin{}
	rendered, err := p.renderSummary(&plugin.HookContext{
		Command:        "apply",
		TemplateLoader: templates.NewLoader(nil),
		Aggregate: schema.KubernetesCIResult{
			Stack:        "dev",
			Component:    "api",
			Command:      "apply",
			ActionCounts: map[string]int{"applied": 1},
		},
	})
	require.NoError(t, err)

	assert.NotContains(t, strings.ToLower(rendered), "github_output")
	assert.NotContains(t, rendered, "has_changes")
	assert.NotContains(t, rendered, "exit_code")
}
