package internal

import (
	"errors"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunHooks fires lifecycle hooks for the active component/stack at the given
// event. This is the shared entrypoint used by both the built-in terraform
// command path and the custom-command path (PR #1904 component types).
//
// `commandName` is the atmos command bucket the args originated from
// (e.g. "terraform"). Pass empty string for custom commands.
//
// `populate` (optional) lets the caller mutate the resolved ConfigAndStacksInfo
// before hook resolution. Custom commands use this to inject ComponentType
// and OutputsFilePath, which the `store` hook uses to look up `.foo`-style
// output references in the ATMOS_OUTPUTS file the steps wrote to.
//
// `preResolvedComponent` (optional) skips the implicit ExecuteDescribeComponent
// call inside GetHooks. The custom-command path already resolved the component
// to populate `{{ .Component.* }}` templates, and the built-in describe path
// does not understand custom component types — re-describing would fail with
// "component not found". When non-nil, hooks are built directly from this map.
func RunHooks(
	event h.HookEvent,
	cmd *cobra.Command,
	args []string,
	commandName string,
	populate func(info *schema.ConfigAndStacksInfo),
	preResolvedComponent map[string]any,
) error {
	finalArgs := append([]string{cmd.Name()}, args...)

	processCmd := commandName
	if processCmd == "" {
		processCmd = cmd.Name()
	}

	info, err := e.ProcessCommandLineArgs(processCmd, cmd, finalArgs, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return errors.Join(errUtils.ErrInitializeCLIConfig, err)
	}

	if populate != nil {
		populate(&info)
	}

	var hooks *h.Hooks
	if preResolvedComponent != nil {
		hooks, err = h.HooksFromComponent(&atmosConfig, &info, preResolvedComponent)
	} else {
		hooks, err = h.GetHooks(&atmosConfig, &info)
	}
	if err != nil {
		return errors.Join(errUtils.ErrGetHooks, err)
	}

	if hooks == nil || !hooks.HasHooks() {
		return nil
	}

	log.Info("Running hooks", "event", event)
	if err := hooks.RunAll(event, &atmosConfig, &info, cmd, args); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return nil
}
