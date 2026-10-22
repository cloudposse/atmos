package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginBuildStreamsDiagnosticsAndPreservesPreviousBinary(t *testing.T) {
	_ = NewTestKit(t)
	directory := t.TempDir()
	environment := pluginTestEnvironment(t, directory)
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, content := range map[string]string{
		"plugin.yaml": `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/build-output, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities:
    commands:
      about: {description: Build fixture}
`,
		"go.mod":  "module example.com/build-output\n",
		"main.go": "package main\nfunc main() {}\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600))
	}
	run := func() (string, string, error) {
		t.Helper()
		command := exec.CommandContext(t.Context(), executable, "plugin", "build")
		command.Dir, command.Env = directory, environment
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}
	stdout, stderr, err := run()
	require.NoError(t, err, stderr)
	require.Empty(t, stdout, "build progress belongs on the UI channel")
	require.Contains(t, stderr, "Plugin built successfully")
	destination := filepath.Join(directory, "bin", "plugin")
	previous, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\nfunc main() { missingCompilerSymbol() }\n"), 0o600))
	stdout, stderr, err = run()
	require.Error(t, err)
	require.Empty(t, stdout, "compiler failures must leave stdout clean")
	require.Contains(t, stderr, "undefined: missingCompilerSymbol")
	require.Contains(t, stderr, "Compiling plugin failed")
	require.NotContains(t, stderr, "Plugin built successfully")
	current, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, previous, current, "the previous successful binary remains usable")
}
