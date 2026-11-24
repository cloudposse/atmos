package terraform

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=../../../internal/exec/mock_terraform_state_getter_test.go -package=exec

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
		authManager any,
		options ...TerraformLookupOptions,
	) (any, error)
}

// defaultStateGetter implements TerraformStateGetter using the real GetTerraformState function.
type defaultStateGetter struct{ executor *Executor }

//nolint:revive // argument-limit: matches interface signature
func (d *defaultStateGetter) GetState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
	options ...TerraformLookupOptions,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.defaultStateGetter.GetState")()

	return d.executor.GetTerraformState(atmosConfig, yamlFunc, stack, component, output, skipCache, authContext, authManager, options...)
}

// StateGetter returns a reference getter bound to this executor.
func (e *Executor) StateGetter() TerraformStateGetter {
	defer perf.Track(nil, "terraform.Executor.StateGetter")()
	return &defaultStateGetter{executor: e}
}
