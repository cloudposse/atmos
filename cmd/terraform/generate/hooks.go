package generate

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// runGeneratePlanfileHooks fires user-defined hooks and CI plugin hooks for
// the given event on the success path (before/after.terraform.generate.planfile).
//
// This is a focused subset of cmd/terraform/utils.go:runHooksWithOutput tailored
// for `generate planfile`, which does not support multi-component flags, path-
// based component arguments, --verify-plan, or identity selection. A future PR
// (see #2497 PR B) will introduce stdout capture for CI plugin handlers; until
// then, the Output field is empty, which user-defined command hooks do not need.
//
// TODO(#2522): consolidate this and cmd/terraform/utils.go:runHooks* into a
// single shared package (cmd/terraform/shared/hooks.go) so plan, apply, deploy,
// and generate-planfile share one implementation. Today the import direction
// cmd/terraform → cmd/terraform/generate blocks direct reuse.
var runGeneratePlanfileHooks = func(event h.HookEvent, cmd *cobra.Command, args []string) error {
	finalArgs := append([]string{cmd.Name()}, args...)

	info, err := e.ProcessCommandLineArgs("terraform", cmd, finalArgs, nil)
	if err != nil {
		return err
	}

	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return errors.Join(errUtils.ErrInitializeCLIConfig, err)
	}

	hooks, err := h.GetHooks(&atmosConfig, &info)
	if err != nil {
		return err
	}

	if hooks != nil && hooks.HasHooks() {
		log.Info("Running hooks", "event", event)
		if err := hooks.RunAll(event, &atmosConfig, &info, cmd, args); err != nil {
			return err
		}
	}

	// PreRunE reads the Cobra flag directly because pflags are only bound to
	// Viper inside RunE (planfileParser.BindFlagsToViper). Falling back to
	// Viper preserves env-var support (ATMOS_CI, CI).
	forceCIMode, _ := cmd.Flags().GetBool("ci")
	if !forceCIMode {
		forceCIMode = viper.GetBool("ci")
	}

	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:       event,
		AtmosConfig: &atmosConfig,
		Info:        &info,
		ForceCIMode: forceCIMode,
	}); err != nil {
		log.Warn("CI hook execution failed", "error", err)
	}

	return nil
}

// runGeneratePlanfileErrorHook fires the failure-path CI hook with the captured
// command error so plugin handlers can update check runs to failed and surface
// exit codes. Cobra skips PostRunE when RunE returns a non-nil error, so this
// must be invoked from a defer in RunE to mirror cmd/terraform/plan.go's pattern.
//
// Declared as a package-level var so tests can stub it to verify the RunE
// defer-guard contract without invoking real plugin handlers.
var runGeneratePlanfileErrorHook = func(event h.HookEvent, cmd *cobra.Command, args []string, cmdErr error) {
	finalArgs := append([]string{cmd.Name()}, args...)

	info, err := e.ProcessCommandLineArgs("terraform", cmd, finalArgs, nil)
	if err != nil {
		return
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return
	}

	forceCIMode, _ := cmd.Flags().GetBool("ci")
	if !forceCIMode {
		forceCIMode = viper.GetBool("ci")
	}

	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:        event,
		AtmosConfig:  &atmosConfig,
		Info:         &info,
		ForceCIMode:  forceCIMode,
		CommandError: cmdErr,
		ExitCode:     errUtils.GetExitCode(cmdErr),
	}); err != nil {
		log.Warn("CI hook execution failed", "error", err)
	}
}
