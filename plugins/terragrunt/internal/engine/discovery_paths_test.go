package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/afero"
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

// A pattern that selects only ordinary files, such as a docs directory, is still reported as
// matching no Terragrunt configuration.
func TestIncludePatternMatchingOnlyOrdinaryFilesIsReported(t *testing.T) {
	root := scanTree(t, "live/dev/vpc/terragrunt.hcl", "docs/README.md", "docs/notes/main.tf")
	scan, err := discoveryPaths(root, []string{"live/dev", "docs"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"live/dev/vpc/terragrunt.hcl"}, relativeUnits(t, root, scan))
	require.Len(t, scan.Warnings, 1)
	require.Equal(t, model.WarningPatternUnmatched, scan.Warnings[0].Code)
	require.Equal(t, "docs", scan.Warnings[0].Path)
}

// A generated child that fails is reported at its own address; its siblings are still discovered.
func TestDiscoveryKeepsSiblingsOfAFailedGeneratedUnit(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	write := func(name, content string) {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("modules/unit/main.tf", "")
	write("catalog/good/terragrunt.hcl", "terraform { source = \"${get_repo_root()}/modules/unit\" }\n")
	write("catalog/broken/terragrunt.hcl", "terraform { source = \"${get_repo_root()}/modules/unit\" }\nterraform_binary = \"tofu\"\n")
	write(".git/HEAD", "ref: refs/heads/main\n")
	write("live/terragrunt.stack.hcl", `
unit "a_broken" {
  source = "../catalog/broken"
  path   = "a_broken"
}
unit "z_good" {
  source = "../catalog/good"
  path   = "z_good"
}
`)
	result, err := Resolve(t.Context(), model.Request{BasePath: root, Phase: "inventory", Command: "plan", Tofu: "must-not-execute", Environment: map[string]string{}, IncludedPaths: []string{"live"}})
	require.NoError(t, err)
	require.Len(t, result.Inventory, 1)
	require.Equal(t, "z_good", result.Inventory[0].Name)
	require.Len(t, result.Failures, 1)
	require.Equal(t, "live", result.Failures[0].Stack)
	require.Equal(t, "a_broken", result.Failures[0].Name)
	problem := asProblem(t, result.Failures[0].Err)
	require.Equal(t, "terraform_binary", problem.Key)
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
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err, "a broken stack is reported as a failure of its own, not of the whole discovery")
	require.Equal(t, []string{"live/app"}, inventoryNames(t, result))
	require.Len(t, result.Failures, 1, "an included broken stack is still reported")
	require.Equal(t, "live/other", result.Failures[0].Stack)
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

// Inject errors during evaluation, after discovery has begun, without global
// parser hooks or timing-dependent cancellation goroutines.
func TestDiscoveryEvaluationErrors(t *testing.T) {
	for _, stage := range []string{"generation", "stack", "unit", "child stack", "child unit"} {
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("invalid unit")} {
			t.Run(stage+"/"+cause.Error(), func(t *testing.T) {
				root, err := filepath.EvalSymlinks(t.TempDir())
				require.NoError(t, err)
				first := filepath.Join(root, "a", "terragrunt.stack.hcl")
				if stage == "unit" {
					first = filepath.Join(root, "a", "terragrunt.hcl")
				}
				later := filepath.Join(root, "z", "terragrunt.hcl")
				for _, path := range []string{first, later} {
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
					require.NoError(t, os.WriteFile(path, nil, 0o644))
				}
				child := filepath.Join(root, "generated", "terragrunt.hcl")
				if stage == "child stack" {
					child = filepath.Join(root, "generated", "terragrunt.stack.hcl")
				}
				failed, continued := false, false
				evaluate := func(_ context.Context, req model.Request, _ afero.Fs) (*model.Resolution, error) {
					if req.Path == later {
						continued = true
						return &model.Resolution{Path: later, Source: "module"}, nil
					}
					target := req.Path == child || (req.Path == first && (stage == "unit" || (stage == "generation" && req.Command == "generate") || (stage == "stack" && req.Command != "generate")))
					if target {
						failed = true
						return nil, fmt.Errorf("evaluating %s: %w", req.Path, cause)
					}
					result := &model.Resolution{}
					if req.Command != "generate" {
						if stage == "child stack" {
							result.StackConfigs = map[string]string{"child": child}
							result.StackSources = map[string]string{"child": filepath.Join(root, "catalog")}
							result.Stacks = map[string]string{"child": "child"}
						} else if stage == "child unit" {
							result.UnitConfigs = map[string]string{"child": child}
							result.UnitSources = map[string]string{"child": filepath.Join(root, "catalog")}
						}
					}
					return result, nil
				}
				result, err := discoverProjectWithResolver(t.Context(), model.Request{BasePath: root}, evaluate)
				require.True(t, failed, "must reach the intended evaluation failure")
				if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
					require.ErrorIs(t, err, cause)
					require.Nil(t, result, "canceled discovery must not return partial inventory")
					require.False(t, continued, "must stop before evaluating subsequent units")
				} else {
					require.NoError(t, err)
					require.True(t, continued)
					require.Len(t, result.Failures, 1)
					require.Len(t, result.Inventory, 1)
					require.Equal(t, later, result.Inventory[0].Config.Path)
				}
			})
		}
	}
}

func TestDiscoveryContextCanceledDuringEvaluation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "terragrunt.hcl"), nil, 0o644))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := discoverProjectWithResolver(ctx, model.Request{BasePath: root}, func(context.Context, model.Request, afero.Fs) (*model.Resolution, error) {
		cancel()
		return &model.Resolution{}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}
