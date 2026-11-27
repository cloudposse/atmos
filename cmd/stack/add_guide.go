package stack

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/stack/adoption"
)

func askAdoption(ctx context.Context, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithTheme(uiutils.NewAtmosHuhTheme()).WithKeyMap(uiutils.NewAtmosKeyMap())
	err := runCreationForm(ctx, form)
	if errors.Is(err, huh.ErrUserAborted) {
		return errUtils.ErrUserAborted
	}
	return err
}

func adoptionText(ctx context.Context, title, suggestion string, valid func(string) bool) (string, error) {
	input := ""
	label := title
	if suggestion != "" {
		label = fmt.Sprintf("%s (Enter for %s)", title, suggestion)
	}
	field := huh.NewInput().Title(label).Placeholder(suggestion).Value(&input).Validate(func(value string) error {
		if !valid(cmp.Or(strings.TrimSpace(value), suggestion)) {
			return fmt.Errorf("%w: enter a valid %s", errUtils.ErrInvalidFlagValue, title)
		}
		return nil
	})
	err := askAdoption(ctx, field)
	return cmp.Or(strings.TrimSpace(input), suggestion), err
}

const manualPath = "Enter a different path"

func adoptionPath(ctx context.Context, r *adoption.Registration, cwd string, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !r.Interactive {
		return "", fmt.Errorf("%w: a source path is required; run atmos stack add ./path/to/deployment -s dev", errUtils.ErrMissingInput)
	}
	roots, err := r.Adapters.Discover(ctx, cwd)
	if err != nil {
		return "", err
	}
	if len(roots) > 0 {
		var options []huh.Option[string]
		for _, root := range roots {
			rel, err := filepath.Rel(cwd, root.Path)
			if err != nil {
				return "", err
			}
			options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", rel, root.Kind), rel))
		}
		selected := options[0].Value
		if err := askAdoption(ctx, huh.NewSelect[string]().Title("Which deployment would you like to register?").Options(append(options, huh.NewOption(manualPath, ""))...).Value(&selected)); err != nil {
			return "", err
		}
		if selected != "" {
			return selected, nil
		}
	}
	return adoptionText(ctx, "Deployment path (for example ./infra/network)", "", func(s string) bool { return s != "" })
}

func guideStackAdd(cmd *cobra.Command, r *adoption.Registration, cwd string, args []string) error {
	ctx := cmd.Context()
	a, selected, err := guidedSource(ctx, r, cwd, args)
	if err != nil {
		return err
	}
	if !r.Interactive || inventoryRequest(r, a) {
		return r.Save(ctx)
	}
	address, err := askDestination(cmd, r, a, selected)
	if err != nil {
		return err
	}
	native, err := askNativeInputs(cmd, r, a, selected)
	if err != nil {
		return err
	}
	if r.DryRun {
		return r.Save(ctx)
	}
	return reviewRegistration(ctx, r, len(args) == 0 || address || native)
}

func inventoryRequest(r *adoption.Registration, adapter adoption.Adapter) bool {
	return adapter.Kind() == "atlantis" && r.DryRun && r.AtlantisProject == ""
}

func guidedSource(ctx context.Context, r *adoption.Registration, cwd string, args []string) (adoption.Adapter, string, error) {
	path, err := adoptionPath(ctx, r, cwd, args)
	if err != nil {
		return nil, "", err
	}
	r.SourcePath = path
	r.Dir, err = filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	a, selected, err := r.Adapters.Detect(r.Dir)
	if err != nil {
		return nil, "", err
	}
	if err := r.CheckOptions(a); err != nil {
		return nil, "", err
	}
	stat, err := os.Stat(selected)
	if err != nil {
		return nil, "", err
	}
	if !stat.IsDir() {
		selected = filepath.Dir(selected)
	}
	return a, selected, nil
}

func askDestination(cmd *cobra.Command, r *adoption.Registration, a adoption.Adapter, dir string) (bool, error) {
	askStack := !cmd.Flags().Changed("stack")
	if askStack {
		value, err := adoptionText(cmd.Context(), "Atmos stack", r.Stack, adoption.ValidName)
		if err != nil {
			return false, err
		}
		r.Stack = value
	}
	if !slices.Contains(a.Options(), "name") || r.Name != "" || r.ReleaseName != "" {
		return askStack, nil
	}
	name := filepath.Base(dir)
	if a.Kind() == "atlantis" {
		name = r.AtlantisProject
	}
	if name == "" {
		return askStack, nil
	} // The Atlantis project picker supplies the name later.
	value, err := adoptionText(cmd.Context(), "Component name", name, adoption.ValidName)
	r.Name = value
	return true, err
}

