package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// DiscoveryError carries only a filesystem identity and a fixed category across
// the RPC boundary. Cause remains local for debugging and cancellation checks.
type DiscoveryError struct {
	File     string
	Category string
	Cause    error
}

func (e *DiscoveryError) Error() string { return e.Cause.Error() }
func (e *DiscoveryError) Unwrap() error { return e.Cause }

func discoveryError(root, path, category string, cause error) error {
	var existing *DiscoveryError
	if errors.As(cause, &existing) {
		return cause
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return cause
	}
	return &DiscoveryError{File: filepath.ToSlash(relative), Category: category, Cause: cause}
}

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
	stacks, units, err := discoveryPaths(root, request.IncludedPaths, request.ExcludedPaths)
	if err != nil {
		return nil, err
	}
	result := &model.Resolution{}
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
		resolved, err := resolve(ctx, req, overlay)
		if err != nil {
			return nil, discoveryError(root, path, "EVALUATION_FAILED", err)
		}
		return resolved, nil
	}
	var visit func(string, string, string, string, []string, []string, int) error
	visit = func(path, stack, name, generationRoot string, files, folders []string, depth int) error {
		if depth > 32 {
			return fmt.Errorf("nested-stack discovery depth exceeded at %s", path)
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
		if len(resolved.Folders) == 0 {
			entries, err := afero.ReadDir(overlay, filepath.Dir(path))
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".tf") || strings.HasSuffix(entry.Name(), ".tf.json")) {
					return discoveryError(root, path, "MISSING_TERRAFORM_SOURCE", fmt.Errorf("inline Terraform modules require terraform.source"))
				}
			}
			return nil
		} // Shared include, not a runnable local-source unit.
		if !plainLocalSource(resolved.Folders[0]) {
			return discoveryError(root, path, "NONLOCAL_SOURCE", fmt.Errorf("discovery requires local sources"))
		}
		resolved.Files = append(resolved.Files, files...)
		resolved.Folders = append(resolved.Folders, folders...)
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
			return nil, fmt.Errorf("discover %s: %w", path, err)
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
		if err := visit(path, stack, name, "", nil, nil, 0); err != nil {
			return nil, fmt.Errorf("discover %s: %w", path, err)
		}
	}
	slices.SortFunc(result.Inventory, func(a, b model.DiscoveredUnit) int { return strings.Compare(a.Stack+"/"+a.Name, b.Stack+"/"+b.Name) })
	identities := map[string]string{}
	for _, unit := range result.Inventory {
		key := unit.Stack + "\x00" + unit.Name
		if previous, exists := identities[key]; exists {
			return nil, fmt.Errorf("discovered instance collision %s/%s: %s and %s", unit.Stack, unit.Name, previous, unit.Config.Path)
		}
		identities[key] = unit.Config.Path
	}
	return result, nil
}
