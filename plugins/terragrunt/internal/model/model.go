// Package model contains evaluator data, independent of the plugin transport.
package model

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
	Source             string            `json:"source"`
	BackendGenerated   bool              `json:"backend_generated,omitempty"`
	BackendType        string            `json:"backend_type"`
	Backend            map[string]any    `json:"backend"`
	Generated          map[string]string `json:"generated"`
	GeneratedPolicies  map[string]string `json:"generated_policies,omitempty"`
	Units              map[string]string `json:"units,omitempty"`
	Stacks             map[string]string `json:"stacks,omitempty"`
	// Failures are units that could not be evaluated. A failing unit never hides healthy ones.
	Failures []UnitFailure `json:"failures,omitempty"`
	// Warnings are non-fatal facts about the scan that the user should see.
	Warnings []Warning `json:"warnings,omitempty"`
}

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
