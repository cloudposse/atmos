package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestGlobalWorkdirProvisionDefault is the regression test for #3197: a global default for workdir
// provisioning is declared in the stack configuration under the toolchain section
// (`terraform.provision.workdir.enabled: true`), consistent with global `vars`, `metadata`, and
// `secrets`. A component with no component-level `provision` block inherits it, while a component
// that explicitly sets `provision.workdir.enabled: false` overrides it - proving the stack-level
// default is the lowest-precedence layer, not a forced value.
func TestGlobalWorkdirProvisionDefault(t *testing.T) {
	tests := []struct {
		name      string
		component string
		want      bool
		reason    string
	}{
		{
			name:      "inherits stack-level default",
			component: "vpc-global",
			want:      true,
			reason:    "a component with no component-level provision must inherit terraform.provision.workdir.enabled: true",
		},
		{
			name:      "component overrides stack-level default",
			component: "vpc-opt-out",
			want:      false,
			reason:    "component-level provision.workdir.enabled: false must override the stack-level global default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("../../tests/fixtures/scenarios/workdir-global-default")

			section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
				Component:            tt.component,
				Stack:                "dev",
				ProcessTemplates:     false,
				ProcessYamlFunctions: false,
				Skip:                 []string{},
			})
			require.NoError(t, err)
			require.NotNil(t, section)

			enabled, present := workdirEnabledFromSection(t, section)
			require.True(t, present, "provision.workdir.enabled must be present")
			assert.Equal(t, tt.want, enabled, tt.reason)
		})
	}
}
