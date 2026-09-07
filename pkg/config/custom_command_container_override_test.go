package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandContainerStepBoolOptOutDoesNotBreakConfigLoad reproduces a
// field-test finding sibling to https://github.com/cloudposse/atmos/issues/2876:
// a custom command step with a bare boolean `container: false` opt-out --
// valid, documented syntax for a workflow-file step (WorkflowContainer.
// UnmarshalYAML, pkg/schema/workflow.go) -- previously broke InitCliConfig
// outright for the ENTIRE atmos.yaml, not just this one command, because
// decodeTaskFromMap left `container:` for the plain mapstructure struct
// decode, which rejects a bool for a struct-typed field. Every Atmos
// command (`atmos describe stacks`, `atmos list stacks`, unrelated custom
// commands, etc.) failed identically as long as this step existed
// anywhere in the merged config tree.
func TestCustomCommandContainerStepBoolOptOutDoesNotBreakConfigLoad(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	atmosYAML := `
base_path: "."
commands:
  - name: opt-out
    description: Run on the host, opting out of any ambient container sandbox
    steps:
      - name: run
        command: echo hello
        container: false
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tempDir)
	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err, "a step-level `container: false` opt-out must not break loading the entire atmos.yaml")

	require.Len(t, cfg.Commands, 1)
	require.Len(t, cfg.Commands[0].Steps, 1)
	step := cfg.Commands[0].Steps[0]

	require.NotNil(t, step.Container)
	assert.False(t, step.Container.IsEnabled())
}

// TestCustomCommandContainerStepMappingOverrideDecodesFully reproduces the
// mapping-form sibling of the same bug: `container: {image: ..., ...}` on a
// custom command step decoded without error even before this fix, but the
// struct fields never went through WorkflowContainer.UnmarshalYAML the way
// the workflow-file path does, and downstream execution wiring never
// consulted the field at all (see cmd/cmd_utils.go), so the step silently
// ran on the bare host regardless of what the mapping contained. This test
// covers the decode half of that gap: the step's Container must be fully
// and correctly populated from the real production config-loading path.
func TestCustomCommandContainerStepMappingOverrideDecodesFully(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	atmosYAML := `
base_path: "."
commands:
  - name: sandboxed
    description: Run inside a step-level container override
    steps:
      - name: run
        command: echo hello
        container:
          image: alpine:3.20
          provider: docker
          workspace: /workspace
          env:
            FOO: bar
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tempDir)
	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	require.Len(t, cfg.Commands, 1)
	require.Len(t, cfg.Commands[0].Steps, 1)
	step := cfg.Commands[0].Steps[0]

	require.NotNil(t, step.Container,
		"container: mapping block must decode into the typed WorkflowContainer, not be silently dropped")
	assert.True(t, step.Container.IsEnabled())
	assert.Equal(t, "alpine:3.20", step.Container.Image)
	assert.Equal(t, "docker", step.Container.Provider)
	assert.Equal(t, "/workspace", step.Container.Workspace)
	assert.Equal(t, map[string]string{"FOO": "bar"}, step.Container.Env)
}
