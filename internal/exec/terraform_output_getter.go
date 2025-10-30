package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
	) (any, bool, error)
}

// defaultOutputGetter implements TerraformOutputGetter using the real GetTerraformOutput function.
type defaultOutputGetter struct{}

//nolint:revive // argument-limit: matches interface signature
func (d *defaultOutputGetter) GetOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.defaultOutputGetter.GetOutput")()

	return GetTerraformOutput(atmosConfig, stack, component, output, skipCache, authContext)
}

// Global variable that can be overridden in tests.
var outputGetter TerraformOutputGetter = &defaultOutputGetter{}
