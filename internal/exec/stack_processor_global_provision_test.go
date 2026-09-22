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

// TestGlobalWorkdirProvisionDefault_Inherited is the regression test for #3197: a global default
// for workdir provisioning is declared in the stack configuration under the toolchain section
// (`terraform.provision.workdir.enabled: true`), consistent with global `vars`, `metadata`, and
// `secrets`. A component that declares no component-level `provision` block must inherit it.
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
		"provision.workdir.enabled must be present from the stack-level terraform.provision default")
	assert.True(t, enabled,
		"a component with no component-level provision must inherit terraform.provision.workdir.enabled: true")
}

// TestGlobalWorkdirProvisionDefault_ComponentOverrides is the precedence guard: a component that
// explicitly sets provision.workdir.enabled: false must override the stack-level global default
// (enabled: true). This proves the stack-level default is the lowest-precedence layer, not a
// forced value.
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
		"component-level provision.workdir.enabled: false must override the stack-level global default")
}
