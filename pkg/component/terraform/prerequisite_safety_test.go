package terraform

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
)

func TestDryRunInitPreservesWorkspaceAndMarker(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit", true: "explicit"}[explicit], func(t *testing.T) {
			root := t.TempDir()
			environment := filepath.Join(root, ".terraform", "environment")
			marker := autoinit.MarkerPath(filepath.Join(root, ".terraform"))
			require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
			require.NoError(t, os.WriteFile(environment, []byte("dev"), 0o600))
			require.NoError(t, os.WriteFile(marker, []byte("existing marker"), 0o600))
			info := &schema.ConfigAndStacksInfo{DryRun: true, Command: "must-not-launch", SubCommand: "plan", ComponentSection: map[string]any{}}
			config := &schema.AtmosConfiguration{}
			_, err := prepareInitExecution(context.Background(), provisioner.OutputWriters{}, config, info, root)
			require.NoError(t, err)
			if explicit {
				info.SubCommand = "init"
				err = testExecutor(t).runMainTerraformCommand(config, info, []string{"init"}, root)
			} else {
				err = testExecutor(t).executeTerraformInitCommand(config, info, root, "vars.json")
			}
			require.NoError(t, err)
			for file, expected := range map[string]string{environment: "dev", marker: "existing marker"} {
				content, readErr := os.ReadFile(file)
				require.NoError(t, readErr)
				assert.Equal(t, expected, string(content))
			}
		})
	}
}

func TestPrerequisiteInitRoutesDataCommandDiagnostics(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, command := range []string{"output", "show", "state", "plan", "init"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr, dataCapture, diagnostics bytes.Buffer
			info := &schema.ConfigAndStacksInfo{
				Command: executable, SubCommand: command, SubCommand2: "pull",
				ComponentEnvList: []string{"_ATMOS_TEST_STDOUT=initializing\n", "_ATMOS_TEST_STDERR=diagnostic\n"},
			}
			captured, err := testExecutor(t).runTerraformInitSubprocess(&schema.AtmosConfiguration{}, info, t.TempDir(), []string{"init"},
				component.WithProcessStreams(process.Streams{Stdout: &stdout, Stderr: &stderr}),
				component.WithStdoutCapture(&dataCapture), component.WithStderrCapture(&diagnostics))
			require.NoError(t, err)
			assert.Contains(t, captured, "initializing")
			if command == "plan" || command == "init" {
				assert.Contains(t, stdout.String(), "initializing")
				assert.Contains(t, dataCapture.String(), "initializing")
			} else {
				assert.Empty(t, stdout.String())
				assert.Empty(t, dataCapture.String())
				assert.Contains(t, stderr.String(), "initializing")
				assert.Contains(t, diagnostics.String(), "initializing")
			}
			assert.Contains(t, stderr.String(), "diagnostic")
			assert.Contains(t, diagnostics.String(), "diagnostic")
		})
	}
}

func TestPrerequisiteDiagnosticDestinationsSerializeConcurrentWrites(t *testing.T) {
	var output, diagnostics bytes.Buffer
	opts := prerequisiteOutputOptions(&schema.ConfigAndStacksInfo{SubCommand: "output"}, []component.ExecutionOption{
		component.WithProcessStreams(process.Streams{Stdout: &output, Stderr: &diagnostics}),
		component.WithStdoutCapture(&output), component.WithStderrCapture(&diagnostics),
	})
	var resolved component.ExecutionOptions
	for _, opt := range opts {
		opt(&resolved)
	}
	var writers sync.WaitGroup
	for _, writer := range []io.Writer{resolved.StdoutOverride, resolved.Streams.Stderr, resolved.StdoutCapture, resolved.StderrCapture} {
		writers.Go(func() {
			for range 1000 {
				_, err := writer.Write([]byte("diagnostic\n"))
				assert.NoError(t, err)
			}
		})
	}
	writers.Wait()
	assert.Empty(t, output.String())
	assert.Equal(t, 4000*len("diagnostic\n"), diagnostics.Len())
}