func askNativeInputs(cmd *cobra.Command, r *adoption.Registration, a adoption.Adapter, dir string) (bool, error) {
	if a.Kind() == "helm" {
		return askHelmTarget(cmd.Context(), r, dir)
	}
	if slices.Contains(a.Options(), "var-file") && !cmd.Flags().Changed("var-file") {
		return chooseAdoptionInputs(cmd.Context(), r, dir, a.Kind())
	}
	return false, nil
}

func askHelmTarget(ctx context.Context, r *adoption.Registration, dir string) (bool, error) {
	askRelease := r.ReleaseName == ""
	if askRelease {
		value, err := adoptionText(ctx, "Helm release", cmp.Or(r.Name, filepath.Base(dir)), adoption.ValidName)
		if err != nil {
			return false, err
		}
		r.ReleaseName = value
	}
	if r.Namespace != "" {
		return askRelease, nil
	}
	value, err := adoptionText(ctx, "Kubernetes namespace", "default", func(s string) bool { return s != "" })
	r.Namespace = value
	return true, err
}

func reviewRegistration(ctx context.Context, r *adoption.Registration, guided bool) error {
	// Run all native pickers and present the proposed inputs before acquiring the write lock.
	// The second run repeats validation against current files with concrete, noninteractive choices.
	preview := *r
	preview.DryRun = true
	var resolved *adoption.Registration
	preview.OnPreview = func(result adoption.Registration) { resolved = &result }
	if err := preview.Save(ctx); err != nil {
		return err
	}
	captured := resolved != nil
	// Adapters implementing the original contract need not report resolved inputs.
	if !captured {
		original := *r
		resolved = &original
	}
	if guided {
		yes := true
		if err := askAdoption(ctx, uiutils.NewAtmosConfirm().Title("Save this registration?").Value(&yes)); err != nil {
			return err
		}
		if !yes {
			return errUtils.ErrUserAborted
		}
	}
	resolved.Dir = r.Dir
	if captured {
		resolved.Interactive = false
	}
	resolved.DryRun = false
	resolved.OnPreview = nil
	return resolved.Save(ctx)
}

func chooseAdoptionInputs(ctx context.Context, r *adoption.Registration, dir, kind string) (bool, error) {
	inputs, err := adoption.DiscoverVarFiles(dir, kind)
	if err != nil {
		return false, err
	}
	if len(inputs.Automatic) > 0 {
		fmt.Fprintln(r.Output, "Loaded automatically:", strings.Join(inputs.Automatic, ", "))
	}
	saved, err := r.SavedVarFiles(kind, cmp.Or(r.Name, filepath.Base(dir)), dir)
	if err != nil {
		return false, err
	}
	if len(saved) > 0 && !r.ReplaceVarFiles {
		keep := true
		if err := askAdoption(ctx, huh.NewSelect[bool]().Title("Variable files for the saved instance").Options(
			huh.NewOption("Keep saved order: "+strings.Join(saved, ", "), true), huh.NewOption("Replace the saved input references", false),
		).Value(&keep)); err != nil {
			return false, err
		}
		if keep {
			r.VarFiles = saved
			return true, nil
		}
		r.ReplaceVarFiles = true
	}
	if len(inputs.Optional) == 0 {
		return false, nil
	}
	return chooseOrderedFiles(ctx, r, inputs.Optional)
}

func chooseOrderedFiles(ctx context.Context, r *adoption.Registration, remaining []string) (bool, error) {
	for len(remaining) > 0 {
		done := "Use no additional files"
		if len(r.VarFiles) > 0 {
			done = "Use this order: " + strings.Join(r.VarFiles, ", ")
		}
		options := []huh.Option[string]{huh.NewOption(done, "")}
		for _, file := range remaining {
			options = append(options, huh.NewOption(file, file))
		}
		pick := ""
		if err := askAdoption(ctx, huh.NewSelect[string]().Title("Additional variable files in precedence order").Options(options...).Value(&pick)); err != nil {
			return false, err
		}
		if pick == "" {
			break
		}
		r.VarFiles = append(r.VarFiles, pick)
		remaining = slices.DeleteFunc(remaining, func(s string) bool { return s == pick })
	}
	fmt.Fprintln(r.Output, "Additional variable files:", cmp.Or(strings.Join(r.VarFiles, ", "), "none"))
	return true, nil
}
