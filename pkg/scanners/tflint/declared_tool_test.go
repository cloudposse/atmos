package tflint

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithOnlyComponentDeclaredTFLint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX shell")
	}
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PATH", t.TempDir())
	config := schema.AtmosConfiguration{BasePath: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.BasePath = "components"
	config.Toolchain.InstallPath = filepath.Join(root, "tools")
	config.Toolchain.Install = schema.ToolchainInstallNever
	config.Toolchain.Aliases = map[string]string{"tflint": "terraform-linters/tflint"}
	binary := filepath.Join(config.Toolchain.InstallPath, "bin", "terraform-linters", "tflint", "0.59.1", "tflint")
	require.NoError(t, os.MkdirAll(filepath.Dir(binary), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "app"), 0o755))
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' '{\"version\":\"2.1.0\",\"runs\":[{\"tool\":{\"driver\":{\"name\":\"tflint\"}},\"results\":[]}]}'\n"), 0o700))
	originalInit := initCLIConfig
	initCLIConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return config, nil }
	t.Cleanup(func() { initCLIConfig = originalInit })
	component := map[string]any{"dependencies": map[string]any{"tools": map[string]any{"tflint": "0.59.1"}}}
	rt := testRuntime()
	rt.DescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager, bool) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{"app": component}}}}, nil
	}
	rt.ProcessStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _ bool, _ bool, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.FinalComponent = "app"
		info.ComponentSection = component
		return info, nil
	}
	require.NoError(t, Execute(context.Background(), rt, &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}, nil, 0))
}
