package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// canonicalPath preserves physical ancestor identity for files generated only in
// the discovery overlay. Actual existence is checked through the active FS.
func canonicalPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = canonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

// logicalPath resolves symlinks in a configuration file's directory but not in the file
// itself. Like upstream Terragrunt, a symlinked terragrunt.hcl is a unit at the place the
// link lives, and the configuration is read through the link.
func logicalPath(path string) (string, error) {
	dir, err := canonicalPath(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(path)), nil
}

func within(path, directory string) bool {
	rel, err := filepath.Rel(directory, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// discoverProject uses an invocation-local copy-on-write overlay. Generated HCL
// retains its original virtual path; no source files or state are moved/written.
func discoverProject(ctx context.Context, request model.Request) (*model.Resolution, error) {
	root, err := canonicalPath(request.BasePath)
	if err != nil {
		return nil, err
	}
	overlay := afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs())
	scan, err := discoveryPaths(root, request.IncludedPaths, request.ExcludedPaths)
	if err != nil {
		return nil, err
	}
	stacks, units := scan.Stacks, scan.Units
	result := &model.Resolution{Warnings: scan.Warnings}
	templates := map[string]bool{}
	skipped := func(path string) bool {
		for dir := range templates {
			if within(path, dir) {
				return true
			}
		}
		return false
	}
	nameFor := func(dir string) string {
		rel, _ := filepath.Rel(root, dir)
		if rel == "." {
			return "default"
		}
		return filepath.ToSlash(rel)
	}
	parse := func(path string, generate bool) (*model.Resolution, error) {
		req := request
		req.Path, req.Phase = path, "discover"
		if generate {
			req.Command = "generate"
		}
		return resolve(ctx, req, overlay)
	}
	var visit func(string, string, string, string, []string, []string, int) error
	visit = func(path, stack, name, generationRoot string, files, folders []string, depth int) error {
		if depth > 32 {
			return &model.Problem{Code: model.CodeEvaluationError, Summary: "Terragrunt stacks are nested too deeply", Detail: "Nested `terragrunt.stack.hcl` generation exceeded 32 levels, which usually means a stack includes itself.", Hints: []string{"Remove the circular `stack` reference."}, File: path, Safe: true}
		}
		resolved, err := parse(path, false)
		if err != nil {
			return err
		}
		if filepath.Base(path) == "terragrunt.stack.hcl" {
			files = append(append([]string{}, files...), resolved.Files...)
			for child, childPath := range resolved.StackConfigs {
				source := resolved.StackSources[child]
				templates[source] = true
				if err := visit(childPath, stack, filepath.Join(name, resolved.Stacks[child]), generationRoot, files, append(append([]string{}, folders...), source), depth+1); err != nil {
					return err
				}
			}
			for child, childPath := range resolved.UnitConfigs {
				source := resolved.UnitSources[child]
				templates[source] = true
				if err := visit(childPath, stack, filepath.Join(name, child), generationRoot, files, append(append([]string{}, folders...), source), depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if resolved.Source == "" && len(resolved.Folders) == 0 {
			entries, err := afero.ReadDir(overlay, filepath.Dir(path))
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".tf") || strings.HasSuffix(entry.Name(), ".tf.json")) {
					return missingSourceProblem(path)
				}
			}
			// Shared include, not a runnable local-source unit. Say so: it is not listed.
			rel, _ := filepath.Rel(root, path)
			result.Warnings = append(result.Warnings, model.Warning{
				Code: model.WarningUnitWithoutSource, Path: filepath.ToSlash(rel),
				Message: fmt.Sprintf("%s has no terraform { source } block and no Terraform files next to it, so it is treated as a shared configuration and is not listed as a component", filepath.ToSlash(rel)),
			})
			return nil
		}
		resolved.Files = append(resolved.Files, files...)
		resolved.Folders = append(resolved.Folders, folders...)
		if err := trackInputSources(overlay, root, resolved); err != nil {
			return err
		}
		result.Inventory = append(result.Inventory, model.DiscoveredUnit{Stack: stack, Name: filepath.ToSlash(name), GenerationRoot: generationRoot, Config: *resolved})
		return nil
	}
	// Catalog stack templates may require values supplied by a live stack. Delay
	// their errors until successful roots have identified the referenced templates.
	failures := map[string]error{}
	for _, path := range stacks {
		if skipped(path) {
			continue
		}
		if _, err := parse(path, true); err != nil {
			failures[path] = err
			continue
		}
		if err := visit(path, nameFor(filepath.Dir(path)), "", path, nil, nil, 0); err != nil {
			failures[path] = err
		}
	}
	for _, path := range stacks {
		if err := failures[path]; err != nil && !skipped(path) {
			// The components of a stack that cannot be generated are unknown, so the stack file itself is the unit.
			result.Failures = append(result.Failures, model.UnitFailure{Stack: nameFor(filepath.Dir(path)), Name: filepath.Base(path), Err: problemFor(err, path)})
		}
	}
	// A template that parsed independently is still a template, not another live stack.
	result.Inventory = slices.DeleteFunc(result.Inventory, func(unit model.DiscoveredUnit) bool { return skipped(unit.GenerationRoot) })
	for _, path := range units {
		if skipped(path) {
			continue
		}
		dir := filepath.Dir(path)
		stack, name := nameFor(filepath.Dir(dir)), filepath.Base(dir)
		if dir == root {
			stack, name = "default", "root"
		}
		// One failing unit is recorded and reported on its own; the other units stay usable.
		if err := visit(path, stack, name, "", nil, nil, 0); err != nil {
			result.Failures = append(result.Failures, model.UnitFailure{Stack: stack, Name: filepath.ToSlash(name), Err: problemFor(err, path)})
		}
	}
	slices.SortFunc(result.Inventory, func(a, b model.DiscoveredUnit) int { return strings.Compare(a.Stack+"/"+a.Name, b.Stack+"/"+b.Name) })
	identities := map[string]string{}
	for _, unit := range result.Inventory {
		key := unit.Stack + "\x00" + unit.Name
		if previous, exists := identities[key]; exists {
			return nil, &model.Problem{
				Code: model.CodeEvaluationError, Summary: "Two Terragrunt units resolve to the same component", Detail: fmt.Sprintf("%s/%s is produced by both %s and %s.", unit.Stack, unit.Name, filepath.ToSlash(mustRel(root, previous)), filepath.ToSlash(mustRel(root, unit.Config.Path))), Safe: true,
				Hints: []string{"Remove one of the units, a symlink that repeats a unit, or an overlapping `included_paths` entry."},
			}
		}
		identities[key] = unit.Config.Path
	}
	return result, nil
}
