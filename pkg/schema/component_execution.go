package schema

// ComponentExecution is evaluated command metadata returned by a configuration provider.
// The host executes it with the invocation's environment, toolchain, working directory,
// cancellation, and I/O; the provider never runs these commands. Lists keep declaration order.
type ComponentExecution struct {
	ExtraArguments []ComponentExtraArguments
	BeforeHooks    []ComponentHook
	AfterHooks     []ComponentHook
	ErrorHooks     []ComponentHook
	// Apply is the metadata the provider evaluated for the apply command while preparing a
	// plan. A saved plan applies it instead of the plan-time metadata, as Terragrunt would
	// evaluate the configuration again for apply. Nil means the plan-time metadata applies.
	Apply *ComponentExecution
}

// ComponentExtraArguments adds arguments, variable files, and environment to the listed commands.
type ComponentExtraArguments struct {
	Commands         []string
	Arguments        []string
	RequiredVarFiles []string
	OptionalVarFiles []string
	Environment      map[string]string
}

// ComponentHook is one command the host runs around a Terraform command.
type ComponentHook struct {
	Name     string
	Commands []string
	// Execute is an argv; the first element is resolved on the invocation's PATH.
	Execute []string
	// WorkingDirectory is empty for the provisioned module directory. Relative paths resolve there.
	WorkingDirectory string
	RunOnError       bool
	SuppressStdout   bool
	// OnErrors holds the regular expressions an error hook matches against preceding failures.
	OnErrors []string
}

// ForSavedPlanApply returns the metadata to run when applying a saved plan prepared with e.
// It is the apply-time evaluation when the provider recorded one, and e otherwise.
func (e *ComponentExecution) ForSavedPlanApply() *ComponentExecution {
	if e == nil || e.Apply == nil {
		return e
	}
	return e.Apply
}

// IsEmpty reports whether the execution adds nothing to a command.
func (e *ComponentExecution) IsEmpty() bool {
	return e == nil || len(e.ExtraArguments)+len(e.BeforeHooks)+len(e.AfterHooks)+len(e.ErrorHooks) == 0
}
