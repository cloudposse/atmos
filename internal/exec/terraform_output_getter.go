package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// TerraformOutputGetter defines the interface for getting terraform output.
// This interface allows for dependency injection and testing.
type TerraformOutputGetter interface {
	// GetOutput retrieves terraform output for a component.
	// Note: High parameter count matches GetTerraformOutput signature.
	GetOutput(
		atmosConfig *schema.AtmosConfiguration,
		stack string,
		component string,
		output string,
		skipCache bool,
		authContext *schema.AuthContext,
		authManager any,
	) (any, bool, error)
}

// defaultOutputGetter implements TerraformOutputGetter using the new pkg/terraform/output package.
type defaultOutputGetter struct{}

//nolint:revive // argument-limit: matches interface signature
func (d *defaultOutputGetter) GetOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.defaultOutputGetter.GetOutput")()

	return tfoutput.GetOutput(atmosConfig, stack, component, output, skipCache, authContext, authManager)
}

// Global variable that can be overridden in tests.
var outputGetter TerraformOutputGetter = &defaultOutputGetter{}

// GetTerraformOutput is a backward-compatible wrapper that delegates to pkg/terraform/output.
// This function is kept for backward compatibility with existing tests and code.
// New code should use tfoutput.GetOutput directly.
//
//nolint:revive // argument-limit: matches interface signature for backward compatibility
func GetTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.GetTerraformOutput")()

	return tfoutput.GetOutput(atmosConfig, stack, component, output, skipCache, authContext, authManager)
}

// GetAllTerraformOutputs is a backward-compatible wrapper that delegates to pkg/terraform/output.
// This function is kept for backward compatibility with existing tests and code.
// New code should use tfoutput.GetComponentOutputs directly.
func GetAllTerraformOutputs(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	skipInit bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.GetAllTerraformOutputs")()

	return tfoutput.GetComponentOutputs(atmosConfig, component, stack, skipInit)
}

// GetStaticRemoteStateOutput is a backward-compatible wrapper that delegates to pkg/terraform/output.
// This function is kept for backward compatibility with existing tests and code.
// New code should use tfoutput.GetStaticRemoteStateOutput directly.
func GetStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.GetStaticRemoteStateOutput")()

	return tfoutput.GetStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateSection, output)
}
