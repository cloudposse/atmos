package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// workdirEnabledFromSection reads componentSection["provision"]["workdir"]["enabled"].
// The second return reports whether the key was present as a bool.
func workdirEnabledFromSection(t *testing.T, section map[string]any) (bool, bool) {
	t.Helper()
	provision, ok := section["provision"].(map[string]any)
	if !ok {
		return false, false
	}
	workdir, ok := provision["workdir"].(map[string]any)
	if !ok {
		return false, false
	}
	enabled, ok := workdir["enabled"].(bool)
	return enabled, ok
}

// TestGlobalWorkdirProvisionDefault_Inherited is the regression test for #3197: the documented
// global default `settings.provision.workdir.enabled` (set in atmos.yaml) must be honored by a
// component that declares no component-level provision block. Before the fix the global default
// was ignored (only component-level `provision.workdir.enabled` worked), so the resolved
// component had no provision.workdir.enabled.
func TestGlobalWorkdirProvisionDefault_Inherited(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/workdir-global-default")

	section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "vpc-global",
		Stack:                "dev",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
	})
	require.NoError(t, err)
	require.NotNil(t, section)

	enabled, present := workdirEnabledFromSection(t, section)
	require.True(t, present,
		"provision.workdir.enabled must be present from the global settings.provision default")
	assert.True(t, enabled,
		"a component with no component-level provision must inherit settings.provision.workdir.enabled: true")
}

// TestGlobalWorkdirProvisionDefault_ComponentOverrides is the precedence guard: a component that
// explicitly sets provision.workdir.enabled: false must override the global default (enabled:
// true). This proves the global default is the lowest-precedence layer, not a forced value.
func TestGlobalWorkdirProvisionDefault_ComponentOverrides(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/workdir-global-default")

	section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "vpc-opt-out",
		Stack:                "dev",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
	})
	require.NoError(t, err)
	require.NotNil(t, section)

	enabled, present := workdirEnabledFromSection(t, section)
	require.True(t, present, "provision.workdir.enabled must be present")
	assert.False(t, enabled,
		"component-level provision.workdir.enabled: false must override the global default")
}

// TestGlobalWorkdirProvisionDefaults_Helper unit-tests the projection of the atmos.yaml
// settings.provision.workdir block into a provision map.
func TestGlobalWorkdirProvisionDefaults_Helper(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		ttl     string
		want    map[string]any
	}{
		{name: "unset returns nil", want: nil},
		{name: "enabled only", enabled: true, want: map[string]any{"workdir": map[string]any{"enabled": true}}},
		{name: "ttl only", ttl: "7d", want: map[string]any{"workdir": map[string]any{"ttl": "7d"}}},
		{
			name: "enabled and ttl", enabled: true, ttl: "24h",
			want: map[string]any{"workdir": map[string]any{"enabled": true, "ttl": "24h"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Settings.Provision.Workdir.Enabled = tt.enabled
			atmosConfig.Settings.Provision.Workdir.TTL = tt.ttl
			assert.Equal(t, tt.want, globalWorkdirProvisionDefaults(atmosConfig))
		})
	}

	assert.Nil(t, globalWorkdirProvisionDefaults(nil), "nil config must return nil")
}
