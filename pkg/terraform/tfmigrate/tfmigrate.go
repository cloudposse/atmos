package tfmigrate

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
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
func AppendPlanVarFile(env []string, varfile string) []string {
	defer perf.Track(nil, "tfmigrate.AppendPlanVarFile")()

	if varfile == "" {
		return env
	}
	arg := "-var-file=" + varfile
	prefix := EnvTfCliArgsPlan + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = entry + " " + arg
			return env
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

	switch mode {
	case ModePlan:
		return ActionPlan, nil
	case ModeApply:
		return ActionApply, nil
	case ModeDynamic:
		switch strings.ReplaceAll(event, "-", ".") {
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

	if len(backend) == 0 {
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
	default:
		return nil
	}

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
