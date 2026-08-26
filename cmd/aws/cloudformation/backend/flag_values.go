package backend

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// flagAutoApprove is create/update's skip-confirmation flag, matching
// cmd/aws/cloudformation's own flagAutoApprove naming convention.
const flagAutoApprove = "auto-approve"

// stackFlagCompletion reuses cmd/terraform/shared's generic stack-name
// completion (component/stack listing is not terraform-specific despite the
// package's name — cmd/terraform/backend already reuses it verbatim).
var stackFlagCompletion = shared.StackFlagCompletion

// getCommandFlagStack is used only as a compatibility fallback for tests and
// callers that invoke RunE directly without Cobra first parsing the
// command's flag set. Normal execution obtains parsed values from
// StandardParser.
func getCommandFlagStack(cmd *cobra.Command) string {
	if flag := cmd.Flags().Lookup("stack"); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	if flag := cmd.InheritedFlags().Lookup("stack"); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	return ""
}

// getCommandFlagBool reads the force flag, whose value is not part of
// StandardOptions. The second return value reports whether the flag was
// explicitly changed on the CLI.
func getCommandFlagBool(cmd *cobra.Command, name string) (bool, bool) {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		value, err := strconv.ParseBool(flag.Value.String())
		if err == nil {
			return value, true
		}
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil && flag.Changed {
		value, err := strconv.ParseBool(flag.Value.String())
		if err == nil {
			return value, true
		}
	}
	return false, false
}

// Package-level seams so tests can override the describe-stacks/list-components
// calls made by componentArgCompletion without hitting real stack config.
var (
	backendInitCliConfig     = cfg.InitCliConfig
	backendDescribeStacks    = e.ExecuteDescribeStacks
	backendListAllComponents = component.ListAllComponents
)

// componentArgCompletion returns names for native aws/cloudformation
// components, mirroring cmd/aws/cloudformation/cloudformation.go's own
// componentArgCompletion (unexported there, so re-implemented here rather
// than exported solely for this package's benefit).
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	stack, _ := cmd.Flags().GetString("stack")
	info := schema.ConfigAndStacksInfo{Stack: stack}
	atmosConfig, err := backendInitCliConfig(info, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := backendDescribeStacks(&atmosConfig, stack, nil, []string{cfg.CloudFormationComponentType}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := backendListAllComponents(context.Background(), cfg.CloudFormationComponentType, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
}
