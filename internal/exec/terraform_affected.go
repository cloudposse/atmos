package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
		)
		return affectedList, err
	default:
		affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRefCheckout(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
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
				affected.Component,
				affected.Stack,
				"",
				"",
				affected.Dependents,
				args,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
