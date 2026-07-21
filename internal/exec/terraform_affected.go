package exec

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// extractAffectedNodeIDs extracts node IDs from the affected components list.
func extractAffectedNodeIDs(affectedList []schema.Affected) []string {
	nodeIDs := make([]string, len(affectedList))
	for i := range affectedList {
		nodeIDs[i] = fmt.Sprintf("%s-%s", affectedList[i].Component, affectedList[i].Stack)
	}
	return nodeIDs
}

// FilterTerraformAffected narrows the affected list to items that `atmos terraform
// plan/apply --affected` should actually execute: terraform components only, and
// only those that still exist (not deleted in HEAD). Helmfile and Packer items
// belong to their own subcommands; deleted components have no on-disk module so
// terraform plan/apply against them would fail or no-op. See issue #2361. Exported
// so other terraform-affected dispatchers (e.g. `atmos terraform lint --affected`)
// reuse the same filter instead of re-declaring it.
func FilterTerraformAffected(affectedList []schema.Affected) []schema.Affected {
	defer perf.Track(nil, "exec.FilterTerraformAffected")()

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

// GetAffectedComponents retrieves the list of affected components based on the
// provided arguments, dispatching to the right git-target resolution mode
// (repo path / clone-and-diff / local ref checkout). Exported so other
// affected-target dispatchers (e.g. `atmos terraform lint --affected`) reuse
// this switch instead of re-declaring it against the same three
// ExecuteDescribeAffectedWith* functions.
func GetAffectedComponents(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	defer perf.Track(nil, "exec.GetAffectedComponents")()

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
	return ExecuteTerraformAffectedWithContext(context.Background(), args, info)
}

// ExecuteTerraformAffectedWithContext executes affected Terraform components through
// the shared graph-backed scheduler path.
func ExecuteTerraformAffectedWithContext(ctx context.Context, args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffectedWithContext")()

	if args == nil {
		return errUtils.ErrNilParam
	}
	if info == nil {
		return errUtils.ErrNilParam
	}

	authDisabled := args.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return err
	}
	if authManager != nil {
		injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}

	args.CLIConfig = &atmosConfig
	args.AuthManager = authManager
	args.AuthDisabled = authDisabled

	affectedList, err := GetAffectedComponents(args)
	if err != nil {
		return err
	}

	// Drop non-terraform component types (helmfile, packer) and deleted components
	// before resolving dependents — `addDependentsToAffected` is expensive and there
	// is no reason to walk dependency graphs for items we will not execute.
	affectedList = FilterTerraformAffected(affectedList)

	if len(affectedList) == 0 {
		ui.Success("No components affected")
		return nil
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)

	stacks, err := ExecuteDescribeStacksWithAuthDisabledAndMocks(
		&atmosConfig,
		"",  // all stacks; the affected selector already constrains direct matches
		nil, // all components; graph filtering applies the selected affected set
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
		authDisabled,
		info.UseMocks,
	)
	if err != nil {
		return err
	}

	return scheduleradapters.ExecuteTerraform(ctx, scheduleradapters.TerraformOptions{
		AtmosConfig: &atmosConfig,
		Info:        info,
		Stacks:      stacks,
		Executor:    executeTerraformQueryComponent,
		Selection: &scheduleradapters.TerraformSelection{
			NodeIDs:           extractAffectedNodeIDs(affectedList),
			IncludeDependents: args.IncludeDependents,
		},
	})
}
