package engine

import (
	"encoding/json"
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

// Terragrunt builds stack template sources by concatenating a native directory with a slash-separated
// suffix, for example "${values.project_root}/catalog/units/app". On Windows that yields mixed separators,
// and the discovery helpers must treat those paths as equal to their native spelling.
func TestPathHelpersAcceptMixedSeparators(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	nativeUnit := filepath.Join(root, "catalog", "units", "app")
	for name, spelling := range map[string]string{
		"native":         nativeUnit,
		"slash suffix":   root + "/catalog/units/app",
		"slash relative": root + string(filepath.Separator) + filepath.FromSlash("catalog/units/app"),
		"redundant":      root + "/catalog/./units/app",
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, plainLocalSource(spelling))
			require.True(t, within(spelling, root))
			require.False(t, within(root, spelling))
			require.Equal(t, nativeUnit, localPath("ignored", spelling))
		})
	}
	// A relative source resolves against the stack directory in native form.
	require.Equal(t, nativeUnit, localPath(root, "catalog/units/app"))
	require.Equal(t, nativeUnit, localPath(filepath.Join(root, "live"), "../catalog/units/app"))
}

// stackProject writes a module, a catalog unit pointing at it, and the given files.
func stackProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	module := filepath.Join(root, "modules", "unit")
	source, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	files["modules/unit/main.tf"] = "terraform {}\n"
	files["catalog/units/app/terragrunt.hcl"] = "terraform { source = " + string(source) + " }\n"
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func inventoryNames(t *testing.T, result *model.Resolution) []string {
	t.Helper()
	names := make([]string, 0, len(result.Inventory))
	for _, unit := range result.Inventory {
		names = append(names, unit.Stack+"/"+unit.Name)
	}
	return names
}

func TestDiscoveryPreservesExpandedInstanceIdentities(t *testing.T) {
	root := stackProject(t, map[string]string{"live/terragrunt.stack.hcl": `
unit "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }
  source = "../catalog/units/app"
  path   = "aurora/${each.key}"
}

unit "shard" {
  expansion {
    count = 2
  }
  source = "../catalog/units/app"
  path   = "shard/${count.index}"
}

unit "vpc" {
  source = "../catalog/units/app"
  path   = "vpc"
}
`})
	result, err := Resolve(t.Context(), model.Request{Phase: "inventory", BasePath: root, IncludedPaths: []string{"live/**"}, Environment: map[string]string{}})
	require.NoError(t, err)
	require.Equal(t, []string{"live/aurora/api", "live/aurora/web", "live/shard/0", "live/shard/1", "live/vpc"}, inventoryNames(t, result))
	paths := map[string]bool{}
	for _, unit := range result.Inventory {
		paths[unit.Config.Path] = true
	}
	require.Len(t, paths, 5, "each expansion element keeps its own generated unit")
}

func TestInstanceIdentityRejectsUnrepresentableKeys(t *testing.T) {
	id, err := instanceIdentity("unit", "app", "", false)
	require.NoError(t, err)
	require.Equal(t, "app", id, "unexpanded blocks keep their label")
	id, err = instanceIdentity("unit", "app", "web", true)
	require.NoError(t, err)
	require.Equal(t, "app/web", id)
	for _, key := range []string{"", ".", "..", "a/b", `a\b`} {
		_, err := instanceIdentity("stack", "app", key, true)
		require.ErrorContains(t, err, "unsupported stack expansion key", key)
	}
}

// Generation must not materialize stack files that the selected stack does not
// declare, including excluded stack files nested under its directory.
func TestStackGenerationIsScopedToDeclaredStacks(t *testing.T) {
	root := stackProject(t, map[string]string{
		"live/terragrunt.stack.hcl": `
unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`,
		// Undeclared and excluded: generating it would fail on the missing value.
		"live/other/terragrunt.stack.hcl": `
unit "app" {
  source = values.missing
  path   = "app"
}
`,
	})
	request := model.Request{Phase: "inventory", BasePath: root, IncludedPaths: []string{"live/**"}, ExcludedPaths: []string{"live/other/**"}, Environment: map[string]string{}}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"live/app"}, inventoryNames(t, result))
	require.NoDirExists(t, filepath.Join(root, "live", ".terragrunt-stack"), "discovery generates only into its overlay")

	request.ExcludedPaths = nil
	_, err = Resolve(t.Context(), request)
	require.Error(t, err, "an included broken stack is still reported")
}

// A unit that a stack generates beside its stack file belongs to that stack; a
// copy already on disk must not be discovered again as a standalone unit.
func TestDiscoverySkipsStackOwnedGeneratedUnits(t *testing.T) {
	root := stackProject(t, map[string]string{
		"live/terragrunt.stack.hcl": `
unit "app" {
  source = "../catalog/units/app"
  path   = "app"
  no_dot_terragrunt_stack = true
}
`,
		"live/standalone/terragrunt.hcl": "",
	})
	module, err := json.Marshal(filepath.ToSlash(filepath.Join(root, "modules", "unit")))
	require.NoError(t, err)
	unit := "terraform { source = " + string(module) + " }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "live", "standalone", "terragrunt.hcl"), []byte(unit), 0o644))
	// Materialized by an earlier `terragrunt stack generate`.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "live", "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "live", "app", "terragrunt.hcl"), []byte(unit), 0o644))

	result, err := Resolve(t.Context(), model.Request{Phase: "inventory", BasePath: root, IncludedPaths: []string{"live/**"}, Environment: map[string]string{}})
	require.NoError(t, err)
	require.Equal(t, []string{"live/app", "live/standalone"}, inventoryNames(t, result))
	require.Equal(t, filepath.Join(root, "live", "terragrunt.stack.hcl"), result.Inventory[0].GenerationRoot, "the stack owns the generated unit")
	require.Empty(t, result.Inventory[1].GenerationRoot)
}
