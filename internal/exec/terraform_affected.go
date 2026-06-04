package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// filterTerraformAffected narrows the affected list to items that `atmos terraform
// plan/apply --affected` should actually execute: terraform components only, and
// only those that still exist (not deleted in HEAD). Helmfile and Packer items
// belong to their own subcommands; deleted components have no on-disk module so
// terraform plan/apply against them would fail or no-op. See issue #2361.
func filterTerraformAffected(affectedList []schema.Affected) []schema.Affected {
	filtered := affectedList[:0]
	for i := range affectedList {
		a := &affectedList[i]
		if a.ComponentType != cfg.TerraformComponentType {
			continue
		}
		if a.Deleted {
			continue
		}
		filtered = append(filtered, *a)
	}
	return filtered
}

// getAffectedComponents retrieves the list of affected components based on the provided arguments.
func getAffectedComponents(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	defer perf.Track(nil, "exec.getAffectedComponents")()

	if args == nil {
		return nil, errUtils.ErrNilParam
	}

	switch {
	case args.RepoPath != "":
		affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
			args.CLIConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	case args.CloneTargetRef:
		affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRefClone(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.SSHKeyPath,
			args.SSHKeyPassword,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	default:
		affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRefCheckout(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.TargetBranch,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	}
}

// ExecuteTerraformAffected executes `atmos terraform <command> --affected`.
func ExecuteTerraformAffected(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffected")()

	if args == nil {
		return errUtils.ErrNilParam
	}
	if info == nil {
		return errUtils.ErrNilParam
	}

	affectedList, err := getAffectedComponents(args)
	if err != nil {
		return err
	}

	// Drop non-terraform component types (helmfile, packer) and deleted components
	// before resolving dependents — `addDependentsToAffected` is expensive and there
	// is no reason to walk dependency graphs for items we will not execute.
	affectedList = filterTerraformAffected(affectedList)

	// Add dependent components for each directly affected component.
	if len(affectedList) > 0 {
		err = addDependentsToAffected(
			args.CLIConfig,
			&affectedList,
			args.IncludeSettings,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			"",
			args.AuthManager,
			args.AuthDisabled,
		)
		if err != nil {
			return err
		}
	}

	return executeAffectedComponents(affectedList, info, args)
}

// executeAffectedComponents processes each affected component in dependency order.
func executeAffectedComponents(affectedList []schema.Affected, info *schema.ConfigAndStacksInfo, args *DescribeAffectedCmdArgs) error {
	defer perf.Track(nil, "exec.executeAffectedComponents")()

	// Early return for empty list - nothing to process.
	if len(affectedList) == 0 {
		ui.Success("No components affected")
		return nil
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)

	for i := 0; i < len(affectedList); i++ {
		affected := &affectedList[i]
		// If the affected component is included in the dependencies of any other component, don't process it now.
		// It will be processed in the dependency order.
		if !affected.IncludedInDependents {
			err = executeTerraformAffectedComponentInDepOrder(
				info,
				affectedList,
				&affectedDepOrderParams{
					AffectedComponent: affected.Component,
					AffectedStack:     affected.Stack,
					Dependents:        affected.Dependents,
				},
				args,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
