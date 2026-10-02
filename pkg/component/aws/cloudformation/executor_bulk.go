package cloudformation

import (
	"fmt"
	"maps"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	sharedoutput "github.com/cloudposse/atmos/pkg/output"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Additional seams for testing (bulk path).
var (
	executeDescribeStacks                = e.ExecuteDescribeStacks
	executeGraph                         = component.ExecuteGraph
	executeAffectedWithRepoPath          = e.ExecuteDescribeAffectedWithTargetRepoPath
	executeAffectedWithRefClone          = e.ExecuteDescribeAffectedWithTargetRefClone
	executeAffectedWithRefCheckout       = e.ExecuteDescribeAffectedWithTargetRefCheckout
	affectedCloudFormationComponentsFunc = affectedCloudFormationComponents
)

// executeBulk discovers the dependency graph without resolving YAML functions, then executes producers
// first for apply and consumers first for delete.
func executeBulk(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
) error {
	authManager, err := authManagerForBulk(atmosConfig, info)
	if err != nil {
		return err
	}

	stacks, err := executeDescribeStacks(
		atmosConfig,
		info.Stack,
		nil,
		[]string{cfg.CloudFormationComponentType},
		nil,
		false,
		!info.DryRun,
		false, // Resolve YAML functions per node, after dependencies have completed.
		true,
		info.Skip,
		authManager,
	)
	if err != nil {
		return err
	}

	selection, err := graphSelectionForBulk(ctx, atmosConfig, info)
	if err != nil {
		return err
	}

	flags, collector, err := withBulkOutputCollector(operation, bulkOperationFlags(operation, ctx.Flags))
	if err != nil {
		return err
	}

	graphErr := executeGraph(ctx.GoContext(), &component.GraphExecutionOptions{
		Provider:      &ComponentProvider{},
		ReverseOrder:  operation == OperationDelete,
		AtmosConfig:   atmosConfig,
		Info:          info,
		Stacks:        stacks,
		ComponentType: cfg.CloudFormationComponentType,
		SubCommand:    string(operation),
		Flags:         flags,
		Selection:     selection,
	})
	if graphErr == nil {
		graphErr = collector.flush()
	}
	return finishFmtBulk(flags, graphErr)
}

func authManagerForBulk(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	if info.DryRun || info.Identity == "" {
		return nil, nil
	}
	authConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)
	authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(info.Identity, authConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		return nil, err
	}
	propagateAuth(info, authManager)
	return authManager, nil
}

func graphSelectionForBulk(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
) (*component.GraphSelection, error) {
	if !info.Affected {
		return nil, nil
	}

	affected, err := affectedCloudFormationComponentsFunc(ctx, atmosConfig, info)
	if err != nil {
		return nil, err
	}

	nodeIDs := make([]string, 0, len(affected))
	for i := range affected {
		item := &affected[i]
		if item.Deleted || item.ComponentType != cfg.CloudFormationComponentType {
			continue
		}
		nodeIDs = append(nodeIDs, component.GraphNodeID(item.Component, item.Stack))
	}

	includeDependents, _ := ctx.Flags["include-dependents"].(bool)
	return &component.GraphSelection{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   includeDependents,
	}, nil
}

// affectedCloudFormationComponents selects changed components without resolving output dependencies
// that may not exist until execution.
func affectedCloudFormationComponents(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
) ([]schema.Affected, error) {
	args := e.DescribeAffectedCmdArgs{
		CLIConfig:                   atmosConfig,
		Stack:                       info.Stack,
		ProcessTemplates:            !info.DryRun,
		ProcessYamlFunctions:        false,
		Skip:                        info.Skip,
		IncludeSettings:             false,
		IncludeDependents:           false,
		IncludeSpaceliftAdminStacks: false,
	}

	applyAffectedFlags(&args, ctx.Flags)

	var authManager auth.AuthManager
	if manager, ok := info.AuthManager.(auth.AuthManager); ok {
		authManager = manager
	}
	args.AuthManager = authManager
	args.AuthDisabled = info.AuthDisabled

	return dispatchAffected(atmosConfig, &args, authManager)
}

func applyAffectedFlags(args *e.DescribeAffectedCmdArgs, flags map[string]any) {
	if value, ok := flags["repo-path"].(string); ok {
		args.RepoPath = value
	}
	applyAffectedBaseFlag(args, flags)
	if value, ok := flags["ref"].(string); ok && value != "" {
		args.Ref = value
	}
	if value, ok := flags["sha"].(string); ok && value != "" {
		args.SHA = value
	}
	if value, ok := flags["ssh-key"].(string); ok {
		args.SSHKeyPath = value
	}
	if value, ok := flags["ssh-key-password"].(string); ok {
		args.SSHKeyPassword = value
	}
	if value, ok := flags["clone-target-ref"].(bool); ok {
		args.CloneTargetRef = value
	}
}

