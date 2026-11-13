package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionalTflintVarFilesResolveAtHookExecution(t *testing.T) {
	moduleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "module.tfvars"), nil, 0o644))
	unitDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "unit.tfvars"), nil, 0o644))
	absolute := filepath.Join(unitDir, "unit.tfvars")
	arguments := []string{"--config", ".tflint.hcl", tflintOptionalVarFilePrefix + "module.tfvars", tflintOptionalVarFilePrefix + "missing.tfvars", tflintOptionalVarFilePrefix + absolute}
	require.Equal(t, []string{"--config", ".tflint.hcl", "--var-file=module.tfvars", "--var-file=" + absolute},
		resolveOptionalTflintVarFiles(arguments, moduleDir))
}

func TestTflintConfigLookupWaitsForHookExecution(t *testing.T) {
	unitDir := t.TempDir()
	arguments := []string{"--config", tflintConfigSearchPrefix + unitDir, "--chdir", "."}
	// Preparation can finish before a previous hook generates the config.
	config := filepath.Join(unitDir, ".tflint.hcl")
	require.NoError(t, os.WriteFile(config, []byte("plugin \"aws\" {}"), 0o644))
	resolved, err := resolveTflintHookArguments(arguments, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, []string{"--config", config, "--chdir", "."}, resolved)
}
