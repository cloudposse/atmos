package exec

import (
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// componentDescriberAdapter implements tfoutput.ComponentDescriber by wrapping ExecuteDescribeComponent.
type componentDescriberAdapter struct{}

// DescribeComponent implements tfoutput.ComponentDescriber.
func (c *componentDescriberAdapter) DescribeComponent(params *tfoutput.DescribeComponentParams) (map[string]any, error) {
	defer perf.Track(nil, "exec.componentDescriberAdapter.DescribeComponent")()

	// Convert AuthManager from any to auth.AuthManager if provided.
	var authMgr auth.AuthManager
	if params.AuthManager != nil {
		if am, ok := params.AuthManager.(auth.AuthManager); ok {
			authMgr = am
		}
	}

	return ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            params.Component,
		Stack:                params.Stack,
		ProcessTemplates:     params.ProcessTemplates,
		ProcessYamlFunctions: params.ProcessYamlFunctions,
		Skip:                 params.Skip,
		AuthManager:          authMgr,
	})
}

// staticRemoteStateGetterAdapter implements tfoutput.StaticRemoteStateGetter.
type staticRemoteStateGetterAdapter struct{}

// GetStaticRemoteStateOutputs implements tfoutput.StaticRemoteStateGetter.
func (s *staticRemoteStateGetterAdapter) GetStaticRemoteStateOutputs(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "exec.staticRemoteStateGetterAdapter.GetStaticRemoteStateOutputs")()

	return GetComponentRemoteStateBackendStaticType(sections)
}

// InitTerraformOutputExecutor initializes the default terraform output executor.
// This must be called during application startup to set up the dependency injection.
func InitTerraformOutputExecutor() {
	defer perf.Track(nil, "exec.InitTerraformOutputExecutor")()

	describer := &componentDescriberAdapter{}
	staticGetter := &staticRemoteStateGetterAdapter{}

	executor := tfoutput.NewExecutor(
		describer,
		tfoutput.WithStaticRemoteStateGetter(staticGetter),
	)

	tfoutput.SetDefaultExecutor(executor)
}

func init() {
	// Initialize the terraform output executor when this package is imported.
	InitTerraformOutputExecutor()
}
