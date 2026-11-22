package schema

import "time"

// ComponentExecution is evaluated command metadata returned by a configuration provider.
// The host executes it with the invocation's environment, toolchain, working directory,
// cancellation, and I/O; the provider never runs these commands. Lists keep declaration order.
type ComponentExecution struct {
	ExtraArguments []ComponentExtraArguments
	BeforeHooks    []ComponentHook
	AfterHooks     []ComponentHook
	ErrorHooks     []ComponentHook
	// Errors holds the retry and ignore policies applied to the Terraform subprocesses of a component.
	Errors *ComponentErrors
	// Apply is the metadata the provider evaluated for the apply command while preparing a
	// plan. A saved plan applies it instead of the plan-time metadata, as Terragrunt would
	// evaluate the configuration again for apply. Nil means the plan-time metadata applies.
	Apply *ComponentExecution
}

// ComponentErrors is the provider's error handling policy: ignore policies are checked first, then
// retry policies, each in declaration order.
type ComponentErrors struct {
	Retry  []ComponentRetryPolicy
	Ignore []ComponentIgnorePolicy
}

// ComponentRetryPolicy retries a failed command whose output matches a pattern. A pattern that
// starts with "!" vetoes the policy when it matches.
type ComponentRetryPolicy struct {
	Name     string
	Patterns []string
	// MaxAttempts counts the first attempt.
	MaxAttempts   int
	SleepInterval time.Duration
}

// ComponentIgnorePolicy turns a failed command whose output matches a pattern into a success.
type ComponentIgnorePolicy struct {
	Name     string
	Patterns []string
	Message  string
	// Signals are written as JSON to a file next to the component's module when the policy applies.
	Signals map[string]any
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
	return e == nil || (len(e.ExtraArguments)+len(e.BeforeHooks)+len(e.AfterHooks)+len(e.ErrorHooks) == 0 && e.Errors == nil)
}
