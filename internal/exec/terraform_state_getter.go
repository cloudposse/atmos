package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TerraformStateGetter defines the interface for getting terraform state.
// This interface allows for dependency injection and testing.
type TerraformStateGetter interface {
	// GetState retrieves terraform state for a component.
	// Note: High parameter count matches GetTerraformState signature.
	GetState(
		atmosConfig *schema.AtmosConfiguration,
		yamlFunc string,
		stack string,
		component string,
		output string,
		skipCache bool,
		authContext *schema.AuthContext,
	) (any, error)
}

// defaultStateGetter implements TerraformStateGetter using the real GetTerraformState function.
type defaultStateGetter struct{}

//nolint:revive // argument-limit: matches interface signature
func (d *defaultStateGetter) GetState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.defaultStateGetter.GetState")()

	return GetTerraformState(atmosConfig, yamlFunc, stack, component, output, skipCache, authContext)
}

// Global variable that can be overridden in tests.
var stateGetter TerraformStateGetter = &defaultStateGetter{}
