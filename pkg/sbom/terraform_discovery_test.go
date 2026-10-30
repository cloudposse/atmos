package sbom

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const terraformModuleFixture = `{"format_version":"1.0","modules":[{"key":"child","source":"./modules/child"}]}`

var (
	_ = schema.AtmosConfiguration{BasePath: "", Components: schema.Components{}}
	_ = schema.Components{Terraform: schema.Terraform{}}
	_ = schema.Terraform{BasePath: "", Command: ""}
)

func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_SBOM_TERRAFORM") == "1" {
		if len(os.Args) != 3 || os.Args[1] != "modules" || os.Args[2] != "-json" {
			os.Exit(2)
		}
		if _, err := os.Stdout.WriteString(terraformModuleFixture); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestTerraformModuleOnlyConfiguration(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "components", "terraform", "module-only")
	module := filepath.Join(root, "modules", "child")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte(`output "value" { value = "child" }`), 0o600))
	manifestDir := filepath.Join(root, ".terraform", "modules")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "modules.json"), []byte(`{"Modules":[]}`), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("_ATMOS_TEST_SBOM_TERRAFORM", "1")
	t.Run("subprocess inherits fixture environment", func(t *testing.T) {
		output, err := exec.Command(executable, "modules", "-json").Output()
		require.NoError(t, err)
		require.JSONEq(t, terraformModuleFixture, string(output))
	})
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = "components/terraform"
	config.Components.Terraform.Command = executable
	graph := &Graph{}
	require.NoError(t, appendTerraform(t.Context(), graph, config))
	require.Contains(t, componentIDs(graph), "terraform:module-only")
	require.Contains(t, componentIDs(graph), "terraform-module:terraform:module-only:child")
	for _, component := range graph.Components {
		if component.ID == "terraform:module-only" {
			require.NotContains(t, component.Properties, "atmos:lock-file")
		}
	}
	foundModules := false
	for _, coverage := range graph.Coverage {
		if coverage.Adapter == "terraform-modules" {
			foundModules = true
			require.Equal(t, "complete", coverage.Status)
			require.Equal(t, "module graphs parsed", coverage.Detail)
		}
	}
	require.True(t, foundModules, "module coverage must be reported")
	directories, err := findTerraformConfigurations(filepath.Join(base, "components", "terraform"), []string{filepath.Join(root, ".terraform.lock.hcl")})
	require.NoError(t, err)
	require.Equal(t, []string{root}, directories, "lock and module manifest must not duplicate roots")
}
