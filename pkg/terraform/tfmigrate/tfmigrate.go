package tfmigrate

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	ActionPlan  = "plan"
	ActionApply = "apply"

	ModeDynamic = "dynamic"
	ModePlan    = ActionPlan
	ModeApply   = ActionApply

	Command        = "tfmigrate"
	ExecPathEnvVar = "TFMIGRATE_EXEC_PATH"

	// EnvTfCliArgsPlan is honored by terraform/tofu for every `plan` they run.
	// Tfmigrate verifies migrations by invoking `terraform plan` itself, without
	// the -var-file argument Atmos normally passes, so the generated varfile is
	// routed through this variable instead.
	EnvTfCliArgsPlan = "TF_CLI_ARGS_plan"

	EnvStack            = "ATMOS_STACK"
	EnvComponent        = "ATMOS_COMPONENT"
	EnvWorkspace        = "ATMOS_TERRAFORM_WORKSPACE"
	EnvHistoryNamespace = "ATMOS_TFMIGRATE_HISTORY_NAMESPACE"
	EnvHistoryKey       = "ATMOS_TFMIGRATE_HISTORY_KEY"
	EnvHistoryPath      = "ATMOS_TFMIGRATE_HISTORY_PATH"
	EnvHistoryStorage   = "ATMOS_TFMIGRATE_HISTORY_STORAGE"
	EnvHistoryBucket    = "ATMOS_TFMIGRATE_HISTORY_BUCKET"
	EnvHistoryRegion    = "ATMOS_TFMIGRATE_HISTORY_REGION"
	EnvHistoryProfile   = "ATMOS_TFMIGRATE_HISTORY_PROFILE"
	EnvHistoryRoleARN   = "ATMOS_TFMIGRATE_HISTORY_ROLE_ARN"
	EnvHistoryEndpoint  = "ATMOS_TFMIGRATE_HISTORY_ENDPOINT"
	EnvHistoryKMSKeyID  = "ATMOS_TFMIGRATE_HISTORY_KMS_KEY_ID"

	defaultHistoryPrefix = "tfmigrate"
	defaultHistoryFile   = "history.json"
	defaultWorkspace     = "default"
	envAssignmentFormat  = "%s=%s"
)

// Options controls a tfmigrate invocation.
type Options struct {
	Action        string
	Migration     string
	Config        string
	BackendConfig []string
}

// HistoryValues contains stable tfmigrate history identifiers for an Atmos component instance.
type HistoryValues struct {
	Stack     string
	Component string
	Workspace string
	Namespace string
	Key       string
}

// Validate checks whether the requested tfmigrate action is supported.
func (o Options) Validate() error {
	defer perf.Track(nil, "tfmigrate.Options.Validate")()

	switch o.Action {
	case ActionPlan, ActionApply:
		return nil
	default:
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Invalid tfmigrate action").
			WithContext("action", o.Action).
			WithHint("Use plan or apply").
			Err()
	}
}

// BuildArgs returns argv for the tfmigrate process.
func BuildArgs(opts Options) ([]string, error) {
	defer perf.Track(nil, "tfmigrate.BuildArgs")()

	if err := opts.Validate(); err != nil {
		return nil, err
	}

	args := []string{opts.Action}
	if opts.Config != "" {
		args = append(args, "--config", opts.Config)
	}
	for _, backendConfig := range opts.BackendConfig {
		if backendConfig == "" {
			continue
		}
		args = append(args, "--backend-config="+backendConfig)
	}
	if opts.Migration != "" {
		args = append(args, opts.Migration)
	}
	return args, nil
}

// AppendPlanVarFile routes the Atmos-generated varfile to the terraform plan
// runs tfmigrate performs internally, via TF_CLI_ARGS_plan. Without it, any
// component with required variables fails tfmigrate's convergence plan with
// "No value for required variable". Existing TF_CLI_ARGS_plan values (from the
// component env or the process environment) are preserved and extended.
//
// Varfile must be an absolute path: TF_CLI_ARGS_plan is a single, global env
// var applied to every `terraform plan` tfmigrate runs internally, and for
// `migration "multi_state"` that includes a convergence-check plan in a
// *second* directory (from_dir) as well as the triggering component's own
// directory (to_dir). An absolute path resolves correctly regardless of which
// of the two tfmigrate's cwd is. This does not make multi_state fully correct
// when from_dir/to_dir have materially different variables (the same varfile
// is applied to both plans, since tfmigrate exposes no way to give the two a
// different -var-file) - callers with that setup should still set
// from_skip_plan/to_skip_plan on the migration block.
func AppendPlanVarFile(env []string, varfile string) []string {
	defer perf.Track(nil, "tfmigrate.AppendPlanVarFile")()

	if varfile == "" {
		return env
	}
	arg := "-var-file=" + varfile
	prefix := EnvTfCliArgsPlan + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			// Copy before extending the entry so the caller's slice (and anything
			// sharing its backing array) is left untouched, matching the
			// append-based branches below.
			updated := slices.Clone(env)
			updated[i] = entry + " " + arg
			return updated
		}
	}
	if existing, ok := os.LookupEnv(EnvTfCliArgsPlan); ok && existing != "" {
		return append(env, prefix+existing+" "+arg)
	}
	return append(env, prefix+arg)
}

