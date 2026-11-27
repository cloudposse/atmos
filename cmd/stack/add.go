package stack

import (
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/packer"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/stack/adoption"
)

func newStackAddCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add [path]", Short: "Register an existing deployment in an Atmos stack", Args: cobra.MaximumNArgs(1), Example: "  atmos stack add\n  atmos stack add ./infra/network -s dev --name network --command terraform --var-file vars/dev.tfvars"}
	parser := flags.NewStandardParser(flags.WithStringFlag("stack", "s", "dev", "Destination Atmos stack"), flags.WithDryRunFlag(),
		flags.WithStringFlag("workspace", "", "", "Use an existing native Terraform or OpenTofu workspace"),
		flags.WithBoolFlag("share-state", "", false, "Register another instance against the same native state"),
		flags.WithBoolFlag("replace-var-files", "", false, "Replace this instance’s ordered variable file references"),
		flags.WithStringFlag("project", "", "", "Select a project declared in atlantis.yaml"),
		flags.WithStringFlag("name", "", "", "Component name in the destination stack"),
		flags.WithStringArrayFlag("var-file", "", nil, "Terraform or Packer variable file, relative to the selected root (repeat in precedence order)"),
		flags.WithStringFlag("release-name", "", "", "Helm release name"),
		flags.WithStringFlag("namespace", "", "", "Helm release namespace"),
		flags.WithStringArrayFlag("values", "f", nil, "Helm values file, relative to the current directory (repeat in precedence order)"))
	parser.RegisterFlags(cmd)
	flags.RegisterComponentCommandFlags(cmd, "terraform")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		explicit, err := flags.ComponentCommand(cmd, "terraform")
		if err != nil {
			return err
		}
		project, err := os.Getwd()
		if err != nil {
			return err
		}
		target, _ := cmd.Flags().GetString("stack")
		preview, _ := cmd.Flags().GetBool("dry-run")
		configured := len(cfg.LoadedConfigFiles()) != 0
		defaults, err := stackCreationConfig()
		if err != nil {
			return err
		}
		r := adoption.Registration{
			Adapters: stackAdapters(),
			Defaults: defaults, Project: project, Stack: target, Explicit: explicit,
			DryRun: preview, Interactive: !ci.ModeEnabled(cmd) && flags.IsInteractive(), Output: cmd.OutOrStdout(),
		}
		r.AtlantisProject, _ = cmd.Flags().GetString("project")
		r.Workspace, _ = cmd.Flags().GetString("workspace")
		r.ShareState, _ = cmd.Flags().GetBool("share-state")
		r.ReplaceVarFiles, _ = cmd.Flags().GetBool("replace-var-files")
		r.Name, _ = cmd.Flags().GetString("name")
		r.ReleaseName, _ = cmd.Flags().GetString("release-name")
		r.Namespace, _ = cmd.Flags().GetString("namespace")
		r.VarFiles, _ = cmd.Flags().GetStringArray("var-file")
		r.Values, _ = cmd.Flags().GetStringArray("values")
		if configured {
			r.Config = defaults
			r.Project = r.Config.BasePathAbsolute
		}
		return guideStackAdd(cmd, &r, project, args)
	}
	internal.RegisterCommandMetadata(cmd, internal.WithDocumentationURL("https://atmos.tools/cli/commands/stack/usage#register-an-existing-deployment"))
	return cmd
}

func init() { stackCmd.AddCommand(newStackAddCommand()) }

// Packer uses a legacy executor and has no registered ComponentProvider.
func stackAdapters() adoption.Adapters {
	var adapters adoption.Adapters
	legacyPacker := true
	for _, providers := range component.ListProviders() {
		for _, provider := range providers {
			if provider.GetType() == "packer" {
				legacyPacker = false
			}
			if source, ok := provider.(adoption.Provider); ok {
				adapters = append(adapters, source.AdoptionAdapters()...)
			}
		}
	}
	if legacyPacker {
		adapters = append(adapters, packer.Adapter{})
	}
	slices.SortStableFunc(adapters, func(a, b adoption.Adapter) int { return strings.Compare(a.Kind(), b.Kind()) })
	return adapters
}
