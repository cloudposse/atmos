// Package model contains evaluator data, independent of the plugin transport.
package model

import "time"

type Request struct {
	IncludedPaths  []string          `json:"included_paths,omitempty"`
	ExcludedPaths  []string          `json:"excluded_paths,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	GenerationRoot string            `json:"generation_root,omitempty"`
	BasePath       string            `json:"base_path,omitempty"`
	OriginFiles    []string          `json:"origin_files,omitempty"`
	OriginFolders  []string          `json:"origin_folders,omitempty"`
	LogLevel       string            `json:"log_level,omitempty"`
	Path           string            `json:"path"`
	Mode           string            `json:"mode"`
	Command        string            `json:"command"`
	Arguments      []string          `json:"arguments"`
	Tofu           string            `json:"tofu"`
	Environment    map[string]string `json:"environment"`
	// Features are explicit feature flag overrides (name to raw value), applied to every unit.
	Features map[string]string `json:"features,omitempty"`
}

type Resolution struct {
	Inventory          []DiscoveredUnit  `json:"inventory,omitempty"`
	Execution          *Execution        `json:"execution,omitempty"`
	Path               string            `json:"path,omitempty"`
	OutputDependencies []string          `json:"output_dependencies,omitempty"`
	Dependencies       []string          `json:"dependencies,omitempty"`
	Files              []string          `json:"files,omitempty"`
	Folders            []string          `json:"folders,omitempty"`
	WorkingDir         string            `json:"working_dir,omitempty"`
	UnitSources        map[string]string `json:"unit_sources,omitempty"`
	StackSources       map[string]string `json:"stack_sources,omitempty"`
	UnitConfigs        map[string]string `json:"unit_configs,omitempty"`
	StackConfigs       map[string]string `json:"stack_configs,omitempty"`
	Inputs             map[string]any    `json:"inputs"`
	// SensitiveInputs are the paths (object keys from the top) of the inputs the unit declares sensitive.
	SensitiveInputs [][]string `json:"sensitive_inputs,omitempty"`
	Source          string     `json:"source"`
	// RegistryAuth is an invocation-only bearer header for a same-origin archive.
	RegistryAuth            *RegistryAuth     `json:"-"`
	RegistryResolutionNanos uint64            `json:"registry_resolution_nanos,omitempty"`
	BackendGenerated        bool              `json:"backend_generated,omitempty"`
	BackendType             string            `json:"backend_type"`
	Backend                 map[string]any    `json:"backend"`
	Generated               map[string]string `json:"generated"`
	GeneratedPolicies       map[string]string `json:"generated_policies,omitempty"`
	// UnitFileCopies identifies literal files copied from beside the unit.
	UnitFileCopies map[string]bool `json:"unit_file_copies,omitempty"`
	// SensitiveGenerated names generated files whose contents can hold
	// credentials. The host masks their bytes and keeps them out of saved-plan records.
	SensitiveGenerated map[string]bool   `json:"sensitive_generated,omitempty"`
	Units              map[string]string `json:"units,omitempty"`
	Stacks             map[string]string `json:"stacks,omitempty"`
	// Failures are units that could not be evaluated. A failing unit never hides healthy ones.
	Failures []UnitFailure `json:"failures,omitempty"`
	// Warnings are non-fatal facts about the scan that the user should see.
	Warnings []Warning `json:"warnings,omitempty"`
	// TerraformBinary is the unit's terraform_binary: a name for the toolchain or PATH, or an absolute path.
	TerraformBinary string `json:"terraform_binary,omitempty"`
	// Errors holds the retry and ignore policies of the unit's errors block.
	Errors *Errors `json:"errors,omitempty"`
	// Exclude is the evaluated exclude block, set only when the block's condition holds.
	Exclude *Exclude `json:"exclude,omitempty"`
	// FeatureFlags maps each declared feature to its effective value rendered as a string.
	FeatureFlags map[string]string `json:"feature_flags,omitempty"`
	// IAM is the role the host assumes for the unit's commands; nil when the unit names none.
	IAM *IAM `json:"iam,omitempty"`
}

type RegistryAuth struct {
	Origin        string
	Authorization string
}

// IAM is the evaluated iam_role setting and its companions. The plugin never assumes the role: the host
// does, on top of the credentials of the invocation.
type IAM struct {
	Role            string `json:"role"`
	SessionName     string `json:"session_name,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
	// WebIdentityToken is a token, or the path of a file that holds one. It is a secret.
	WebIdentityToken string `json:"-"`
}

// Errors is the evaluated errors block, in declaration order.
type Errors struct {
	Retry  []RetryPolicy  `json:"retry,omitempty"`
	Ignore []IgnorePolicy `json:"ignore,omitempty"`
}

// RetryPolicy retries a failed command whose output matches a pattern. A pattern that starts with
// "!" vetoes the policy when it matches.
type RetryPolicy struct {
	Name             string   `json:"name"`
	Patterns         []string `json:"patterns"`
	MaxAttempts      int      `json:"max_attempts"`
	SleepIntervalSec int      `json:"sleep_interval_sec"`
}

// IgnorePolicy turns a failed command whose output matches a pattern into a success.
type IgnorePolicy struct {
	Name     string         `json:"name"`
	Patterns []string       `json:"patterns"`
	Message  string         `json:"message,omitempty"`
	Signals  map[string]any `json:"signals,omitempty"`
}

