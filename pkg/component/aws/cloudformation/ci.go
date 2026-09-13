package cloudformation

import (
	"os"
	"reflect"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
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
	if p.commandErr != nil {
		result.Error = p.commandErr.Error()
	}
	populateCloudFormationCIResultFromSummary(result, p.summary)
	if err := runCIHooks(&hooks.RunCIHooksOptions{
		Event:        p.event,
		AtmosConfig:  p.atmosConfig,
		Info:         p.info,
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
		result.ResourceChanges = summaryLen(changes)
	}
	if driftStatus, ok := summary["drift_status"].(string); ok {
		result.DriftStatus = driftStatus
	}
	if driftedCount, ok := summary["drifted_resource_count"].(int32); ok {
		result.DriftedCount = int(driftedCount)
	}
	if drifts, ok := summary["drifts"]; ok && result.DriftedCount == 0 {
		result.DriftedCount = summaryLen(drifts)
	}
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
