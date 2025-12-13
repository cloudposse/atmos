package workflow

import (
	"sort"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const stackHint = "Filter by stack name or pattern"

// addStackCompletion adds the --stack flag with shell completion to a command.
func addStackCompletion(cobraCmd *cobra.Command) {
	if cobraCmd.Flag("stack") == nil {
		cobraCmd.PersistentFlags().StringP("stack", "s", "", stackHint)
	}
	if err := cobraCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion); err != nil {
		log.Trace("Failed to register stack flag completion", "error", err)
	}
}

// stackFlagCompletion provides shell completion for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// List all stacks.
	output, err := listAllStacks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listAllStacks returns all available stacks.
func listAllStacks() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	output, err := l.FilterAndListStacks(stacksMap, "")
	return output, err
}

// identityFlagCompletion provides shell completion for identity flags by fetching
// available identities from the Atmos configuration.
func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
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

// addIdentityCompletion registers shell completion for the identity flag if present on the command.
func addIdentityCompletion(cmd *cobra.Command) {
	if cmd.Flag("identity") != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}

// workflowNameCompletion provides shell completion for workflow names.
// Returns all available workflow names with their file context.
func workflowNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first positional argument (workflow name).
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get all workflows.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		// Silently fail - completion should be non-intrusive.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	workflows, _, _, err := e.ExecuteDescribeWorkflows(atmosConfig)
	if err != nil {
		// Silently fail - completion should be non-intrusive.
		// This can happen if there are invalid workflow files.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Build completion list with file context.
	// Format: "workflow-name\tfile.yaml" - tab-separated for shell completion descriptions.
	completions := make([]string, 0, len(workflows))
	seen := make(map[string]bool)

	for _, wf := range workflows {
		// Each workflow name might appear multiple times (in different files).
		// We show all occurrences with their file context.
		key := wf.Workflow + "\t" + wf.File
		if !seen[key] {
			completions = append(completions, key)
			seen[key] = true
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
