package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestDocsGenerateCmd_Error(t *testing.T) {
	err := docsGenerateCmd.RunE(docsGenerateCmd, []string{})
	assert.Error(t, err, "docs generate command should return an error when called with no parameters")
}

// TestDocsGenerateCmd_AbsoluteBase verifies the command preserves an absolute module directory.
func TestDocsGenerateCmd_AbsoluteBase(t *testing.T) {
	_ = NewTestKit(t)
	workingDir, moduleDir := t.TempDir(), t.TempDir()
	t.Chdir(workingDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workingDir)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(`variable "bucket" { type = string }`), 0o600))
	templatePath := filepath.Join(workingDir, "README.md.gomplate")
	require.NoError(t, os.WriteFile(templatePath, []byte(`{{ with terraform }}{{ range .Inputs }}{{ .Name }}{{ end }}{{ else }}unavailable{{ end }}`), 0o600))
	outputPath := filepath.Join(workingDir, "README.md")
	config := map[string]any{"docs": map[string]any{"generate": map[string]any{"absolute": map[string]any{
		"base-dir": moduleDir, "template": templatePath, "output": outputPath, "terraform": map[string]any{"enabled": true, "source": "."},
	}}}}
	content, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "atmos.yaml"), content, 0o600))
	require.NoError(t, docsGenerateCmd.RunE(docsGenerateCmd, []string{"absolute"}))
	actual, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "bucket", string(actual))
}