func applyAffectedBaseFlag(args *e.DescribeAffectedCmdArgs, flags map[string]any) {
	value, ok := flags["base"].(string)
	if !ok {
		return
	}
	if ci.IsCommitSHA(value) {
		args.SHA = value
		return
	}
	args.Ref = value
}

func dispatchAffected(
	atmosConfig *schema.AtmosConfiguration,
	args *e.DescribeAffectedCmdArgs,
	authManager auth.AuthManager,
) ([]schema.Affected, error) {
	switch {
	case args.RepoPath != "":
		affected, _, _, _, err := executeAffectedWithRepoPath(
			atmosConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	case args.CloneTargetRef:
		affected, _, _, _, err := executeAffectedWithRefClone(
			atmosConfig,
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
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	default:
		affected, _, _, _, err := executeAffectedWithRefCheckout(
			atmosConfig,
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
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	}
}

// bulkOperationFlags preserves selection context for fmt after graph dispatch
// clears selectors to prevent recursively entering the bulk path.
func bulkOperationFlags(operation Operation, flags map[string]any) map[string]any {
	if operation != OperationFmt {
		return flags
	}
	result := maps.Clone(flags)
	if result == nil {
		result = make(map[string]any)
	}
	attachFmtBulkState(result)
	result[fmtSkipInlineKey] = true
	return result
}

// bulkOutputCollectorKey is the private flag through which a bulk `output` run
// hands its collector to every component node (the graph copies flags to each
// node). It is not a CLI flag.
const bulkOutputCollectorKey = "bulk-output-collector"

// bulkOutputCollector makes `output --all` (and --tags/--labels/--affected)
// produce one coherent result instead of unlabeled per-component output:
//   - json and yaml collect every component's outputs and print ONE document
//     keyed by stack, then component, once the whole run succeeds.
//   - table prints a title above each component's table.
//
// Other formats (env, dotenv, bash, hcl, csv, tsv, github) stay flat,
// per-component streams: they have no way to carry a stack/component key.
type bulkOutputCollector struct {
	format sharedoutput.Format
	// docs maps stack -> component -> outputs for the structured formats.
	docs map[string]any
}

// withBulkOutputCollector installs a collector for a bulk `output` run in a copy
// of flags (the caller's map is never modified) and returns it. Any other
// operation, and formats with no stack/component structure, get a nil collector.
func withBulkOutputCollector(operation Operation, flags map[string]any) (map[string]any, *bulkOutputCollector, error) {
	if operation != OperationOutput {
		return flags, nil, nil
	}
	format, err := outputFormat(flags)
	if err != nil {
		return nil, nil, err
	}
	switch format {
	case sharedoutput.FormatJSON, sharedoutput.FormatYAML, sharedoutput.FormatTable:
	default:
		return flags, nil, nil
	}
	collector := &bulkOutputCollector{format: format, docs: make(map[string]any)}
	result := maps.Clone(flags)
	if result == nil {
		result = make(map[string]any)
	}
	result[bulkOutputCollectorKey] = collector
	return result, collector, nil
}

// runOutputOperation runs the `output` verb for one component. In a bulk run the
// bulk collector decides how it is presented; otherwise the outputs are rendered
// directly.
func runOutputOperation(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	collector, _ := octx.Flags[bulkOutputCollectorKey].(*bulkOutputCollector)
	if collector == nil {
		return runOutput(octx.Ctx, client, spec.StackName, octx.Flags, summary)
	}
	return collector.run(octx, client, spec.StackName, summary)
}

// run handles one component of a bulk output run: structured formats record the
// outputs for the final document, table prints a title and the table.
func (c *bulkOutputCollector) run(octx *opContext, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	stack, component := octx.Info.Stack, octx.Info.ComponentFromArg
	if c.format == sharedoutput.FormatTable {
		if err := data.Writeln(fmt.Sprintf("%s in stack %s:", component, stack)); err != nil {
			return summary, err
		}
		return runOutput(octx.Ctx, client, stackName, octx.Flags, summary)
	}
	return c.record(octx, client, stackName, summary)
}

// record fetches one component's presented (masked) outputs and files them under
// stack and component.
func (c *bulkOutputCollector) record(octx *opContext, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	stack, component := octx.Info.Stack, octx.Info.ComponentFromArg
	outputs, err := presentedStackOutputs(octx.Ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["outputs"] = outputs
	byComponent, _ := c.docs[stack].(map[string]any)
	if byComponent == nil {
		byComponent = make(map[string]any)
		c.docs[stack] = byComponent
	}
	byComponent[component] = outputs
	return summary, nil
}

// flush prints the single aggregated document for the structured formats. It is
// a no-op for a nil collector and for table output, which streams per component.
func (c *bulkOutputCollector) flush() error {
	if c == nil || c.format == sharedoutput.FormatTable {
		return nil
	}
	var rendered string
	var err error
	if c.format == sharedoutput.FormatJSON {
		rendered, err = encodeJSON(c.docs)
	} else {
		rendered, err = sharedoutput.FormatOutputs(c.docs, c.format)
	}
	if err != nil {
		return err
	}
	return data.Write(rendered)
}
