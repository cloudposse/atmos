package engine

import (
	"os"
	"path/filepath"
	"slices"
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
	scan, err := discoveryPaths(root, []string{"live/**", "live/unit/**"}, []string{"live/excluded/**"})
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(root, "live/terragrunt.stack.hcl")}, scan.Stacks)
	require.Equal(t, []string{filepath.Join(root, "live/unit/terragrunt.hcl")}, scan.Units)
	scan, err = discoveryPaths(root, []string{"absent/**"}, nil)
	require.NoError(t, err)
	require.Len(t, scan.Warnings, 1)
	require.Equal(t, model.WarningPatternUnmatched, scan.Warnings[0].Code)
	for _, pattern := range []string{"", "[", "../**", `live\**`, filepath.Join(root, "**")} {
		_, err := discoveryPaths(root, []string{pattern}, nil)
		require.Error(t, err)
	}
	_, err = discoveryPaths(root, nil, []string{"../x"})
	require.ErrorContains(t, err, "excluded_paths")
	scan, err = discoveryPaths(root, nil, nil)
	require.NoError(t, err)
	require.Contains(t, scan.Units, filepath.Join(root, "catalog/terragrunt.hcl"))
}

func TestDiscoveryRejectsOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := discoveryPaths(root, []string{"external/**"}, nil)
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
	require.Len(t, result.Warnings, 1, "a shared configuration without terraform.source is reported, not silently dropped")
	require.Equal(t, model.WarningUnitWithoutSource, result.Warnings[0].Code)
	require.Equal(t, "terragrunt.hcl", result.Warnings[0].Path)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("terraform {}"), 0o644))
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, result.Failures, 1)
	require.ErrorContains(t, result.Failures[0].Err, "terraform.source")
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
			// A failing unit or stack is reported on its own instead of failing the project.
			result, err := Resolve(t.Context(), req)
			require.NoError(t, err)
			require.Len(t, result.Failures, 1)
			var problem *model.Problem
			require.ErrorAs(t, result.Failures[0].Err, &problem)
			require.NotEmpty(t, problem.Hints)
			if name == "named workspace outputs" {
				require.Equal(t, model.CodeWorkspace, problem.Code)
			}
		})
	}
}

// scanTree writes empty unit files and returns the project root.
func scanTree(t *testing.T, files ...string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))
	}
	return root
}

func relativeUnits(t *testing.T, root string, scan pathScan) []string {
	t.Helper()
	var result []string
	for _, path := range scan.Units {
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		result = append(result, filepath.ToSlash(rel))
	}
	return result
}

// included_paths and excluded_paths share one pattern language: a pattern selects what it
// matches and everything below it, with or without a trailing slash, `./`, or `/**`.
func TestIncludedAndExcludedPathsShareOnePatternLanguage(t *testing.T) {
	root := scanTree(t, "live/dev/vpc/terragrunt.hcl", "live/dev/app/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl")
	all := []string{"live/dev/app/terragrunt.hcl", "live/dev/vpc/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl"}
	for _, test := range []struct {
		pattern string
		want    []string
	}{
		{"live", all},
		{"live/", all},
		{"./live", all},
		{"./**", all},
		{".", all},
		{"live/**", all},
		{"live/dev", all[:2]},
		{"live/dev/", all[:2]},
		{"live/dev/*", all[:2]},
		{"live/dev/vpc", []string{"live/dev/vpc/terragrunt.hcl"}},
		{"live/dev/vpc/terragrunt.hcl", []string{"live/dev/vpc/terragrunt.hcl"}},
		{"**/vpc", []string{"live/dev/vpc/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl"}},
		{"live/*/vpc", []string{"live/dev/vpc/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl"}},
	} {
		t.Run("include "+test.pattern, func(t *testing.T) {
			scan, err := discoveryPaths(root, []string{test.pattern}, nil)
			require.NoError(t, err)
			require.Equal(t, test.want, relativeUnits(t, root, scan))
			require.Empty(t, scan.Warnings)
		})
		t.Run("exclude "+test.pattern, func(t *testing.T) {
			scan, err := discoveryPaths(root, nil, []string{test.pattern})
			require.NoError(t, err)
			var kept []string
			for _, unit := range all {
				if !slices.Contains(test.want, unit) {
					kept = append(kept, unit)
				}
			}
			require.Equal(t, kept, relativeUnits(t, root, scan))
		})
	}
}

func TestUnmatchedIncludePatternIsReported(t *testing.T) {
	root := scanTree(t, "live/dev/vpc/terragrunt.hcl")
	scan, err := discoveryPaths(root, []string{"live/dev", "lives", "live/none/*"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"live/dev/vpc/terragrunt.hcl"}, relativeUnits(t, root, scan))
	var patterns []string
	for _, warning := range scan.Warnings {
		require.Equal(t, model.WarningPatternUnmatched, warning.Code)
		patterns = append(patterns, warning.Path)
	}
	require.Contains(t, patterns, "live/none/*")
	require.Len(t, patterns, 2, "the matching pattern is not reported")
}

// Upstream Terragrunt does not follow directory symlinks (without its symlinks experiment) and
// treats a symlinked terragrunt.hcl as a unit where the link lives. A skipped link is reported.
func TestSymlinksFollowUpstreamTerragruntAndAreReported(t *testing.T) {
	root := scanTree(t, "live/prod/vpc/terragrunt.hcl", "live/dev/vpc/terragrunt.hcl")
	if err := os.Symlink("prod", filepath.Join(root, "live", "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "live", "dev", "copy"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "vpc", "terragrunt.hcl"), filepath.Join(root, "live", "dev", "copy", "terragrunt.hcl")))
	scan, err := discoveryPaths(root, []string{"live/**", "live"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"live/dev/copy/terragrunt.hcl", "live/dev/vpc/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl"}, relativeUnits(t, root, scan), "the symlinked file is a unit at its own location; the linked directory adds none")
	require.Len(t, scan.Warnings, 1, "overlapping patterns report a skipped link once")
	require.Equal(t, model.WarningSymlinkedDirectory, scan.Warnings[0].Code)
	require.Equal(t, "live/linked", scan.Warnings[0].Path)
	require.Contains(t, scan.Warnings[0].Message, "live/linked")
	again, err := discoveryPaths(root, []string{"live/**"}, nil)
	require.NoError(t, err)
	require.Equal(t, scan, again, "discovery is deterministic")
}

func TestLogicalPathKeepsASymlinkedConfigurationFile(t *testing.T) {
	root := scanTree(t, "a/terragrunt.hcl")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "b"), 0o755))
	if err := os.Symlink(filepath.Join("..", "a", "terragrunt.hcl"), filepath.Join(root, "b", "terragrunt.hcl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	got, err := logicalPath(filepath.Join(root, "b", "terragrunt.hcl"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "b", "terragrunt.hcl"), got)
	canonical, err := canonicalPath(filepath.Join(root, "b", "terragrunt.hcl"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "a", "terragrunt.hcl"), canonical)
}