// EnsureResolved verifies that the resolved tfmigrate command points at an
// installed binary. Toolchain resolution returns the bare command name
// unchanged when the tool is neither toolchain-managed nor on PATH, which
// would otherwise surface as a raw exec failure mid-run.
func EnsureResolved(resolved string) error {
	defer perf.Track(nil, "tfmigrate.EnsureResolved")()

	if resolved != Command {
		return nil
	}
	if _, err := exec.LookPath(Command); err == nil {
		return nil
	}
	return errUtils.Build(errUtils.ErrCommandNotFound).
		WithExplanationf("`atmos terraform migrate` requires %q, which is not installed and not on PATH", Command).
		WithHintf("Declare `%s: \"<version>\"` in the component's dependencies.tools to auto-install it", Command).
		WithHintf("Alternatively, install %s manually so it appears on PATH", Command).
		WithContext("command", Command).
		Err()
}

// ActionForMode resolves a hook mode and lifecycle event to a tfmigrate action.
func ActionForMode(mode, event string) (string, error) {
	defer perf.Track(nil, "tfmigrate.ActionForMode")()

	if mode == "" {
		mode = ModeDynamic
	}

	normalizedEvent := strings.ReplaceAll(event, "-", ".")

	switch mode {
	case ModePlan:
		if isApplyLikeEvent(normalizedEvent) {
			// Legitimate (if unusual) - preview-only forever, even on real
			// applies - but easy to hit by mistake (e.g. mode: plan copied
			// onto a before.terraform.apply hook), so warn loudly: the
			// migration will never actually be applied by this hook.
			log.Warn(
				"tfmigrate hook mode is \"plan\" on an apply/deploy event; the migration will only be previewed, never applied",
				"event", event,
			)
		}
		return ActionPlan, nil
	case ModeApply:
		if !isApplyLikeEvent(normalizedEvent) {
			// Unlike mode: plan on an apply event, this direction has no safe
			// reading: it silently mutates real state on what looks like a
			// read-only operation (plan, or --dry-run), so it must not be
			// allowed to pass silently.
			return "", errUtils.Build(errUtils.ErrInvalidConfig).
				WithExplanation("tfmigrate hook mode \"apply\" is bound to an event that is not before.terraform.apply or before.terraform.deploy").
				WithContext("mode", mode).
				WithContext("event", event).
				WithHint("Use mode: dynamic (the default) so the action always matches the triggering event, or bind this hook to before.terraform.apply/before.terraform.deploy").
				Err()
		}
		return ActionApply, nil
	case ModeDynamic:
		switch normalizedEvent {
		case "before.terraform.plan":
			return ActionPlan, nil
		case "before.terraform.apply", "before.terraform.deploy":
			return ActionApply, nil
		default:
			return "", errUtils.Build(errUtils.ErrInvalidConfig).
				WithExplanation("tfmigrate dynamic mode only supports before.terraform.plan, before.terraform.apply, and before.terraform.deploy").
				WithContext("event", event).
				Err()
		}
	default:
		return "", errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Invalid tfmigrate hook mode").
			WithContext("mode", mode).
			WithHint("Use dynamic, plan, or apply").
			Err()
	}
}

// isApplyLikeEvent reports whether a normalized lifecycle event triggers a
// real terraform apply (as opposed to a read-only plan/preview).
func isApplyLikeEvent(normalizedEvent string) bool {
	return normalizedEvent == "before.terraform.apply" || normalizedEvent == "before.terraform.deploy"
}

// AppendExecPath adds TFMIGRATE_EXEC_PATH unless it is already configured.
func AppendExecPath(env []string, terraformCommand string) []string {
	defer perf.Track(nil, "tfmigrate.AppendExecPath")()

	if terraformCommand == "" || hasEnvKey(env, ExecPathEnvVar) {
		return env
	}
	if _, ok := os.LookupEnv(ExecPathEnvVar); ok {
		return env
	}
	return append(env, fmt.Sprintf("%s=%s", ExecPathEnvVar, terraformCommand))
}

