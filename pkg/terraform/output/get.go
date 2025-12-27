package output

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultExecutor is the package-level executor used by GetComponentOutputs.
// It must be set via SetDefaultExecutor before GetComponentOutputs is called.
var defaultExecutor *Executor

// SetDefaultExecutor sets the default executor used by package-level functions.
// This must be called from internal/exec during initialization to break the circular dependency.
func SetDefaultExecutor(exec *Executor) {
	defer perf.Track(nil, "output.SetDefaultExecutor")()

	defaultExecutor = exec
}

// GetDefaultExecutor returns the default executor, or nil if not set.
func GetDefaultExecutor() *Executor {
	defer perf.Track(nil, "output.GetDefaultExecutor")()

	return defaultExecutor
}

// GetComponentOutputs retrieves all terraform outputs for a component in a stack.
// This is used by the --format flag to get all outputs at once for formatting.
//
// Parameters:
//   - atmosConfig: Atmos configuration pointer
//   - component: Component identifier
//   - stack: Stack identifier
//   - skipInit: If true, skip terraform init (assumes already initialized)
//
// Returns:
//   - outputs: Map of all terraform output values
//   - error: Any error that occurred during retrieval
func GetComponentOutputs(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	skipInit bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.GetComponentOutputs")()

	if defaultExecutor == nil {
		panic("output.SetDefaultExecutor must be called before GetComponentOutputs")
	}

	return defaultExecutor.GetAllOutputs(atmosConfig, component, stack, skipInit)
}

// GetComponentOutputsWithExecutor retrieves all terraform outputs using a specific executor.
// Use this when you need to provide a custom executor instance.
func GetComponentOutputsWithExecutor(
	exec *Executor,
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	skipInit bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.GetComponentOutputsWithExecutor")()

	return exec.GetAllOutputs(atmosConfig, component, stack, skipInit)
}

// ExecuteWithSections retrieves terraform outputs using pre-loaded sections.
// This is used when the caller already has sections from describing a component
// and wants to execute terraform output directly without re-describing.
func ExecuteWithSections(
	atmosConfig *schema.AtmosConfiguration,
	component, stack string,
	sections map[string]any,
	authContext *schema.AuthContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.ExecuteWithSections")()

	if defaultExecutor == nil {
		panic("output.SetDefaultExecutor must be called before ExecuteWithSections")
	}

	return defaultExecutor.ExecuteWithSections(atmosConfig, component, stack, sections, authContext)
}

// GetOutput retrieves a specific terraform output for a component in a stack.
// This is the main entry point for getting individual outputs, used by YAML functions
// like !terraform.output.
//
// Parameters:
//   - atmosConfig: Atmos configuration pointer
//   - stack: Stack identifier
//   - component: Component identifier
//   - output: Output variable key to retrieve
//   - skipCache: Flag to bypass cache lookup
//   - authContext: Authentication context for credential access (may be nil)
//   - authManager: Optional auth manager for nested operations that need authentication
//
// Returns:
//   - value: The output value (may be nil if the output exists but has a null value)
//   - exists: Whether the output key exists in the terraform outputs
//   - error: Any error that occurred during retrieval
//
//nolint:revive // argument-limit: matches GetTerraformOutput signature for backward compatibility.
func GetOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.GetOutput")()

	if defaultExecutor == nil {
		panic("output.SetDefaultExecutor must be called before GetOutput")
	}

	return defaultExecutor.GetOutput(atmosConfig, stack, component, output, skipCache, authContext, authManager)
}
