package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func TestDiscoveryScopeAndPruning(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, name := range []string{"live/terragrunt.stack.hcl", "live/unit/terragrunt.hcl", "live/excluded/terragrunt.hcl", "live/.terragrunt-cache/terragrunt.hcl", "live/node_modules/terragrunt.hcl", "catalog/terragrunt.hcl"} {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))
	}
	stacks, units, err := discoveryPaths(root, []string{"live/**", "live/unit/**"}, []string{"live/excluded/**"})
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(root, "live/terragrunt.stack.hcl")}, stacks)
	require.Equal(t, []string{filepath.Join(root, "live/unit/terragrunt.hcl")}, units)
	_, _, err = discoveryPaths(root, []string{"absent/**"}, nil)
	require.NoError(t, err)
	for _, pattern := range []string{"", "[", "../**", `live\**`, filepath.Join(root, "**")} {
		_, _, err := discoveryPaths(root, []string{pattern}, nil)
		require.Error(t, err)
	}
	_, units, err = discoveryPaths(root, nil, nil)
	require.NoError(t, err)
	require.Contains(t, units, filepath.Join(root, "catalog/terragrunt.hcl"))
}

func TestDiscoveryRejectsOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, _, err := discoveryPaths(root, []string{"external/**"}, nil)
	require.ErrorContains(t, err, "outside the Atmos base path")
}

func TestDiscoveryDistinguishesIncludesFromInlineModules(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte("locals { shared = true }"), 0o644))
	request := model.Request{Phase: "inventory", BasePath: root, Environment: map[string]string{}}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, result.Inventory)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("terraform {}"), 0o644))
	_, err = Resolve(t.Context(), request)
	require.ErrorContains(t, err, "inline Terraform modules")
}

func TestDiscoveryReportsBrokenStacksAndMissingDependencies(t *testing.T) {
	for _, name := range []string{"broken stack", "missing dependency", "named workspace outputs"} {
		t.Run(name, func(t *testing.T) {
			req := nativeUnit(t, "")
			switch name {
			case "broken stack":
				require.NoError(t, os.WriteFile(filepath.Join(req.BasePath, "terragrunt.stack.hcl"), []byte("unit {"), 0o644))
			case "missing dependency":
				f, err := os.OpenFile(req.Path, os.O_APPEND|os.O_WRONLY, 0o644)
				require.NoError(t, err)
				_, err = f.WriteString("\ndependencies { paths = [\"./absent\"] }\n")
				require.NoError(t, err)
				require.NoError(t, f.Close())
			case "named workspace outputs":
				dep := filepath.Join(req.BasePath, "network")
				require.NoError(t, os.MkdirAll(dep, 0o755))
				content, err := os.ReadFile(req.Path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dep, "terragrunt.hcl"), content, 0o644))
				require.NoError(t, os.WriteFile(req.Path, append(content, []byte("\ndependency \"network\" { config_path = \"./network\" }\n")...), 0o644))
				req.Environment["TF_WORKSPACE"] = "stage"
			}
			req.Phase = "inventory"
			_, err := Resolve(t.Context(), req)
			require.Error(t, err)
			if name == "named workspace outputs" {
				require.ErrorContains(t, err, "non-default workspaces")
			}
		})
	}
}
