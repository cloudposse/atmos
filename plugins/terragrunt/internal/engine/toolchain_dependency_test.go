package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readCallLog(t *testing.T, path string) []toolchainReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var calls []toolchainReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var call toolchainReport
		require.NoError(t, decoder.Decode(&call))
		calls = append(calls, call)
	}
	return calls
}

// A Terragrunt dependency read runs the selected absolute Terraform/OpenTofu executable
// with the invocation environment: that executable is not on any PATH, the dependency's
// own run_cmd uses the toolchain PATH, and nothing from the ambient process returns.
func TestDependencyOutputReadsUseSelectedExecutableAndInvocationEnvironment(t *testing.T) {
	layout := newToolchainLayout(t, `dependency "network" {
  config_path = "../network"
}
inputs = { network = dependency.network.outputs.report }
`)
	network := filepath.Join(layout.root, "network")
	require.NoError(t, os.MkdirAll(network, 0o755))
	source, err := json.Marshal(filepath.ToSlash(layout.module))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(network, "terragrunt.hcl"), []byte("terraform { source = "+string(source)+` }
locals { identity = jsondecode(run_cmd("fixture-tool")).profile }
remote_state {
  backend = "local"
  config  = { path = "${local.identity}.tfstate" }
}
`), 0o644))
	toolchain := installFixtureExecutable(t, filepath.Join(layout.root, "toolchain", "bin"), toolchainToolName)
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	var observed []toolchainReport
	for _, profile := range []string{"production", "development"} {
		log := filepath.Join(layout.root, profile+".jsonl")
		path := filepath.Dir(toolchain) + string(os.PathListSeparator) + layout.empty
		env := map[string]string{"PATH": path, "AWS_PROFILE": profile, toolchainChildEnv: "1", toolchainCallLog: log}
		result, err := Resolve(t.Context(), layout.request(env))
		require.NoError(t, err)

		// The dependency output reached the unit's inputs from the selected executable.
		output := mustReport(t, result.Inputs["network"])
		require.Equal(t, []string{"output", "-json"}, output.Arguments)
		require.Equal(t, physicalPath(t, layout.tofu), physicalPath(t, output.Executable))

		calls := readCallLog(t, log)
		require.NotEmpty(t, calls)
		tofuCalls, toolCalls := 0, 0
		for _, call := range calls {
			require.Equal(t, profile, call.Profile, "identity must reach every child: %v", call.Arguments)
			require.Equal(t, path, call.Path, "toolchain PATH must reach every child: %v", call.Arguments)
			require.Empty(t, call.Token, "ambient credentials must not return")
			if physicalPath(t, call.Executable) == physicalPath(t, layout.tofu) {
				tofuCalls++
				continue
			}
			toolCalls++
			require.Equal(t, physicalPath(t, toolchain), physicalPath(t, call.Executable))
			require.Equal(t, physicalPath(t, network), physicalPath(t, call.Directory), "a dependency command runs in the dependency directory")
		}
		require.GreaterOrEqual(t, tofuCalls, 2, "init and output both use the selected executable")
		require.Positive(t, toolCalls, "the dependency's own HCL command must find the toolchain tool")
		observed = append(observed, output)
	}
	// Consecutive identities do not share cached dependency outputs.
	require.Equal(t, "production", observed[0].Profile)
	require.Equal(t, "development", observed[1].Profile)
}

// Terragrunt hooks belong to command execution, never to HCL evaluation. Whether the
// evaluator rejects them or reports them for the host to run, evaluating a unit must
// not spawn the hook command, including a hook that names an explicit working directory.
func TestEvaluationNeverRunsHooks(t *testing.T) {
	layout := newToolchainLayout(t, "")
	source, err := json.Marshal(filepath.ToSlash(layout.module))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layout.unit, "terragrunt.hcl"), []byte("terraform {\n  source = "+string(source)+`
  before_hook "before" {
    commands    = ["plan"]
    execute     = ["fixture-tool", "before"]
    working_dir = "."
  }
  after_hook "after" {
    commands = ["plan"]
    execute  = ["./fixture-tool", "after"]
  }
}
inputs = { value = "constant" }
`), 0o644))
	toolchain := installFixtureExecutable(t, filepath.Join(layout.root, "toolchain", "bin"), toolchainToolName)
	installFixtureExecutable(t, layout.unit, toolchainToolName)
	log := filepath.Join(layout.root, "hooks.jsonl")
	env := map[string]string{"PATH": filepath.Dir(toolchain), toolchainChildEnv: "1", toolchainCallLog: log}
	_, err = Resolve(t.Context(), layout.request(env))
	if err != nil {
		// The hooks must have been parsed; a syntax error would make this test vacuous.
		require.ErrorContains(t, err, "hook")
	}
	require.Empty(t, readCallLog(t, log), "hook commands must not run while evaluating configuration")
}
