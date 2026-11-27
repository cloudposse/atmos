package terraform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/terraformadoption"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/process"
)

// Only a bare project's explicit path enters adoption. Configured components,
// including paths that select among existing instances, keep their normal lifecycle.
func isAdoptionPlan(args []string) bool {
	return len(args) > 0 && component.IsExplicitComponentPath(args[0]) && len(cfg.LoadedConfigFiles()) == 0
}

func runAdoptionPlan(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: expected one root directory; pass native plan arguments after --", errUtils.ErrInvalidFlagValue)
	}
	if err := validateAdoptionPlan(cmd); err != nil {
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
	explicit, err := flags.ComponentCommand(cmd, "terraform")
	if err != nil {
		return err
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	nativeArgs := append([]string{}, compat.GetSeparated()...)
	if plain, _ := cmd.Flags().GetBool("no-color"); plain {
		nativeArgs = append(nativeArgs, "-no-color")
	}
	task, err := adoptionPlanTask(cmd, dir, project, explicit, nativeArgs)
	if err != nil {
		return err
	}
	task.DryRun = dry
	task.Streams = process.Streams{Stdin: cmd.InOrStdin(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
	return process.NewDefaultRunner().Run(cmd.Context(), task).Err
}

func adoptionPlanTask(cmd *cobra.Command, dir, project, explicit string, args []string) (process.TaskSpec, error) {
	unit, err := terraformadoption.TerragruntUnit(dir)
	if err != nil {
		return process.TaskSpec{}, err
	}
	if unit != "" {
		native, err := terraformadoption.TerragruntPlan(cmd.Context(), unit, project, explicit, args)
		if err != nil {
			return process.TaskSpec{}, err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Terragrunt unit: %s\n", unit)
		return process.TaskSpec{Command: native.Binary, Dir: native.WorkingDir, Args: native.Args, Env: native.Env}, nil
	}
	engine, err := terraformadoption.Select(cmd.Context(), dir, project, terraformadoption.Selection{Explicit: explicit, Interactive: !ci.ModeEnabled(cmd) && flags.IsInteractive()})
	if err != nil {
		return process.TaskSpec{}, err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Engine: %s (%s)\n", engine.Command, engine.Reason)
	return process.TaskSpec{Command: engine.Command, Path: engine.Path, Dir: dir, Args: append([]string{"plan"}, args...)}, nil
}

func validateAdoptionPlan(cmd *cobra.Command) error {
	var invalid string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "command", "terraform-command", "interactive", "chdir", "dry-run", "ci", "no-color", "logs-level":
		default:
			invalid = f.Name
		}
	})
	if invalid != "" {
		return fmt.Errorf("%w: --%s requires a registered component", errUtils.ErrInvalidFlagValue, invalid)
	}
	bundle, err := admission.LoadBundle()
	if err != nil {
		return err
	}
	if bundle != nil {
		for _, kind := range []policy.InputKind{policy.ComponentCommand, policy.Operation, policy.TerraformPlan} {
			if len(bundle.RulesFor(kind)) > 0 {
				return fmt.Errorf("%w: %s policies require a named component; register the root with atmos stack add first", errUtils.ErrInvalidFlagValue, kind)
			}
		}
	}
	return nil
}
