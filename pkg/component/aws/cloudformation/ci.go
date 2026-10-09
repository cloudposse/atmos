package cloudformation

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// runCIHooks is a seam for testing — swapped in tests to capture RunCIHooksOptions
// without needing a real CI provider.
var runCIHooks = hooks.RunCIHooks

// ciHookParams bundles runCIHook's arguments, mirroring helm's
// helmCIHookParams — keeps the function under the linter's argument-count
// limit and keeps call sites self-documenting via field names.
type ciHookParams struct {
	event       hooks.HookEvent
	flags       map[string]any
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	summary     map[string]any
	commandErr  error
}

// invokedVerbFlag is the key under which the CLI records a top-level alias verb
// (`plan`, `deploy`) that dispatches under a shared operation identifier.
const invokedVerbFlag = "invoked-verb"

// runCIHook builds the compact CI result for one CloudFormation operation and
// fires the Native CI plugin dispatch (hooks.RunCIHooks). Errors here are
// logged, not returned — a CI summary failure must never fail the underlying
// CloudFormation operation, matching Kubernetes's runKubernetesCIHook.
func runCIHook(p ciHookParams) {
	if p.event == "" {
		return
	}
	result := &schema.CloudFormationCIResult{
		ExitCode: errUtils.GetExitCode(p.commandErr),
	}
	if p.info != nil {
		result.Stack = p.info.Stack
		result.Component = p.info.ComponentFromArg
		result.Command = p.info.SubCommand
	}
	// Prefer the verb the user ran, so a `plan` summary is not titled `diff`.
	if invoked, ok := p.flags[invokedVerbFlag].(string); ok && invoked != "" {
		result.Command = invoked
	}
	if p.commandErr != nil {
		result.Error = p.commandErr.Error()
	}
	if failOnDrift, ok := p.flags["fail-on-drift"].(bool); ok {
		result.FailOnDrift = failOnDrift
	}
	populateCloudFormationCIResultFromSummary(result, p.summary)
	if err := runCIHooks(&hooks.RunCIHooksOptions{
		Event:        p.event,
		AtmosConfig:  p.atmosConfig,
		Info:         p.info,
		Output:       ciOutputText(p.summary),
		ForceCIMode:  cloudformationCIModeEnabled(p.flags),
		CommandError: p.commandErr,
		ExitCode:     result.ExitCode,
		Aggregate:    result,
	}); err != nil {
		log.Warn("CloudFormation CI summary skipped", "event", p.event, "error", err)
	}
}

// populateCloudFormationCIResultFromSummary copies the operation-specific
// fields the CloudFormation operation handlers (runDiff/runApply/runDelete/
// runDriftDetect/runDriftDescribe) put into their summary map into the
// compact CI result. Fields left unpopulated by a given operation's summary
// stay zero-valued rather than inventing new summary keys.
func populateCloudFormationCIResultFromSummary(result *schema.CloudFormationCIResult, summary map[string]any) {
	if summary == nil {
		return
	}
	if stackName, ok := summary["stack_name"].(string); ok {
		result.StackName = stackName
	}
	if changeSetName, ok := summary["changeset_name"].(string); ok {
		result.ChangeSetName = changeSetName
	}
	if changes, ok := summary["changes"]; ok {
		result.ResourceChanges = resourceChangeCount(changes)
	}
	if noOp, ok := summary["no_op"].(bool); ok {
		result.NoOp = noOp
		result.HasChanges = !noOp
	}
	if status, ok := summary["final_status"].(string); ok {
		result.StackStatus = status
	}
	if outputs, ok := summary["outputs"].(map[string]any); ok {
		result.Outputs = stringifyOutputs(outputs)
	}
	if driftStatus, ok := summary["drift_status"].(string); ok {
		result.DriftStatus = driftStatus
	}
	if driftedCount, ok := summary["drifted_resource_count"].(int32); ok {
		result.DriftedCount = int(driftedCount)
	}
}

