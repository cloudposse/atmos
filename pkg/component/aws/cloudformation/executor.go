package cloudformation

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	sharedoutput "github.com/cloudposse/atmos/pkg/output"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Seams for testing.
var (
	initCliConfig                    = cfg.InitCliConfig
	processStacks                    = e.ProcessStacks
	setupComponentAuthForCLI         = e.SetupComponentAuthForCLI
	provisionAndResolveComponentPath = component.ProvisionAndResolveComponentPath
	getHooks                         = hooks.GetHooks
)

// opContext bundles the request-scoped values threaded through the operation
// dispatch, keeping each function's own argument list short.
type opContext struct {
	Ctx         context.Context
	AtmosConfig *schema.AtmosConfiguration
	Info        *schema.ConfigAndStacksInfo
	Flags       map[string]any
}

// Execute runs a single aws/cloudformation component operation.
func Execute(ctx *component.ExecutionContext, operation Operation) error {
	defer perf.Track(ctx.AtmosConfig, "cloudformation.ExecuteOperation")()

	info := ctx.ConfigAndStacksInfo
	info.ComponentType = cfg.CloudFormationComponentType
	if info.SubCommand == "" {
		info.SubCommand = ctx.SubCommand
	}
	if info.SubCommand == "" {
		info.SubCommand = string(operation)
	}
	info.CliArgs = []string{cfg.CloudFormationComponentType, info.SubCommand}

	atmosConfig, err := initCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.All || info.Affected || len(info.Tags) > 0 || len(info.Labels) > 0 {
		return executeBulk(ctx, &atmosConfig, &info, operation)
	}
	return executeSingle(ctx, &atmosConfig, &info, operation)
}

// executeSingle runs the operation for a single component (the non-bulk path).
func executeSingle(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation) error {
	discovered, err := processStacks(atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return err
	}
	*info = discovered
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	if err := (&ComponentProvider{}).ValidateComponent(info.ComponentSection); err != nil {
		return err
	}

	if operation != OperationRender {
		authManager, err := setupComponentAuthForCLI(atmosConfig, info)
		if err != nil {
			return err
		}
		info.AuthManager = authManager
	}

	spec, err := resolveSpecAndTemplate(atmosConfig, info, operation)
	if err != nil {
		return err
	}

	return runWithHooks(ctx, atmosConfig, info, operation, spec)
}

// resolveSpecAndTemplate resolves the component's on-disk path (including JIT
// source provisioning), builds the SDK-ready stackSpec, and — for every
// operation except delete, which needs no template — loads the template body,
// registers NoEcho values with the masker, and loads the stack policy.
func resolveSpecAndTemplate(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation) (*stackSpec, error) {
	componentPath, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return nil, err
	}
	componentPath, _, err = provisionAndResolveComponentPath(context.Background(), provisioner.OutputWriters{}, atmosConfig, info, cfg.CloudFormationComponentType, componentPath)
	if err != nil {
		return nil, err
	}

	spec, err := buildStackSpec(info.ComponentSection)
	if err != nil {
		return nil, err
	}

	if operation == OperationDelete {
		return spec, nil
	}

	spec.TemplateBody, err = loadTemplateBody(componentPath, spec)
	if err != nil {
		return nil, err
	}
	registerNoEchoValues(spec.TemplateBody, spec)

	spec.StackPolicyBody, err = loadStackPolicyBody(componentPath, spec)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// runWithHooks runs the before/after hooks around the operation.
func runWithHooks(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation, spec *stackSpec) error {
	hookSet, err := getHooks(atmosConfig, info)
	if err != nil {
		return err
	}
	before, after := eventsFor(operation)
	if err := hookSet.RunAll(before, atmosConfig, info, nil, nil); err != nil {
		return err
	}

	octx := &opContext{Ctx: context.Background(), AtmosConfig: atmosConfig, Info: info, Flags: ctx.Flags}
	_, opErr := runOperation(octx, operation, spec)
	if opErr != nil {
		return opErr
	}

	return hookSet.RunAll(after, atmosConfig, info, nil, nil)
}

// eventsFor maps an Operation to its before/after hook events.
func eventsFor(operation Operation) (hooks.HookEvent, hooks.HookEvent) {
	switch operation {
	case OperationDiff:
		return hooks.BeforeAwsCloudFormationDiff, hooks.AfterAwsCloudFormationDiff
	case OperationApply:
		return hooks.BeforeAwsCloudFormationApply, hooks.AfterAwsCloudFormationApply
	case OperationDelete:
		return hooks.BeforeAwsCloudFormationDelete, hooks.AfterAwsCloudFormationDelete
	default:
		return hooks.HookEvent(""), hooks.HookEvent("")
	}
}

