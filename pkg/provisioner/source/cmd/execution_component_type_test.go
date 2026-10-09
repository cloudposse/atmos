package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

func mixedSourceProviders(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	terraformDir := filepath.Join(base, "terraform-target")
	cloudFormationDir := filepath.Join(base, "cloudformation-target")
	for _, dir := range []string{terraformDir, cloudFormationDir, filepath.Join(base, "stacks")} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	config := `base_path: .
components:
  terraform:
    base_path: components/terraform
  aws/cloudformation:
    base_path: components/cloudformation
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
  name_template: "{{ .vars.stage }}"
`
	stack := fmt.Sprintf(`vars:
  stage: dev
components:
  terraform:
    shared:
      metadata:
        working_directory: %q
      source:
        uri: github.com/example/terraform
  aws/cloudformation:
    shared:
      metadata:
        working_directory: %q
      source:
        uri: github.com/example/cloudformation
`, terraformDir, cloudFormationDir)
	require.NoError(t, os.WriteFile(filepath.Join(base, "atmos.yaml"), []byte(config), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, "stacks", "dev.yaml"), []byte(stack), 0o600))
	t.Chdir(base)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", base)
	t.Setenv("ATMOS_BASE_PATH", base)
	e.ClearBaseComponentConfigCache()
	e.ClearFileContentCache()
	t.Cleanup(func() { e.ClearBaseComponentConfigCache(); e.ClearFileContentCache() })
	return terraformDir, cloudFormationDir
}

func TestSourceDelete_MixedProviders(t *testing.T) {
	terraformDir, cloudFormationDir := mixedSourceProviders(t)
	require.NoError(t, source.WriteProvenance(cloudFormationDir, &source.Provenance{Component: "shared", Source: "github.com/example/cloudformation"}))
	cmd := DeleteCommand(&Config{ComponentType: "aws/cloudformation"})
	cmd.SetArgs([]string{"shared", "--stack", "dev", "--force"})
	require.NoError(t, cmd.Execute())
	assert.NoDirExists(t, cloudFormationDir)
	assert.DirExists(t, terraformDir)
}

func TestSourceDescribe_MixedProviders(t *testing.T) {
	mixedSourceProviders(t)
	for _, componentType := range []string{"aws/cloudformation", "terraform"} {
		t.Run(componentType, func(t *testing.T) {
			cmd := DescribeCommand(&Config{ComponentType: componentType})
			cmd.SetArgs([]string{"shared", "--stack", "dev"})
			output := captureDescribeStdout(t, func() { require.NoError(t, cmd.Execute()) })
			expectedURI := "github.com/example/" + strings.TrimPrefix(componentType, "aws/")
			assert.Contains(t, output, expectedURI)
		})
	}
}

func TestSourcePull_MixedProviders(t *testing.T) {
	mixedSourceProviders(t)
	originalMerge, originalCreate, originalProvision := mergeAuthFunc, createAuthFunc, provisionSourceFunc
	t.Cleanup(func() {
		mergeAuthFunc, createAuthFunc, provisionSourceFunc = originalMerge, originalCreate, originalProvision
	})
	for _, componentType := range []string{"aws/cloudformation", "terraform"} {
		t.Run(componentType, func(t *testing.T) {
			expectedURI := "github.com/example/" + strings.TrimPrefix(componentType, "aws/")
			authLookups, provisions := 0, 0
			mergeAuthFunc = func(_ *schema.AuthConfig, component map[string]any, _ *schema.AtmosConfiguration, _ string) (*schema.AuthConfig, error) {
				authLookups++
				assert.Equal(t, expectedURI, component["source"].(map[string]any)["uri"])
				return &schema.AuthConfig{}, nil
			}
			createAuthFunc = func(string, *schema.AuthConfig, string) (auth.AuthManager, error) { return nil, nil }
			provisionSourceFunc = func(_ context.Context, params *source.ProvisionParams) error {
				provisions++
				assert.Equal(t, componentType, params.ComponentType)
				assert.Equal(t, expectedURI, params.ComponentConfig["source"].(map[string]any)["uri"])
				return nil
			}
			cmd := PullCommand(&Config{ComponentType: componentType})
			cmd.SetArgs([]string{"shared", "--stack", "dev", "--force"})
			require.NoError(t, cmd.Execute())
			assert.Equal(t, 1, authLookups)
			assert.Equal(t, 1, provisions)
		})
	}
}

func TestSourceDryRun_MixedProviders(t *testing.T) {
	terraformDir, cloudFormationDir := mixedSourceProviders(t)
	require.NoError(t, source.WriteProvenance(cloudFormationDir, &source.Provenance{Component: "shared", Source: "github.com/example/cloudformation"}))
	originalDescribe := describeTypedComponentFunc
	t.Cleanup(func() { describeTypedComponentFunc = originalDescribe })
	for _, verb := range []string{"pull", "delete"} {
		t.Run(verb, func(t *testing.T) {
			lookups := 0
			describeTypedComponentFunc = func(componentType, component, stack string) (map[string]any, error) {
				lookups++
				assert.Equal(t, "aws/cloudformation", componentType)
				config, err := originalDescribe(componentType, component, stack)
				require.NoError(t, err)
				assert.Equal(t, "github.com/example/cloudformation", config["source"].(map[string]any)["uri"])
				return config, nil
			}
			cmd := mountSourceCommand(t, verb, true)
			cmd.SetArgs([]string{"source", verb, "shared", "--stack", "dev", "--dry-run"})
			require.NoError(t, cmd.Execute())
			assert.Equal(t, 1, lookups)
			assert.DirExists(t, terraformDir)
			assert.DirExists(t, cloudFormationDir)
		})
	}
}
