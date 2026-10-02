package cloudformation

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
	// RequestedIdentity preserves CLI/env precedence before component default selection.
	RequestedIdentity string
	Ctx               context.Context
	AtmosConfig       *schema.AtmosConfiguration
	Info              *schema.ConfigAndStacksInfo
	Flags             map[string]any
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
	// Stack evaluation can read secrets before the CloudFormation client is
	// authenticated. Bind each lookup lazily to its component/store identity;
	// never install credentials on a shared configuration or unused backend.
	localConfig := *atmosConfig
	atmosConfig = &localConfig
	if !info.DryRun && !authdeferred.IsDeferred(atmosConfig.AuthManager) {
		atmosConfig.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{
			Config: atmosConfig, Disabled: cfg.NormalizeIdentityValue(info.Identity) == cfg.IdentityFlagDisabledValue,
		})
	}
	discovered, err := processStacks(atmosConfig, *info, true, !info.DryRun, !info.DryRun, nil, nil)
	if err != nil {
		return err
	}
	*info = discovered
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	// Resolve static config before any authentication, provisioning or hooks.
	if info.DryRun {
		return validateDryRun(atmosConfig, info, ctx.Flags, operation)
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

	spec, err := resolveSpecAndTemplate(ctx.GoContext(), atmosConfig, info, operation)
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
// name/ID (delete, output, explicit changeset execute/list/delete, drift, get)
// and never send a local template to CloudFormation, so resolving the
// component's on-disk path (including JIT source provisioning) and loading the
// template file from disk would be pure overhead. Named execution still loads
// a configured stack policy to protect the current update.
var operationsSkippingTemplateLoad = map[Operation]bool{
	OperationDelete:            true,
	OperationOutput:            true,
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

// operationsSkippingStackPolicyLoad are operations that load a template (so
// they're not in operationsSkippingTemplateLoad) but never consume
// spec.StackPolicyBody: fmt only round-trips the template file, and
// changeset-create's createChangeSet call has no stack-policy parameter
// (CreateChangeSet/ExecuteChangeSet don't support one — see setStackPolicy's
// doc comment). Apply and explicit changeset execution read
// StackPolicyBody, so a missing or unreadable stack_policy file must not
// block either of these.
var operationsSkippingStackPolicyLoad = map[Operation]bool{
	OperationFmt:             true,
	OperationChangesetCreate: true,
}

// resolveSpecAndTemplate builds the SDK-ready stackSpec and — for every
// operation not in operationsSkippingTemplateLoad — resolves the component's
// on-disk path (including JIT source provisioning), loads the template body,
// registers NoEcho values with the masker, and loads the stack policy.
// Operations in operationsSkippingTemplateLoad only need spec fields already
// set by buildStackSpec (e.g. StackName), except named execution also loads its
// configured policy without loading the template.
// Operations in operationsSkippingStackPolicyLoad need the template but never
// consume StackPolicyBody, so they return right after the template load instead of also
// resolving and reading a stack_policy file that a missing/unreadable policy
// would otherwise block them on for no reason.
func resolveSpecAndTemplate(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation) (*stackSpec, error) {
	spec, err := buildStackSpec(info.ComponentSection)
	if err != nil {
		return nil, err
	}
	spec.withAtmosIdentity(info)

	if operationsSkippingTemplateLoad[operation] {
		if operation == OperationChangesetExecute && spec.StackPolicyFile != "" {
			return resolveExecutionPolicy(ctx, atmosConfig, info, spec)
		}
		return spec, nil
	}

	componentPath, err := prepareComponentFiles(ctx, atmosConfig, info)
	if err != nil {
		return nil, err
	}

	if err := resolveTemplateBody(componentPath, spec); err != nil {
		return nil, err
	}

	registerNoEchoValues(spec.TemplateBody, spec)

	if operationsSkippingStackPolicyLoad[operation] {
		return spec, nil
	}

	spec.StackPolicyBody, err = loadStackPolicyBody(componentPath, spec)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// runWithHooks runs the before/after hooks around the operation. The Native CI
// summary hook (runCIHook) fires on both success and failure — mirroring
// Kubernetes's runWithHooks — since a job summary describing a failed
// operation is exactly as useful as one describing a successful one.
func runWithHooks(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation, spec *stackSpec) error {
	hookSet, err := getHooks(atmosConfig, info)
	if err != nil {
		return err
	}
	before, after := eventsFor(operation)
	if err := hookSet.RunAll(before, atmosConfig, info, nil, nil); err != nil {
		return err
	}

	octx := &opContext{Ctx: ctx.GoContext(), AtmosConfig: atmosConfig, Info: info, Flags: ctx.Flags, RequestedIdentity: ctx.ConfigAndStacksInfo.Identity}
	summary, opErr := runOperation(octx, operation, spec)

	// The outcome status describes the CloudFormation operation, not the
	// after-hooks: a failed apply must let `when: failure`/`always` hooks run
	// and see the right status, mirroring runUserHooks in cmd/terraform/utils.go.
	outcome := hooks.Outcome{Status: hooks.RunSuccess}
	if opErr != nil {
		outcome = hooks.Outcome{Status: hooks.RunFailure, Err: opErr, ExitCode: errUtils.GetExitCode(opErr)}
	}
	hookSet.SetOutcome(outcome)

	// User-defined `hooks:` blocks run first, then the CI summary dispatch —
	// matching the ordering Kubernetes's executor uses between its own
	// user-hook and CI-hook calls. The CI hook is fire-and-forget (errors are
	// logged, not returned; see runCIHook), so it never masks opErr or the
	// after-hooks error below.
	afterErr := hookSet.RunAll(after, atmosConfig, info, nil, nil)
	// opErr takes precedence (it's the more actionable failure, matching the
	// return below), but when the operation succeeded and only the after-hook
	// failed, the CI summary must still reflect that failure rather than
	// silently reporting success.
	ciErr := opErr
	if ciErr == nil {
		ciErr = afterErr
	}
	runCIHook(ciHookParams{
		event:       after,
		flags:       ctx.Flags,
		atmosConfig: atmosConfig,
		info:        info,
		summary:     summary,
		commandErr:  ciErr,
	})

	// The underlying operation's own error takes priority: a failed apply is
	// the more actionable/severe failure than a problem in a user-defined
	// after-hook, and callers (exit code, CI status) should see it first.
	if opErr != nil {
		return opErr
	}
	return afterErr
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
	case OperationDriftDetect:
		return hooks.BeforeAwsCloudFormationDriftDetect, hooks.AfterAwsCloudFormationDriftDetect
	case OperationDriftDescribe:
		return hooks.BeforeAwsCloudFormationDriftDescribe, hooks.AfterAwsCloudFormationDriftDescribe
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
	OperationValidate: runValidate,
	OperationDiff:     runDiff,
	OperationApply:    runApply,
	OperationDelete: func(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
		return runDelete(octx.Ctx, client, octx.Flags, spec, summary)
	},
	OperationOutput:          runOutputOperation,
	OperationChangesetCreate: runChangesetCreate,
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
	ssCfg, err := resolveStackSetTarget(provisionSection, flagTarget)
	if err != nil {
		return nil, err
	}
	// A permission_model typo fails here, locally, instead of at the AWS API. This
	// is the real-run path only: dry-run resolves the target without rendering
	// templates, so it validates a deferred permission_model itself.
	if err := validateStackSetPermissionModel(ssCfg.Name, ssCfg.PermissionModel); err != nil {
		return nil, err
	}
	return ssCfg, nil
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
		return summary, data.Write(spec.TemplateBody)
	}

	// Even diff/validate can write remote changesets or packaged templates.
	// Stop before confirmation and client creation for every dry-run operation.
	if octx.Info.DryRun {
		return summary, nil
	}

	if operation == OperationFmt {
		return runFmt(spec, octx.Flags, summary)
	}

	// apply asks only after its changeset is created and previewed (see
	// deployDirect), and only when it deploys a stack: publish-only and external
	// deliveries change no stack and never ask.
	if operation != OperationApply {
		if err := requireConfirmation(operation, spec.StackName, octx.Flags); err != nil {
			return summary, err
		}
	}

	client, err := clientForOperation(octx, operation)
	if err != nil {
		return summary, err
	}

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
func runDiff(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	if err := prepareTemplateForAPI(octx, spec, summary); err != nil {
		return summary, err
	}
	result, err := createChangeSet(octx.Ctx, client, spec)
	if err != nil {
		discardChangeSet(octx.Ctx, client, spec.StackName, result)
		return summary, err
	}
	summary["changeset_id"] = result.ChangeSetID
	summary["changeset_name"] = result.ChangeSetName
	summary["no_op"] = result.NoOp
	summary["changes"] = result.Changes
	renderErr := renderDiffSummary(spec.StackName, result)

	// Clean up the preview even if its output could not be written: the changeset
	// and, for a stack that did not exist before this diff, the empty
	// REVIEW_IN_PROGRESS stub the CREATE changeset registered. A cleanup failure is
	// only a warning; any rendering failure stays the command error.
	discardChangeSet(octx.Ctx, client, spec.StackName, result)
	return summary, renderErr
}

// renderDiffSummary writes the changeset's predicted resource changes to the
// data channel (stdout), one line per resource — the `plan`/`diff` counterpart
// to renderOutputsSummary, without which those verbs produce no visible output
// at all despite successfully creating and describing the changeset.
func renderDiffSummary(stackName string, result *changeSetResult) error {
	if err := data.Writeln(diffSummaryText(stackName, result)); err != nil {
		return fmt.Errorf("write CloudFormation diff summary: %w", err)
	}
	return nil
}

// diffSummaryText builds the text renderDiffSummary writes: a header line with
// the number of resource changes, then one line per changed resource (or the
// no-op line when the changeset would change nothing).
func diffSummaryText(stackName string, result *changeSetResult) string {
	lines := []string{fmt.Sprintf("%s: no changes (changeset would be a no-op)", stackName)}
	if !result.NoOp {
		lines = diffResourceLines(result.Changes)
		lines = append([]string{fmt.Sprintf("%s: %d resource change(s)", stackName, len(lines))}, lines...)
	}
	return strings.Join(lines, "\n")
}

// diffResourceLines renders resource changes, excluding non-resource changes.
func diffResourceLines(changes []cfntypes.Change) []string {
	var lines []string
	for _, change := range changes {
		rc := change.ResourceChange
		if rc == nil {
			continue
		}
		line := fmt.Sprintf("  %-8s %-28s %s", rc.Action, aws.ToString(rc.ResourceType), aws.ToString(rc.LogicalResourceId))
		if rc.Replacement != "" {
			line += fmt.Sprintf(" (replacement: %s)", rc.Replacement)
		}
		lines = append(lines, line)
	}
	return lines
}

// runApply executes the changeset (creating or updating the stack) and renders
// the end-of-deploy Outputs summary — the same view the standalone `output` verb
// renders, per the PRD's direct response to the #1 Rain-user complaint.
//
// The stack-policy/termination-protection/outputs follow-up steps below only
// run when deliverApply actually performed a direct stack deploy, signaled by
// a non-nil result. The other two outcomes of deliverApply — a `kind: aws/s3`
// target selected directly (publish-only: template uploaded, no stack
// touched) and any other kind (e.g. `kind: git`, delivered via the generic
// target registry) — never create or touch a CloudFormation stack, so there
// is nothing for these stack-scoped calls to act on; running them anyway
// previously crashed with a raw "Stack does not exist" AWS error that had
// nothing to do with what the user actually asked for. A non-nil result is a
// reliable signal for this: it is populated exclusively by deployDirect's
// changeset flow (see waitForChangeSet), which always returns a non-nil
// result on every success path (including the no-op case), and any error
// from that path is already handled by the err != nil check above.
func runApply(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	deploySummary, result, err := deliverApply(octx, client, spec)
	for k, v := range deploySummary {
		summary[k] = v
	}
	// Preserve the reviewed changeset even when confirmation, execution or event
	// polling failed. CI reports the planned changes alongside the operation error.
	if result != nil {
		summary["changeset_id"] = result.ChangeSetID
		summary["changeset_name"] = result.ChangeSetName
		summary["changes"] = result.Changes
		// A failed computation does not establish whether the stack has changes.
		if result.Status == cfntypes.ChangeSetStatusCreateComplete || result.NoOp {
			summary["no_op"] = result.NoOp
		}
		if result.StackStatus != "" {
			summary["final_status"] = string(result.StackStatus)
		}
	}
	if err != nil {
		return summary, err
	}
	if result == nil {
		// Publish-only (aws/s3) or external-target (e.g. git) delivery: no
		// direct stack deploy happened.
		return summary, nil
	}
	if spec.StackPolicyBody != "" && (result.NoOp || result.ChangeSetType == cfntypes.ChangeSetTypeCreate) {
		if err := setStackPolicy(octx.Ctx, client, spec); err != nil {
			return summary, err
		}
	}

	if err := applyTerminationProtection(octx.Ctx, client, spec); err != nil {
		return summary, err
	}

	return runOutput(octx.Ctx, client, spec.StackName, octx.Flags, summary)
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