// runOperation dispatches to the requested aws/cloudformation operation.
func runOperation(octx *opContext, operation Operation, spec *stackSpec) (map[string]any, error) {
	summary := map[string]any{"stack_name": spec.StackName}

	if operation == OperationRender {
		summary["template"] = spec.TemplateBody
		return summary, nil
	}

	region := resolveRegion(octx.Info.ComponentSection)
	awsCfg, err := buildAWSConfig(octx.Ctx, octx.Info, region)
	if err != nil {
		return summary, err
	}
	client := newClient(awsCfg)

	switch operation {
	case OperationValidate:
		return summary, validateTemplate(octx.Ctx, client, spec.TemplateBody)
	case OperationDiff:
		return runDiff(octx.Ctx, client, spec, summary)
	case OperationApply:
		return runApply(octx, client, spec, summary)
	case OperationDelete:
		return runDelete(octx.Ctx, client, octx.Flags, spec, summary)
	case OperationOutput:
		return runOutput(octx.Ctx, client, spec.StackName, octx.Flags, summary)
	default:
		return summary, fmt.Errorf("%w: %q", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, operation)
	}
}

// runDiff creates (or reuses) a changeset and renders the predicted changes
// without executing it.
func runDiff(ctx context.Context, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	result, err := createChangeSet(ctx, client, spec)
	if err != nil {
		return summary, err
	}
	summary["changeset_id"] = result.ChangeSetID
	summary["no_op"] = result.NoOp
	summary["changes"] = result.Changes
	return summary, nil
}

// runApply executes the changeset (creating or updating the stack) and renders
// the end-of-deploy Outputs summary — the same view the standalone `output` verb
// renders, per the PRD's direct response to the #1 Rain-user complaint.
func runApply(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	deploySummary, result, err := deliverApply(octx, client, spec)
	for k, v := range deploySummary {
		summary[k] = v
	}
	if err != nil {
		return summary, err
	}
	if result != nil {
		summary["changeset_id"] = result.ChangeSetID
		summary["no_op"] = result.NoOp
	}

	if spec.StackPolicyBody != "" {
		if err := setStackPolicy(octx.Ctx, client, spec); err != nil {
			return summary, err
		}
	}

	outputs, err := describeStackOutputs(octx.Ctx, client, spec.StackName)
	if err != nil {
		return summary, err
	}
	summary["outputs"] = outputs
	renderOutputsSummary(outputs, octx.Flags)
	return summary, nil
}

// runDelete deletes the stack and streams events until it's gone.
func runDelete(ctx context.Context, client CloudFormationClient, flags map[string]any, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	opts := deleteOptionsFromFlags(flags)
	if err := deleteStack(ctx, client, spec, opts); err != nil {
		return summary, err
	}
	status, err := streamStackEvents(ctx, client, spec.StackName)
	if err != nil {
		return summary, err
	}
	summary["final_status"] = string(status)
	if isFailedStackStatus(status) {
		return summary, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationChangeSetFailed, spec.StackName, status)
	}
	return summary, nil
}

// deleteOptionsFromFlags builds deleteOptions from the command's flags.
func deleteOptionsFromFlags(flags map[string]any) deleteOptions {
	opts := deleteOptions{}
	if v, ok := flags["retain-resources"].([]string); ok {
		opts.RetainResources = v
	}
	if v, ok := flags["disable-termination-protection"].(bool); ok {
		opts.DisableTerminationProtection = v
	}
	return opts
}

// runOutput renders the deployed stack's Outputs via the standalone `output`
// verb's path (also called by runApply for the end-of-deploy summary).
func runOutput(ctx context.Context, client CloudFormationClient, stackName string, flags map[string]any, summary map[string]any) (map[string]any, error) {
	outputs, err := describeStackOutputs(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["outputs"] = outputs
	renderOutputsSummary(outputs, flags)
	return summary, nil
}

// renderOutputsSummary writes the Outputs to the data channel (stdout) in the
// requested format (default: table), reusing the shared pkg/output formatter —
// the full standard format set (json/yaml/hcl/env/dotenv/bash/csv/tsv/github).
func renderOutputsSummary(outputs map[string]any, flags map[string]any) {
	format := sharedoutput.FormatTable
	if f, ok := flags["format"].(string); ok && f != "" {
		format = sharedoutput.Format(f)
	}

	opts := sharedoutput.FormatOptions{}
	if flatten, ok := flags["flatten"].(bool); ok {
		opts.Flatten = flatten
	}
	if uppercase, ok := flags["uppercase"].(bool); ok {
		opts.Uppercase = uppercase
	}

	rendered, err := sharedoutput.FormatOutputsWithOptions(outputs, format, opts)
	if err != nil {
		ui.Error(fmt.Sprintf("failed to format outputs: %v", err))
		return
	}
	_ = data.Write(rendered)
}
