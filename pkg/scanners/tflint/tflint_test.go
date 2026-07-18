package tflint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigPathPrefersComponentConfig(t *testing.T) {
	base := t.TempDir()
	componentPath := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".tflint.hcl"), []byte("plugin \"terraform\" {}"), 0o600))

	config := &schema.AtmosConfiguration{BasePathAbsolute: base}
	config.TerraformDirAbsolutePath = filepath.Join(base, "components", "terraform")
	config.Components.Terraform.Lint.Config = "config/.tflint.hcl"
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}

	require.Equal(t, filepath.Join(componentPath, ".tflint.hcl"), ConfigPath(config, info))
}

func TestConfigPathUsesConfiguredGlobalFallback(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: base}
	config.TerraformDirAbsolutePath = filepath.Join(base, "components", "terraform")
	config.Components.Terraform.Lint.Config = ".tflint.hcl"
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}

	require.Equal(t, filepath.Join(base, ".tflint.hcl"), ConfigPath(config, info))
	require.Equal(
		t,
		[]string{"--chdir=$ATMOS_COMPONENT_PATH", "--format=sarif", "--config=" + filepath.Join(base, ".tflint.hcl")},
		ResolveArgs(DefaultArgs(), config, info),
	)
}

func TestConfigPathUsesMostSpecificStandardLocation(t *testing.T) {
	base := t.TempDir()
	componentsPath := filepath.Join(base, "components", "terraform")
	componentPath := filepath.Join(componentsPath, "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".tflint.hcl"), []byte("root"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(componentsPath, ".tflint.hcl"), []byte("base"), 0o600))

	config := &schema.AtmosConfiguration{BasePathAbsolute: base, TerraformDirAbsolutePath: componentsPath}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
	require.Equal(t, filepath.Join(componentsPath, ".tflint.hcl"), ConfigPath(config, info))

	require.NoError(t, os.Remove(filepath.Join(componentsPath, ".tflint.hcl")))
	require.Equal(t, filepath.Join(base, ".tflint.hcl"), ConfigPath(config, info))

	require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".tflint.hcl"), []byte("component"), 0o600))
	require.Equal(t, filepath.Join(componentPath, ".tflint.hcl"), ConfigPath(config, info))
}

func TestResolveArgsPreservesExplicitConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Lint.Config = "global.tflint.hcl"
	args := []string{"--config=custom.tflint.hcl", "--format=sarif"}

	require.Equal(t, args, ResolveArgs(args, config, &schema.ConfigAndStacksInfo{}))
}
