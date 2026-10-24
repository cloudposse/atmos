package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribeComponent_StackAssertedVersionTrack is the end-to-end guard for
// a stack asserting its Atmos Version Tracker track via a top-level
// `version: {track: ...}` section. Both `!version name` (a YAML function) and
// `{{ .version.name }}` (a Go-template context) inside `components.terraform.
// <name>.vars` must resolve against the STACK's asserted track, not the
// project-wide default. Regression coverage for a bug where ProcessStackConfig
// silently dropped the stack-level `version` section before it reached
// EffectiveTrackFromStack, so every stack silently fell back to the default
// track regardless of what it asserted.
//
// It exercises ExecuteDescribeComponent — the real stack-processing pipeline —
// rather than constructing schema.ConfigAndStacksInfo by hand, which is what
// let the original bug ship with green unit tests.
func TestDescribeComponent_StackAssertedVersionTrack(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/version-track-stack-assertion")

	dev, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "app",
		Stack:                "dev",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err)
	require.NotNil(t, dev)

	prod, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "app",
		Stack:                "prod",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err)
	require.NotNil(t, prod)

	devVars, ok := dev["vars"].(map[string]any)
	require.True(t, ok, "dev component must have a vars section, got: %v", dev["vars"])

	prodVars, ok := prod["vars"].(map[string]any)
	require.True(t, ok, "prod component must have a vars section, got: %v", prod["vars"])

	// !version widget must resolve against each stack's own asserted track.
	assert.Equal(t, "2.0.0", devVars["widget_version"], "dev stack must resolve !version against its own asserted track")
	assert.Equal(t, "1.0.0", prodVars["widget_version"], "prod stack must resolve !version against its own asserted track")

	// {{ .version.widget }} must resolve the same way.
	assert.Equal(t, "widget:2.0.0", devVars["widget_tag"])
	assert.Equal(t, "widget:1.0.0", prodVars["widget_tag"])
}
