package helmfile

import (
	"errors"
	"os"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// isHelpRequest checks if any of the arguments indicate a help request.
func isHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

// handleHelpRequest checks if the user requested help and displays it.
// Returns true if help was requested and displayed.
func handleHelpRequest(cmd *cobra.Command, args []string) bool {
	defer perf.Track(nil, "helmfile.handleHelpRequest")()

	if isHelpRequest(args) {
		_ = cmd.Help()
		return true
	}
	return false
}

// enableHeatmapIfRequested enables heatmap tracking if the --heatmap flag is present.
func enableHeatmapIfRequested() {
	defer perf.Track(nil, "helmfile.enableHeatmapIfRequested")()

	for _, arg := range os.Args {
		if arg == "--heatmap" {
			perf.EnableTracking(true)
			return
		}
	}
}

// getConfigAndStacksInfo processes command line arguments and returns configuration info.
// This includes handling double-dash separator and resolving path-based component arguments.
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "helmfile.getConfigAndStacksInfo")()

	// Handle double-dash separator.
	var argsAfterDoubleDash []string
	finalArgs := args

	doubleDashIndex := lo.IndexOf(args, "--")
	if doubleDashIndex > 0 {
		finalArgs = lo.Slice(args, 0, doubleDashIndex)
		argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
	}

	info, err := e.ProcessCommandLineArgs(commandName, cmd, finalArgs, argsAfterDoubleDash)
	if err != nil {
		return schema.ConfigAndStacksInfo{}, err
	}

	// Resolve path-based component arguments to component names.
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := resolveComponentPath(&info, commandName); err != nil {
			return schema.ConfigAndStacksInfo{}, err
		}
	}

	return info, nil
}

// resolveComponentPath resolves a path-based component argument to a component name.
// It validates the component exists in the specified stack and handles type mismatches.
func resolveComponentPath(info *schema.ConfigAndStacksInfo, commandName string) error {
	defer perf.Track(nil, "helmfile.resolveComponentPath")()

	// Initialize config with processStacks=true to enable stack-based validation.
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return errUtils.Build(errUtils.ErrPathResolutionFailed).
			WithCause(err).
			Err()
	}

	// Resolve component from path WITH stack validation.
	// This will detect type mismatches (e.g., terraform path for helmfile command).
	resolvedComponent, err := e.ResolveComponentFromPath(
		&atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		commandName, // Component type is the command name (terraform, helmfile, packer).
	)
	if err != nil {
		return handlePathResolutionError(err)
	}

	log.Debug("Resolved component from path",
		"original_path", info.ComponentFromArg,
		"resolved_component", resolvedComponent,
		"stack", info.Stack,
	)

	info.ComponentFromArg = resolvedComponent
	info.NeedsPathResolution = false // Mark as resolved.
	return nil
}

// handlePathResolutionError wraps path resolution errors with appropriate hints.
func handlePathResolutionError(err error) error {
	defer perf.Track(nil, "helmfile.handlePathResolutionError")()

	// These errors already have detailed hints from the resolver, return directly.
	if errors.Is(err, errUtils.ErrAmbiguousComponentPath) ||
		errors.Is(err, errUtils.ErrComponentNotInStack) ||
		errors.Is(err, errUtils.ErrStackNotFound) ||
		errors.Is(err, errUtils.ErrUserAborted) ||
		errors.Is(err, errUtils.ErrComponentTypeMismatch) {
		return err
	}
	// Generic path resolution error - add hint.
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithCause(err).
		WithHint("Make sure the path is within your component directories").
		Err()
}

// addStackCompletion adds stack completion to a command.
func addStackCompletion(cmd *cobra.Command) {
	defer perf.Track(nil, "helmfile.addStackCompletion")()

	cmd.PersistentFlags().StringP("stack", "s", "", "The stack flag specifies the environment or configuration set for deployment in Atmos CLI.")
	_ = cmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
}

// stackFlagCompletion provides shell completion for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "helmfile.stackFlagCompletion")()

	// Return empty completion - the actual completion logic would need to be implemented.
	return nil, cobra.ShellCompDirectiveNoFileComp
}
