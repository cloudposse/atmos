package schema

// ComponentExecution is evaluated command metadata returned by a configuration provider.
// The host executes it with the invocation's environment, toolchain, working directory,
// cancellation, and I/O; the provider never runs these commands. Lists keep declaration order.
type ComponentExecution struct {
	ExtraArguments []ComponentExtraArguments
	BeforeHooks    []ComponentHook
	AfterHooks     []ComponentHook
	ErrorHooks     []ComponentHook
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

// IsEmpty reports whether the execution adds nothing to a command.
func (e *ComponentExecution) IsEmpty() bool {
	return e == nil || len(e.ExtraArguments)+len(e.BeforeHooks)+len(e.AfterHooks)+len(e.ErrorHooks) == 0
}