// resourceChangeCount counts the resource changes in a summary's "changes" value. Typed
// changeset results count only entries that carry a ResourceChange (non-resource changes are
// not resource changes); any other slice falls back to its length.
func resourceChangeCount(changes any) int {
	typed, ok := changes.([]cfntypes.Change)
	if !ok {
		return summaryLen(changes)
	}
	count := 0
	for i := range typed {
		if typed[i].ResourceChange != nil {
			count++
		}
	}
	return count
}

// stringifyOutputs converts the presented (already NoEcho-masked) stack outputs into strings.
// The values are taken from the same masked map the `output` verb renders, so CI never sees a
// value the terminal would have redacted.
func stringifyOutputs(outputs map[string]any) map[string]string {
	result := make(map[string]string, len(outputs))
	for key, value := range outputs {
		result[key] = fmt.Sprint(value)
	}
	return result
}

// ciOutputText builds the text for the CI summary's "CloudFormation output" section from an
// operation summary: the stack Outputs after apply, the resource changes for a diff, or the
// drifted resources for drift describe. It returns "" when the operation produced none of
// these, which omits the section. The text passes through the global masker, so any registered
// secret value is redacted before it reaches the job summary.
func ciOutputText(summary map[string]any) string {
	var lines []string
	if outputs, ok := summary["outputs"].(map[string]any); ok {
		lines = append(lines, outputLines(outputs)...)
	}
	if changes, ok := summary["changes"].([]cfntypes.Change); ok {
		lines = append(lines, changeLines(changes)...)
	}
	if drifts, ok := summary["drifts"].([]cfntypes.StackResourceDrift); ok {
		lines = append(lines, driftLines(drifts)...)
	}
	if len(lines) == 0 {
		return ""
	}
	text := strings.Join(lines, "\n")
	if ioCtx := iolib.GetContext(); ioCtx != nil {
		text = ioCtx.Masker().Mask(text)
	}
	return text
}

// outputLines renders stack outputs as sorted "Key = Value" lines.
func outputLines(outputs map[string]any) []string {
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s = %v", key, outputs[key]))
	}
	return lines
}

// changeLines renders a changeset's resource changes (action, type, logical ID, replacement).
func changeLines(changes []cfntypes.Change) []string {
	var lines []string
	for i := range changes {
		rc := changes[i].ResourceChange
		if rc == nil {
			continue
		}
		line := fmt.Sprintf("%-8s %-28s %s", rc.Action, aws.ToString(rc.ResourceType), aws.ToString(rc.LogicalResourceId))
		if rc.Replacement != "" {
			line += fmt.Sprintf(" (replacement: %s)", rc.Replacement)
		}
		lines = append(lines, line)
	}
	return lines
}

// summaryLen returns the length of a summary value that is a slice (e.g.
// []cfntypes.Change, []cfntypes.StackResourceDrift), or zero for any other
// (or nil) value — used for count-only summary fields whose concrete slice
// element type this package doesn't otherwise need to know.
func summaryLen(value any) int {
	if value == nil {
		return 0
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice {
		return 0
	}
	return rv.Len()
}

// cloudformationCIModeEnabled reports whether CI mode is forced for the operation:
// either via the --ci flag (threaded through ExecutionContext.Flags) or the
// standard CI environment variables. Mirrors kubernetesCIModeEnabled/helmCIModeEnabled.
func cloudformationCIModeEnabled(flags map[string]any) bool {
	if value, ok := flags["ci"].(bool); ok && value {
		return true
	}
	return ciEnvEnabled("ATMOS_CI") || ciEnvEnabled("CI")
}

// ciEnvEnabled reports whether a CI environment variable is set to a truthy value.
func ciEnvEnabled(key string) bool {
	//nolint:forbidigo // Standard CI env vars (ATMOS_CI/CI), read directly for CI auto-detection.
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value != "" && value != "false" && value != "0" && value != "no"
}
