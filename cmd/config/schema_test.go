package config

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/datafetcher"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// configTestStreams is a minimal io.Streams implementation for capturing
// data output in tests, mirroring cmd/stack's stackConfigTestStreams.
type configTestStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *configTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *configTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *configTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *configTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *configTestStreams) RawError() stdio.Writer  { return ts.stderr }

// initConfigTestWriter wires a fresh data writer that captures stdout,
// cleaning up afterward. Needed because runConfigSchema writes through
// data.Write, which panics unless data.InitWriter was called.
func initConfigTestWriter(t *testing.T) *bytes.Buffer {
	t.Helper()

	streams := &configTestStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return streams.stdout
}

func TestConfigSchemaCmd_RegisteredUnderConfig(t *testing.T) {
	found := false
	for _, c := range configCmd.Commands() {
		if c.Name() == "schema" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"schema\" to be registered as a subcommand of \"config\"")
}

func TestRunConfigSchema_Stdout(t *testing.T) {
	stdout := initConfigTestWriter(t)

	require.NoError(t, runConfigSchema(nil))

	want, err := datafetcher.NewDataFetcher(nil).GetData(configSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stdout.String())
}

func TestConfigSchemaCmd_RunE_Stdout(t *testing.T) {
	stdout := initConfigTestWriter(t)

	require.NoError(t, configSchemaCmd.RunE(configSchemaCmd, nil))

	want, err := datafetcher.NewDataFetcher(nil).GetData(configSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stdout.String())
}

func TestRunConfigSchema_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "new-subdir", "atmos-config.json")

	require.NoError(t, runConfigSchema([]string{outputPath}))

	got, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	want, err := datafetcher.NewDataFetcher(nil).GetData(configSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestRunConfigSchema_ReturnsCreateDirectoryError(t *testing.T) {
	blockerPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockerPath, nil, 0o644))

	err := runConfigSchema([]string{filepath.Join(blockerPath, "atmos-config.json")})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateDirectory)
}

func TestRunConfigSchema_ReturnsWriteFileError(t *testing.T) {
	outputPath := t.TempDir()

	err := runConfigSchema([]string{outputPath})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWriteFile)
}
