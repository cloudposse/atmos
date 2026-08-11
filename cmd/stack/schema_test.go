package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/datafetcher"
)

func TestStackSchemaCmd_RegisteredUnderStack(t *testing.T) {
	found := false
	for _, c := range stackCmd.Commands() {
		if c.Name() == "schema" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"schema\" to be registered as a subcommand of \"stack\"")
}

func TestRunStackSchema_Stdout(t *testing.T) {
	stdout := initStackConfigTestWriter(t)

	require.NoError(t, runStackSchema(nil))

	want, err := datafetcher.NewDataFetcher(nil).GetData(manifestSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stdout.String())
}

func TestStackSchemaCmd_RunE_Stdout(t *testing.T) {
	stdout := initStackConfigTestWriter(t)

	require.NoError(t, stackSchemaCmd.RunE(stackSchemaCmd, nil))

	want, err := datafetcher.NewDataFetcher(nil).GetData(manifestSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stdout.String())
}

func TestRunStackSchema_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "new-subdir", "atmos-manifest.json")

	require.NoError(t, runStackSchema([]string{outputPath}))

	got, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	want, err := datafetcher.NewDataFetcher(nil).GetData(manifestSchemaSource)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestRunStackSchema_ReturnsCreateDirectoryError(t *testing.T) {
	blockerPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockerPath, nil, 0o644))

	err := runStackSchema([]string{filepath.Join(blockerPath, "atmos-manifest.json")})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateDirectory)
}

func TestRunStackSchema_ReturnsWriteFileError(t *testing.T) {
	outputPath := t.TempDir()

	err := runStackSchema([]string{outputPath})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWriteFile)
}
