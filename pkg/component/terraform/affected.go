package terraform

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
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

// ExecuteTerraformAffected executes `atmos terraform <command> --affected`.
func (e *Executor) ExecuteTerraformAffected(args *component.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffected")()
	return e.ExecuteTerraformAffectedWithContext(context.Background(), args, info)
}

// ExecuteTerraformAffectedWithContext executes affected Terraform components through
// the shared graph-backed scheduler path.
func (e *Executor) ExecuteTerraformAffectedWithContext(ctx context.Context, args *component.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffectedWithContext")()

	if args == nil || info == nil {
		return errUtils.ErrNilParam
	}

	authDisabled := applyAffectedAuthDisable(args, info)

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return err
	}
	if authManager != nil {
		e.injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}

	args.CLIConfig = &atmosConfig
	args.AuthManager = authManager
	args.AuthDisabled = authDisabled

	affectedList, err := e.services.GetAffectedComponents(args)
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

	stacks, err := e.describeAffectedTerraformStacks(&atmosConfig, info, authManager, authDisabled)
	if err != nil {
		return err
	}

	return scheduleradapters.ExecuteTerraform(ctx, scheduleradapters.TerraformOptions{
		AtmosConfig: &atmosConfig,
		Info:        info,
		Stacks:      stacks,
		Executor:    e.executeTerraformQueryComponent,
		Selection:   affectedTerraformSelection(affectedList, args, info),
	})
}

// affectedTerraformSelection builds the scheduler selection for the affected
// set. The args.IncludeDependents bool bridges the depth-carrying
// --include-dependents flag, so the selection must also carry the parsed depth
// from info: the closure spec merges selection and info keeping the most
// permissive depth per direction, and a zero DependentDepth means *unlimited*
// in dependency.Filter encoding — leaving it unset would silently erase a
// user-supplied --include-dependents=N bound. When the bool is set without a
// flag-side depth (e.g. dependents forced programmatically), zero is kept and
// correctly means unlimited.
func affectedTerraformSelection(affectedList []schema.Affected, args *component.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) *scheduleradapters.TerraformSelection {
	selection := &scheduleradapters.TerraformSelection{
		NodeIDs:           extractAffectedNodeIDs(affectedList),
		IncludeDependents: args.IncludeDependents,
	}
	if args.IncludeDependents && info != nil {
		selection.DependentDepth = scheduleradapters.FlagDepthToFilterDepth(info.IncludeDependents)
	}
	return selection
}

// applyAffectedAuthDisable preserves an explicit authentication opt-out for all selected nodes.
func applyAffectedAuthDisable(args *component.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) bool {
	authDisabled := args.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
	}

	return authDisabled
}

// describeAffectedTerraformStacks evaluates the full graph before applying the affected selection.
func (e *Executor) describeAffectedTerraformStacks(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, authManager auth.AuthManager, authDisabled bool) (map[string]any, error) {
	return e.services.ExecuteDescribeStacksWithAuthDisabledAndMocks(
		atmosConfig,
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
}
