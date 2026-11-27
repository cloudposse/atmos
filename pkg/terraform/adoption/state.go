package adoption

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/flags"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
)

func (r *request) registrationWorkspace(ctx context.Context, engine Engine, entry deployment, relative string) (string, error) {
	openTofu, err := usesTofu(ctx, r.Dir, engine)
	if err != nil {
		return "", err
	}
	layout, err := backend.ResolveLocalState(r.Dir, openTofu)
	if err != nil {
		return "", err
	}
	workspace, err := verifiedWorkspace(r.Dir, layout, cmp.Or(r.Workspace, entry.Metadata.Workspace))
	if err != nil || r.ShareState || entry.Metadata.Component == relative {
		return workspace, err
	}
	return r.checkStateTarget(layout, workspace)
}

func (r *request) checkStateTarget(layout backend.LocalState, workspace string) (string, error) {
	owners, err := r.registeredStateTargets()
	if err != nil {
		return "", err
	}
	for {
		state, err := filesystem.CanonicalPath(layout.Path(workspace))
		if err != nil {
			return "", err
		}
		owner, exists := owners[state]
		if !exists {
			return workspace, nil
		}
		workspace, err = r.chooseStateTarget(layout, workspace, owner)
		if err != nil || r.ShareState {
			return workspace, err
		}
	}
}

var promptStateTarget = flags.PromptForValue

func (r *request) chooseStateTarget(layout backend.LocalState, current, owner string) (string, error) {
	const share = "Share this state with the existing instance"
	message := fmt.Sprintf("Workspace %q uses the same local state as %s", current, owner)
	if !r.Interactive {
		return "", fmt.Errorf("%w: %s; choose another existing --workspace or pass --share-state to share it intentionally", errUtils.ErrInvalidStackConfig, message)
	}
	names := []string{"default"}
	entries, err := os.ReadDir(layout.Workspaces)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "default" {
			names = append(names, e.Name())
		}
	}
	names = slices.DeleteFunc(names, func(name string) bool { return name == current || r.atlantisWorkspace != "" })
	selected, err := promptStateTarget("workspace", message+". Choose a target or press Esc to cancel.", append(names, share))
	if err == nil && selected == share {
		r.ShareState = true
		return current, nil
	}
	return selected, err
}

func (r *request) registeredStateTargets() (map[string]string, error) {
	base, stacks, err := r.layout()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(stacks)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	owners := make(map[string]string)
	for _, f := range entries {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		if err := indexManifestStates(filepath.Join(stacks, f.Name()), base, owners); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name(), err)
		}
	}
	return owners, nil
}

func indexManifestStates(file, base string, owners map[string]string) error {
	data, _, err := stackadoption.ReadRegular(file)
	if err != nil {
		return err
	}
	var manifest adoptionManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return err
	}
	// Only standalone, fully specified registrations can be resolved from this file alone.
	if len(manifest.Import) > 0 || len(manifest.Shared) > 0 {
		return nil
	}
	stack := cmp.Or(manifest.Name, strings.TrimSuffix(filepath.Base(file), ".yaml"))
	for _, name := range slices.Sorted(maps.Keys(manifest.Components.Terraform)) {
		entry := manifest.Components.Terraform[name]
		if entry.Metadata.Component == "" || entry.Metadata.Workspace == "" {
			continue
		}
		label := fmt.Sprintf("component %s in Atmos stack %s (workspace %s)", name, stack, entry.Metadata.Workspace)
		if err := indexDeployment(base, entry, label, owners); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	return nil
}

// A wrapper can launch either engine. Consider both native file layouts without
// executing another component's wrapper during discovery.
func indexDeployment(base string, entry deployment, owner string, owners map[string]string) error {
	dir := filepath.Join(base, filepath.FromSlash(entry.Metadata.Component))
	engines := []bool{false, true}
	switch strings.TrimSuffix(filepath.Base(entry.Command), ".exe") {
	case terraform:
		engines = []bool{false}
	case tofu:
		engines = []bool{true}
	}
	for _, openTofu := range engines {
		layout, err := backend.ResolveLocalState(dir, openTofu)
		if errors.Is(err, backend.ErrNonLocalBackend) {
			continue
		}
		if err != nil {
			return err
		}
		key, err := filesystem.CanonicalPath(layout.Path(entry.Metadata.Workspace))
		if err != nil {
			return err
		}
		owners[key] = owner
	}
	return nil
}
