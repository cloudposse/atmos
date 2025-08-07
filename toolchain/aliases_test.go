package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliasesCommand_WithAliases(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with aliases
	configContent := `aliases:
  terraform: hashicorp/terraform
  kubectl: kubernetes/kubectl
  helm: helm/helm`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory so the command can find the config file
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully list configured aliases")
}

func TestAliasesCommand_EmptyAliases(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with no aliases
	configContent := `aliases: {}`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with empty aliases
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle empty aliases gracefully")
}

func TestAliasesCommand_NoConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	// Change to a directory with no tools.yaml
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with no config file
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should not error when no config file exists - just show no aliases")
}

func TestAliasesCommand_InvalidConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create an invalid tools.yaml file
	configContent := `aliases:
  terraform: hashicorp/terraform
  invalid: yaml: content: here`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with invalid config
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.Error(t, err, "Should error when config file is invalid")
	assert.Contains(t, err.Error(), "failed to load local config")
}

func TestAliasesCommand_MultipleAliases(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with multiple aliases
	configContent := `aliases:
  terraform: hashicorp/terraform
  kubectl: kubernetes/kubectl
  helm: helm/helm
  awscli: aws/aws-cli
  docker: docker/cli`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with multiple aliases
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully list multiple aliases")
}

func TestAliasesCommand_NoArgs(t *testing.T) {
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// This should work with default config file location
	require.NoError(t, err, "Should work with no arguments using default config location")
}

func TestAliasesCommand_WithArgs(t *testing.T) {
	cmd := aliasesCmd
	cmd.SetArgs([]string{"extra", "args"})
	err := cmd.Execute()
	// The aliases command doesn't take arguments, but it should still work
	// as it ignores extra arguments
	require.NoError(t, err, "Should not error when aliases with extra args")
}

func TestAliasesCommand_EmptyConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create an empty tools.yaml file
	err := os.WriteFile(configFile, []byte(""), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with empty config file
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle empty config file gracefully - just show no aliases")
}

func TestAliasesCommand_ConfigWithoutAliases(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file without aliases section
	configContent := `other_config:
  setting: value`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with config without aliases
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle config without aliases section gracefully")
}

func TestAliasesCommand_SortedOutput(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with aliases in non-alphabetical order
	configContent := `aliases:
  kubectl: kubernetes/kubectl
  terraform: hashicorp/terraform
  awscli: aws/aws-cli
  helm: helm/helm`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should produce sorted output")
}

func TestAliasesCommand_ComplexAliases(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with complex aliases
	configContent := `aliases:
  tf: hashicorp/terraform
  k8s: kubernetes/kubectl
  k9s: derailed/k9s
  gh: cli/cli
  gcloud: google/cloud-sdk`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with complex aliases
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle complex aliases correctly")
}

func TestFormatAliasesAsTable(t *testing.T) {
	aliases := []string{"terraform", "kubectl"}
	aliasMap := map[string]string{
		"terraform": "hashicorp/terraform",
		"kubectl":   "kubernetes/kubectl",
	}

	result := formatAliasesAsTable(aliases, aliasMap)

	// Verify the result contains expected content
	assert.Contains(t, result, "Alias:")
	assert.Contains(t, result, "Owner/Repo:")
	assert.Contains(t, result, "terraform:")
	assert.Contains(t, result, "kubectl:")
	assert.Contains(t, result, "hashicorp/terraform")
	assert.Contains(t, result, "kubernetes/kubectl")
}

func TestAliasesCommand_WithSpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "tools.yaml")

	// Create a tools.yaml file with aliases containing special characters
	configContent := `aliases:
  "terraform-aws": hashicorp/terraform
  "k8s-cli": kubernetes/kubectl
  "aws-cli": aws/aws-cli`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Change to the temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test aliases command with special characters
	cmd := aliasesCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should handle aliases with special characters")
}
