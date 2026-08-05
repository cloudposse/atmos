package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestConfigValidateCmd_RegisteredUnderConfig(t *testing.T) {
	found := false
	for _, c := range configCmd.Commands() {
		if c.Name() == "validate" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"validate\" to be registered as a subcommand of \"config\"")
}

func TestConfigValidateCmdHasAffectedFlags(t *testing.T) {
	assert.NotNil(t, configValidateCmd.Flags().Lookup("affected"))
	assert.NotNil(t, configValidateCmd.Flags().Lookup("base"))
}

func TestRunConfigValidate_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("logs:\n  level: Debug\n"), 0o644))
	t.Chdir(dir)

	require.NoError(t, runConfigValidate())
}

func TestRunConfigValidate_InvalidConfigExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("logs:\n  level: 42\n"), 0o644))
	t.Chdir(dir)

	exitCode := 0
	originalOsExit := errUtils.OsExit
	errUtils.OsExit = func(code int) { exitCode = code }
	defer func() { errUtils.OsExit = originalOsExit }()

	_ = runConfigValidate()

	assert.Equal(t, 1, exitCode, "a schema violation must exit with code 1, matching `atmos validate schema config`")
}