// Exclude is an exclude block whose condition holds.
type Exclude struct {
	Actions             []string `json:"actions"`
	ExcludeDependencies bool     `json:"exclude_dependencies,omitempty"`
	// NoRun is nil when the block does not set it, which keeps Terragrunt's exact-action default.
	NoRun *bool `json:"no_run,omitempty"`
}

// Hook lifecycle commands that name Terragrunt's own stages rather than a Terraform command.
const (
	// LifecycleReadConfig runs after the unit's configuration is read (after_hook only).
	LifecycleReadConfig = "terragrunt-read-config"
	// LifecycleReadConfigAlias is the documented spelling of LifecycleReadConfig.
	LifecycleReadConfigAlias = "read-config"
	// LifecycleInitFromModule runs around the source download.
	LifecycleInitFromModule = "init-from-module"
)

// UnitFailure is a unit that discovery could not evaluate. Stack and Name are the Atmos
// coordinates the unit would have had; Err is a Problem or a plain error.
type UnitFailure struct {
	Stack string
	Name  string
	Err   error
}

// Warning is a non-fatal discovery finding. Code is stable and machine readable; Path is
// relative to the project root and uses forward slashes.
type Warning struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Warning codes reported by discovery.
const (
	WarningSymlinkedDirectory = "symlinked_directory_not_followed"
	WarningPatternUnmatched   = "discovery_pattern_unmatched"
	WarningUnitWithoutSource  = "unit_without_terraform_source"
)

// DiscoveredUnit carries source identity independently from its Atmos name.
type DiscoveredUnit struct {
	Stack          string     `json:"stack"`
	Name           string     `json:"name"`
	GenerationRoot string     `json:"generation_root,omitempty"`
	Config         Resolution `json:"config"`
}

// Execution preserves evaluated HCL declaration order for host command dispatch.
// It is data only: the host runs hooks and applies extra arguments to its own commands.
type Execution struct {
	ExtraArguments []ExtraArguments `json:"extra_arguments,omitempty"`
	BeforeHooks    []Hook           `json:"before_hooks,omitempty"`
	AfterHooks     []Hook           `json:"after_hooks,omitempty"`
	ErrorHooks     []Hook           `json:"error_hooks,omitempty"`
	// Apply is the metadata evaluated for the apply command while preparing a plan whose
	// configuration depends on the command. Nil means the plan-time metadata applies.
	Apply *Execution `json:"apply,omitempty"`
}

// ExtraArguments adds arguments, variable files, and environment to the listed commands.
type ExtraArguments struct {
	Commands         []string          `json:"commands"`
	Arguments        []string          `json:"arguments,omitempty"`
	RequiredVarFiles []string          `json:"required_var_files,omitempty"`
	OptionalVarFiles []string          `json:"optional_var_files,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
}

// Hook is one command the host runs around a Terraform command.
type Hook struct {
	Name string `json:"name"`
	// Commands lists the Terraform commands that trigger the hook.
	Commands []string `json:"commands"`
	// Execute is an argv, resolved on the invocation's PATH by the host.
	Execute []string `json:"execute"`
	// WorkingDirectory is empty for the provisioned module directory.
	WorkingDirectory string   `json:"working_directory,omitempty"`
	RunOnError       bool     `json:"run_on_error,omitempty"`
	SuppressStdout   bool     `json:"suppress_stdout,omitempty"`
	OnErrors         []string `json:"on_errors,omitempty"`
}

// LogEntry is one event of the Terragrunt logger, kept for the host to render. The message and fields
// never carry evaluated values by design; the host masks them as well.
type LogEntry struct {
	Time    time.Time
	Fields  map[string]string
	Level   string
	Message string
	// Unit is the unit the event is about, relative to the project root, when the logger reports one.
	Unit string
}

// Problem codes are stable identifiers shared with the host and with tests.
const (
	CodeUnsupportedSetting = "unsupported_setting"
	CodeSyntaxError        = "syntax_error"
	CodeEvaluationError    = "evaluation_failed"
	CodeEffectDenied       = "effect_denied"
	CodeDependency         = "dependency_error"
	CodeInvalidSource      = "invalid_source"
	CodeInvalidGenerate    = "invalid_generate_path"
	CodeMissingSource      = "missing_terraform_source"
	CodeUnsupportedHook    = "unsupported_hook"
	CodeWorkspace          = "workspace_unsupported"
	CodeInternal           = "plugin_error"
)

// Problem is a rejection the user can act on. Summary, Hints, File, Line, Column, and Key never
// contain evaluated values and are always shown. Detail is shown only when Safe is true.
type Problem struct {
	Code    string
	Summary string
	Detail  string
	Hints   []string
	// File is an absolute path; the provider makes it project relative.
	File         string
	Line, Column int
	// Key names the setting, block, or lifecycle that caused the problem.
	Key  string
	Safe bool
	// Err is the underlying cause. It is never shown to the user.
	Err error
}

// Error returns the headline and detail for local diagnostics and tests. The provider never sends it
// to the host: it sends only the structured fields, honoring Safe.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (p *Problem) Error() string {
	if p.Detail == "" {
		return p.Summary
	}
	return p.Summary + ": " + p.Detail
}

// Unwrap exposes the cause for classification.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (p *Problem) Unwrap() error { return p.Err }
