package shared

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const errWrapFormat = "%w: %w"

// IsMultiComponentExecution checks if the command should be routed to multi-component execution.
func IsMultiComponentExecution(info *schema.ConfigAndStacksInfo) bool {
	return info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "")
}

// HasMultiComponentFlags checks if any multi-component flags are set.
func HasMultiComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.All || info.Affected || len(info.Components) > 0 || info.Query != ""
}

// HasNonAffectedMultiFlags checks if multi-component flags, excluding --affected, are set.
func HasNonAffectedMultiFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.All || len(info.Components) > 0 || info.Query != ""
}

// HasSingleComponentFlags checks if single-component flags are set.
func HasSingleComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.PlanFile != "" || info.UseTerraformPlan
}

// CheckTerraformFlags checks the usage of the single-component and multi-component flags.
func CheckTerraformFlags(info *schema.ConfigAndStacksInfo) error {
	if info.ComponentFromArg != "" && HasMultiComponentFlags(info) {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	if info.Affected && HasNonAffectedMultiFlags(info) {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}
	if HasSingleComponentFlags(info) && HasMultiComponentFlags(info) {
		return errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags
	}
	return nil
}

// HandleInteractiveIdentitySelection handles the case where --identity was used without a value.
func HandleInteractiveIdentitySelection(info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrInitializeCLIConfig, err)
	}

	if len(atmosConfig.Auth.Providers) == 0 && len(atmosConfig.Auth.Identities) == 0 {
		return fmt.Errorf("%w: no authentication configured", errUtils.ErrNoIdentitiesAvailable)
	}

	authManager, err := auth.CreateAndAuthenticateManager(
		cfg.IdentityFlagSelectValue,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
	)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	selectedIdentity, err := authManager.GetDefaultIdentity(true)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			log.Debug("User aborted identity selection, exiting with SIGINT code")
			return errUtils.WithExitCode(err, errUtils.ExitCodeSIGINT)
		}
		return fmt.Errorf(errWrapFormat, errUtils.ErrDefaultIdentity, err)
	}

	info.Identity = selectedIdentity
	return nil
}

// ResolveAndPromptForArgs handles path resolution and interactive prompts for component/stack.
func ResolveAndPromptForArgs(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := ResolveComponentPath(info, cfg.TerraformComponentType); err != nil {
			return err
		}
	}
	return HandleInteractiveComponentStackSelection(info, cmd)
}

// HandleInteractiveComponentStackSelection prompts for missing component and stack.
func HandleInteractiveComponentStackSelection(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	if HasMultiComponentFlags(info) || info.NeedHelp {
		return nil
	}

	if info.Stack != "" && info.ComponentFromArg == "" {
		if err := ValidateStackExists(cmd, info.Stack); err != nil {
			return err
		}
	}

	if info.ComponentFromArg != "" && info.Stack != "" {
		return nil
	}

	if err := promptMissingComponent(info, cmd); err != nil {
		return err
	}
	if err := promptMissingStack(info, cmd); err != nil {
		return err
	}

	return nil
}

func promptMissingComponent(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	if info.ComponentFromArg != "" {
		return nil
	}
	component, err := PromptForComponent(cmd, info.Stack)
	if err = HandlePromptError(err, "component"); err != nil {
		return err
	}
	info.ComponentFromArg = component
	return nil
}

func promptMissingStack(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	if info.Stack != "" {
		return nil
	}
	stack, err := PromptForStack(cmd, info.ComponentFromArg)
	if err = HandlePromptError(err, "stack"); err != nil {
		return err
	}
	info.Stack = stack
	return nil
}

// ResolveComponentPath resolves a path-based component argument to a component name.
func ResolveComponentPath(info *schema.ConfigAndStacksInfo, commandName string) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrPathResolutionFailed, err)
	}

	resolvedComponent, err := e.ResolveComponentFromPath(
		&atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		commandName,
	)
	if err != nil {
		return HandlePathResolutionError(err)
	}

	log.Debug(
		"Resolved component from path",
		"original_path", info.ComponentFromArg,
		"resolved_component", resolvedComponent,
		"stack", info.Stack,
	)

	info.ComponentFromArg = resolvedComponent
	info.NeedsPathResolution = false
	return nil
}

// HandlePathResolutionError wraps path resolution errors with appropriate hints.
func HandlePathResolutionError(err error) error {
	if errors.Is(err, errUtils.ErrAmbiguousComponentPath) ||
		errors.Is(err, errUtils.ErrComponentNotInStack) ||
		errors.Is(err, errUtils.ErrStackNotFound) ||
		errors.Is(err, errUtils.ErrUserAborted) {
		return err
	}
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithCause(err).
		WithHint("Make sure the path is within your component directories").
		Err()
}

// RegisterCompletions registers component and identity completion functions for a terraform subcommand.
func RegisterCompletions(tfCmd *cobra.Command) {
	tfCmd.ValidArgsFunction = ComponentsArgCompletion
	addIdentityCompletion(tfCmd)
}

func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(buildConfigAndStacksInfo(cmd), false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

func addIdentityCompletion(cmd *cobra.Command) {
	flag := cmd.Flag("identity")
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup("identity")
	}
	if flag != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}
