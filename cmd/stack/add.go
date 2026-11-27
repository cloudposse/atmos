package stack

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/internal/terraformadoption"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newStackAddCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add <directory>", Short: "Register an existing Terraform or OpenTofu root in a stack", Args: cobra.ExactArgs(1)}
	parser := flags.NewStandardParser(flags.WithStringFlag("stack", "s", "dev", "Destination Atmos stack"), flags.WithDryRunFlag())
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
		dir, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		target, _ := cmd.Flags().GetString("stack")
		preview, _ := cmd.Flags().GetBool("dry-run")
		r := terraformadoption.Registration{
			Project: project, Dir: dir, Stack: target, Explicit: explicit,
			DryRun: preview, Interactive: !ci.ModeEnabled(cmd) && flags.IsInteractive(), Output: cmd.OutOrStdout(),
		}
		if len(cfg.LoadedConfigFiles()) != 0 {
			r.Config, err = stackCreationConfig()
			if err != nil {
				return err
			}
			r.Project = r.Config.BasePathAbsolute
		}
		return r.Save(cmd.Context())
	}
	internal.RegisterCommandMetadata(cmd, internal.WithDocumentationURL("https://atmos.tools/cli/commands/stack/usage#adopt-a-terraform-or-opentofu-root"))
	return cmd
}

func init() { stackCmd.AddCommand(newStackAddCommand()) }
