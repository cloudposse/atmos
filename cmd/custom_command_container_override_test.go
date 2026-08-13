package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestCustomCommandStepContainerOverrideRunsInsideContainer is the execution-wiring
// sibling of TestCustomCommandContainerBuildPassesWithBlockToDocker: a custom
// command's ordinary `type: shell` (default) step with a step-level
// `container:` override must actually execute inside a sandboxed container,
// the same way the identical `container:` block on a workflow-file step does
// (see internal/exec/workflow_utils.go's `case workflowPkg.StepContainerOverride`).
//
// Before this fix, cmd/cmd_utils.go's step-execution loop dispatched every
// `type: shell` step straight to process.RunShellStep on the host and never
// consulted step.Container at all -- the block decoded (once the pkg/schema/task.go
// fix landed) but was silently ignored at execution time. This test proves the
// step reaches the container runtime: a fake, logging `docker` executable on
// PATH (testhelpers.InstallFakeContainerRuntime) records the real argv Atmos
// emits, so a `docker create ...` followed by a `docker exec ... /bin/sh -lc
// "<command>"` invocation must appear -- not nothing (which is what a
// host-only RunShellStep would leave behind, since RunShellStep never
// touches docker).
//
// This writes real config to disk, loads it through cfg.InitCliConfig (the
// same production config-loading path `atmos` itself uses), registers it via
// processCustomCommands, and invokes the resulting custom command through
// RootCmd.Execute() exactly as a user would from the shell.
func TestCustomCommandStepContainerOverrideRunsInsideContainer(t *testing.T) {
	_ = NewTestKit(t)

	tempDir := t.TempDir()

	atmosYAML := `
base_path: "."
commands:
  - name: test-container-step-override
    description: Run a step inside a step-level container override
    steps:
      - name: run-in-container
        command: echo host-should-not-run-this
        container:
          image: alpine
          provider: docker
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
	t.Setenv("ATMOS_BASE_PATH", tempDir)
	t.Chdir(tempDir)

	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: "docker",
		Mode: testhelpers.FakeContainerRuntimeStep,
	})

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))

	RootCmd.SetArgs([]string{"test-container-step-override"})
	require.NoError(t, RootCmd.Execute())

	content, err := os.ReadFile(argsPath)
	require.NoError(t, err, "the fake docker executable must have been invoked at least once -- "+
		"the step must run inside the container, not on the host")
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")

	// The container is created (persistent-style, like a workflow-level
	// sandbox) then the step's command is issued as a separate `docker exec`
	// against it -- mirroring RunEphemeralContainer's create/start/exec
	// sequence -- so the step's actual command shows up on the `exec` line,
	// not `create`.
	var createLine, execLine string
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "create":
			createLine = line
		case "exec":
			execLine = line
		}
	}
	require.NotEmpty(t, createLine, "expected a `docker create ...` invocation; got invocations: %v", lines)
	require.NotEmpty(t, execLine, "expected a `docker exec ...` invocation; got invocations: %v", lines)

	createFields := strings.Split(createLine, "\t")
	assert.Contains(t, createFields, "alpine", "configured container image must be applied")

	execFields := strings.Split(execLine, "\t")
	assert.Contains(t, execFields, "/bin/sh", "step command must be wrapped for shell execution inside the container")
	assert.Contains(t, execFields, "-lc")
	assert.Contains(t, execFields, "echo host-should-not-run-this", "the step's own command must reach the container")
}

// TestCustomCommandStepContainerFalseOptOutRunsOnHost confirms the boolean
// `container: false` form -- previously breaking InitCliConfig entirely for
// the whole atmos.yaml (see pkg/config's
// TestCustomCommandContainerStepBoolOptOutDoesNotBreakConfigLoad) -- not only
// loads but still runs the step on the host as documented (no container
// override, no ambient block to opt out of for custom commands, so this is a
// pure no-op that must not regress into unexpectedly invoking docker).
func TestCustomCommandStepContainerFalseOptOutRunsOnHost(t *testing.T) {
	_ = NewTestKit(t)

	tempDir := t.TempDir()

	markerPath := filepath.Join(tempDir, "ran-on-host.txt")
	atmosYAML := `
base_path: "."
commands:
  - name: test-container-step-optout
    description: Explicitly opt out of any container sandbox
    steps:
      - name: run
        command: echo host > "` + filepath.ToSlash(markerPath) + `"
        container: false
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
	t.Setenv("ATMOS_BASE_PATH", tempDir)
	t.Chdir(tempDir)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "container: false must not break loading the entire atmos.yaml")

	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))

	RootCmd.SetArgs([]string{"test-container-step-optout"})
	require.NoError(t, RootCmd.Execute())

	content, err := os.ReadFile(markerPath)
	require.NoError(t, err, "the step must have run on the host")
	assert.Equal(t, "host\n", string(content))
}
