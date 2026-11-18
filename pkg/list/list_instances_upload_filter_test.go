package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func proSettings(enabled bool) map[string]any {
	return map[string]any{"pro": map[string]any{"enabled": enabled}}
}

// Every instance is uploaded with its component type and resolved Pro state,
// regardless of type, so Atmos Pro can show it and learn when it is disabled.
func TestBuildUploadInstances_ReportsAllTypesWithEnabledState(t *testing.T) {
	instances := []schema.Instance{
		{Component: "tf-off", Stack: "dev", ComponentType: "terraform", Settings: proSettings(false)},
		{Component: "ans-off", Stack: "dev", ComponentType: "ansible", Settings: proSettings(false)},
		{Component: "ans-on", Stack: "dev", ComponentType: "ansible", Settings: proSettings(true)},
		{Component: "ans-meta", Stack: "dev", ComponentType: "ansible", Settings: proSettings(true), Metadata: map[string]any{"enabled": false}},
	}

	got := buildUploadInstances(instances)

	assert.Len(t, got, len(instances))
	want := map[string]bool{"tf-off": false, "ans-off": false, "ans-on": true, "ans-meta": false}
	for _, g := range got {
		pro, _ := g.Settings["pro"].(map[string]any)
		assert.Equal(t, want[g.Component], pro["enabled"], g.Component)
	}
	assert.Equal(t, "ansible", got[1].ComponentType)
}
