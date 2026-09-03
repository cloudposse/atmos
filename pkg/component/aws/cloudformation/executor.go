package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

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
	propagateAuth                    = e.PropagateAuth
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

	if !operationsSkippingAuth[operation] {
		authManager, err := setupComponentAuthForCLI(atmosConfig, info)
		if err != nil {
			return err
		}
		propagateAuth(info, authManager)
	}

	spec, err := resolveSpecAndTemplate(atmosConfig, info, operation)
	if err != nil {
		return err
	}

	return runWithHooks(ctx, atmosConfig, info, operation, spec)
}

// operationsSkippingAuth are operations that never call the CloudFormation API
// and so need no active identity: render (client-side template rendering) and
// fmt (a local YAML round-trip, no different from running it against a file
// with a text editor).
var operationsSkippingAuth = map[Operation]bool{
	OperationRender: true,
	OperationFmt:    true,
}

// operationsSkippingTemplateLoad are operations that act on a deployed stack by
// name/ID (delete, explicit changeset execute/list/delete, drift, get) and never
// send a local template to CloudFormation, so loading and reading the template
// file from disk would be pure overhead.
var operationsSkippingTemplateLoad = map[Operation]bool{
	OperationDelete:            true,
	OperationChangesetExecute:  true,
	OperationChangesetList:     true,
	OperationChangesetDelete:   true,
	OperationDriftDetect:       true,
	OperationDriftDescribe:     true,
	OperationGetTemplate:       true,
	OperationGetPolicy:         true,
	OperationStackSetDelete:    true,
	OperationStackSetInstances: true,
	OperationTree:              true,
	OperationLogs:              true,
	OperationWatch:             true,
}

// resolveSpecAndTemplate resolves the component's on-disk path (including JIT
// source provisioning), builds the SDK-ready stackSpec, and — for every
// operation that sends a local template to CloudFormation — loads the template
// body, registers NoEcho values with the masker, and loads the stack policy.
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

	if operationsSkippingTemplateLoad[operation] {
		return spec, nil
	}

	if spec.TemplateBody == "" {
		spec.TemplateAbsPath = resolveTemplateFilePath(componentPath, spec)
		spec.TemplateBody, err = loadTemplateBody(componentPath, spec)
		if err != nil {
			return nil, err
		}
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

// operationHandler runs one mutating/read operation against an already-built
// CloudFormationClient and stackSpec.
type operationHandler func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error)

// operationHandlers maps every non-render Operation to its handler. A map
// dispatch keeps runOperation a flat lookup instead of a long switch.
var operationHandlers = map[Operation]operationHandler{
	OperationValidate: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return summary, validateTemplate(octx.Ctx, client, spec.StackName, spec.TemplateBody)
	},
	OperationDiff: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runDiff(octx.Ctx, client, spec, summary)
	},
	OperationApply: runApply,
	OperationDelete: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runDelete(octx.Ctx, client, octx.Flags, spec, summary)
	},
	OperationOutput: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runOutput(octx.Ctx, client, spec.StackName, octx.Flags, summary)
	},
	OperationChangesetCreate: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runChangesetCreate(octx.Ctx, client, spec, summary)
	},
	OperationChangesetExecute: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runChangesetExecute(octx.Ctx, client, spec, changesetNameFlag(octx.Flags), summary)
	},
	OperationChangesetList: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runChangesetList(octx.Ctx, client, spec, summary)
	},
	OperationChangesetDelete: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runChangesetDelete(octx.Ctx, client, spec, changesetNameFlag(octx.Flags), summary)
	},
	OperationDriftDetect: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		failOnDrift, _ := octx.Flags["fail-on-drift"].(bool)
		return runDriftDetect(octx.Ctx, client, spec.StackName, failOnDrift, summary)
	},
	OperationDriftDescribe: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runDriftDescribe(octx.Ctx, client, spec.StackName, summary)
	},
	OperationGetTemplate: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runGetTemplate(octx.Ctx, client, spec.StackName, octx.Flags, summary)
	},
	OperationGetPolicy: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runGetPolicy(octx.Ctx, client, spec.StackName, summary)
	},
	OperationStackSetCreate: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		ssCfg, err := resolveStackSetTargetFromContext(octx)
		if err != nil {
			return summary, err
		}
		return runStackSetCreate(octx.Ctx, client, spec, ssCfg, summary)
	},
	OperationStackSetUpdate: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		ssCfg, err := resolveStackSetTargetFromContext(octx)
		if err != nil {
			return summary, err
		}
		return runStackSetUpdate(octx.Ctx, client, spec, ssCfg, summary)
	},
	OperationStackSetDelete: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runStackSetDelete(octx.Ctx, client, spec.StackName, summary)
	},
	OperationStackSetInstances: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runStackSetInstances(octx.Ctx, client, spec.StackName, summary)
	},
	OperationTree: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runTree(octx.Ctx, client, spec.StackName, summary)
	},
	OperationLogs: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		chart, _ := octx.Flags["chart"].(bool)
		follow, _ := octx.Flags["follow"].(bool)
		return runLogs(octx.Ctx, client, spec.StackName, logsOptions{Chart: chart, Follow: follow}, summary)
	},
	OperationWatch: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runWatch(octx.Ctx, client, spec.StackName, summary)
	},
}

