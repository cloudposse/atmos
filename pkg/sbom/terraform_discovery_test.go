package sbom

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformModuleOnlyConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX shell")
	}
	base := t.TempDir()
	root := filepath.Join(base, "components", "terraform", "module-only")
	module := filepath.Join(root, "modules", "child")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte(`output "value" { value = "child" }`), 0o600))
	manifestDir := filepath.Join(root, ".terraform", "modules")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "modules.json"), []byte(`{"Modules":[]}`), 0o600))
	executable := filepath.Join(base, "terraform")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\n[ \"$1 $2\" = 'modules -json' ] || exit 2\nprintf '%s' '{\"format_version\":\"1.0\",\"modules\":[{\"key\":\"child\",\"source\":\"./modules/child\"}]}'\n"), 0o700))
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
	for _, coverage := range graph.Coverage {
		if coverage.Adapter == "terraform-modules" {
			require.Equal(t, "complete", coverage.Status)
			require.Equal(t, "module graphs parsed", coverage.Detail)
		}
	}
	directories, err := findTerraformConfigurations(filepath.Join(base, "components", "terraform"), []string{filepath.Join(root, ".terraform.lock.hcl")})
	require.NoError(t, err)
	require.Equal(t, []string{root}, directories, "lock and module manifest must not duplicate roots")
}