// HistoryEnv returns stable per-instance variables for tfmigrate history config.
// Tfmigrate HCL can reference these through the `env` object, for example:
// key = "${env.ATMOS_TFMIGRATE_HISTORY_KEY}".
func HistoryEnv(stack, component, workspace string) []string {
	defer perf.Track(nil, "tfmigrate.HistoryEnv")()

	values := HistoryNames(stack, component, workspace)

	return []string{
		envAssignment(EnvStack, values.Stack),
		envAssignment(EnvComponent, values.Component),
		envAssignment(EnvWorkspace, values.Workspace),
		envAssignment(EnvHistoryNamespace, values.Namespace),
		envAssignment(EnvHistoryKey, values.Key),
		envAssignment(EnvHistoryPath, values.Key),
	}
}

// HistoryNames returns stable per-instance values for tfmigrate history config.
func HistoryNames(stack, component, workspace string) HistoryValues {
	defer perf.Track(nil, "tfmigrate.HistoryNames")()

	if workspace == "" {
		workspace = defaultWorkspace
	}
	namespace := historyPath(defaultHistoryPrefix, stack, component, workspace)
	key := historyPath(namespace, defaultHistoryFile)
	return HistoryValues{
		Stack:     stack,
		Component: component,
		Workspace: workspace,
		Namespace: namespace,
		Key:       key,
	}
}

// BackendHistoryEnv exposes Terraform backend values that tfmigrate history
// storage can reuse from its config file.
func BackendHistoryEnv(backendType string, backend map[string]any) []string {
	defer perf.Track(nil, "tfmigrate.BackendHistoryEnv")()

	values := BackendHistoryValues(backendType, backend)
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, envAssignment(key, value))
	}
	return env
}

// BackendHistoryValues returns Terraform backend values that tfmigrate history
// storage can reuse from its config file.
func BackendHistoryValues(backendType string, backend map[string]any) map[string]string {
	defer perf.Track(nil, "tfmigrate.BackendHistoryValues")()

	if backendType == "" {
		return nil
	}

	values := map[string]string{EnvHistoryStorage: backendType}
	setBackendValue := func(envKey, backendKey string) {
		if value := backendString(backend, backendKey); value != "" {
			values[envKey] = value
		}
	}

	switch backendType {
	case "s3":
		setBackendValue(EnvHistoryBucket, "bucket")
		setBackendValue(EnvHistoryRegion, "region")
		setBackendValue(EnvHistoryProfile, "profile")
		if roleARN := s3RoleARN(backend); roleARN != "" {
			values[EnvHistoryRoleARN] = roleARN
		}
		if endpoint := s3Endpoint(backend); endpoint != "" {
			values[EnvHistoryEndpoint] = endpoint
		}
		setBackendValue(EnvHistoryKMSKeyID, "kms_key_id")
	case "gcs":
		gcsBackend := backend
		if nested, ok := backend["gcs"].(map[string]any); ok {
			gcsBackend = nested
		}
		if value := backendString(gcsBackend, "bucket"); value != "" {
			values[EnvHistoryBucket] = value
		}
	}
	// Any other backend type (local, azurerm, consul, remote, ...) still
	// reports EnvHistoryStorage = backendType (set above) with no extra
	// fields - only s3/gcs have bucket/region/etc. to add. Falling through
	// here (rather than returning nil) is what makes `migrate list`'s
	// History Storage column show the real backend type instead of a blank
	// cell for every backend that isn't s3/gcs.

	return values
}

func envAssignment(key, value string) string {
	return fmt.Sprintf(envAssignmentFormat, key, value)
}

func historyPath(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, strings.Trim(part, "/"))
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	return strings.Join(nonEmpty, "/")
}

// s3Endpoint reads the S3 endpoint from either the legacy top-level `endpoint`
// argument or the Terraform 1.6+ nested `endpoints { s3 = ... }` form.
func s3Endpoint(backend map[string]any) string {
	if endpoint := backendString(backend, "endpoint"); endpoint != "" {
		return endpoint
	}
	if endpoints, ok := backend["endpoints"].(map[string]any); ok {
		return backendString(endpoints, "s3")
	}
	return ""
}

func s3RoleARN(backend map[string]any) string {
	if assumeRole, ok := backend["assume_role"].(map[string]any); ok {
		if roleARN := backendString(assumeRole, "role_arn"); roleARN != "" {
			return roleARN
		}
	}
	return backendString(backend, "role_arn")
}

func backendString(backend map[string]any, key string) string {
	if value, ok := backend[key].(string); ok {
		return value
	}
	return ""
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