// resolveStackSetTargetFromContext resolves the `kind: aws/stackset` provision
// target for stackset create/update, reading provision.targets from the
// component section and the --target flag the same way deliverApply does.
func resolveStackSetTargetFromContext(octx *opContext) (*stackSetConfig, error) {
	provisionSection, _ := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	flagTarget, _ := octx.Flags[targetKey].(string)
	return resolveStackSetTarget(provisionSection, flagTarget)
}

// changesetNameFlag extracts the required --changeset-name flag value.
func changesetNameFlag(flags map[string]any) string {
	name, _ := flags["changeset-name"].(string)
	return name
}

// runOperation dispatches to the requested aws/cloudformation operation.
func runOperation(octx *opContext, operation Operation, spec *stackSpec) (map[string]any, error) {
	summary := map[string]any{"stack_name": spec.StackName}

	if operation == OperationRender {
		summary["template"] = spec.TemplateBody
		return summary, nil
	}
	if operation == OperationFmt {
		return runFmt(spec, octx.Flags, summary)
	}

	if err := requireConfirmation(operation, spec.StackName, octx.Flags); err != nil {
		return summary, err
	}

	region := resolveRegion(octx.Info.ComponentSection)
	awsCfg, err := buildAWSConfig(octx.Ctx, octx.Info, region)
	if err != nil {
		return summary, err
	}
	client := newClient(awsCfg, resolveEndpointURL(octx.Info))

	handler, ok := operationHandlers[operation]
	if !ok {
		return summary, fmt.Errorf("%w: %q", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, operation)
	}
	return handler(octx, client, spec, summary)
}

// runDiff creates a changeset, renders the predicted changes, then deletes the
// changeset — diff/plan is a preview, and unlike changeset create (an explicit,
// named, reusable artifact the user asked to keep), a diff's changeset has no
// reason to outlive the command: leaving it would silently accumulate an AWS
// object (against the account's changeset quota) on every single diff run.
func runDiff(ctx context.Context, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	result, err := createChangeSet(ctx, client, spec)
	if err != nil {
		return summary, err
	}
	summary["changeset_id"] = result.ChangeSetID
	summary["no_op"] = result.NoOp
	summary["changes"] = result.Changes
	renderDiffSummary(spec.StackName, result)

	// Best-effort cleanup: the diff itself already succeeded and was rendered,
	// so a failure to delete the preview changeset shouldn't fail the command
	// — surface it as a warning instead.
	if err := deleteChangeSet(ctx, client, spec.StackName, result.ChangeSetName); err != nil {
		ui.Warning(fmt.Sprintf("failed to clean up preview changeset %q: %v", result.ChangeSetName, err))
	}
	return summary, nil
}

// renderDiffSummary writes the changeset's predicted resource changes to the
// data channel (stdout), one line per resource — the `plan`/`diff` counterpart
// to renderOutputsSummary, without which those verbs produce no visible output
// at all despite successfully creating and describing the changeset.
func renderDiffSummary(stackName string, result *changeSetResult) {
	if result.NoOp {
		_ = data.Writeln(fmt.Sprintf("%s: no changes (changeset would be a no-op)", stackName))
		return
	}

	_ = data.Writeln(fmt.Sprintf("%s: %d resource change(s)", stackName, len(result.Changes)))
	for _, change := range result.Changes {
		rc := change.ResourceChange
		if rc == nil {
			continue
		}
		line := fmt.Sprintf("  %-8s %-28s %s", rc.Action, aws.ToString(rc.ResourceType), aws.ToString(rc.LogicalResourceId))
		if rc.Replacement != "" {
			line += fmt.Sprintf(" (replacement: %s)", rc.Replacement)
		}
		_ = data.Writeln(line)
	}
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

	if err := applyTerminationProtection(octx.Ctx, client, spec); err != nil {
		return summary, err
	}

	outputs, err := describeStackOutputs(octx.Ctx, client, spec.StackName)
	if err != nil {
		return summary, err
	}
	summary["outputs"] = outputs
	if err := renderOutputsSummary(outputs, octx.Flags); err != nil {
		return summary, err
	}
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
		return summary, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationOperationFailed, spec.StackName, status)
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
	if err := renderOutputsSummary(outputs, flags); err != nil {
		return summary, err
	}
	return summary, nil
}

// renderOutputsSummary writes the Outputs to the data channel (stdout) in the
// requested format (default: table), reusing the shared pkg/output formatter —
// the full standard format set (json/yaml/hcl/env/dotenv/bash/csv/tsv/github).
// Returns an error on an invalid --format instead of swallowing it: a bad
// value must fail the command, not silently exit 0 with empty stdout.
func renderOutputsSummary(outputs map[string]any, flags map[string]any) error {
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
		return fmt.Errorf("%w: failed to format outputs: %w", errUtils.ErrInvalidFlag, err)
	}
	_ = data.Write(rendered)
	return nil
}
