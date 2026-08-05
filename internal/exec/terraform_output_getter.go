package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/auth"
	log "github.com/cloudposse/atmos/pkg/logger"
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

	// Resolve the target component's own auth section (when it declares a default identity)
	// before fetching its outputs, so `!terraform.output` matches `!terraform.state` and
	// atmos.Component() instead of always reusing the enclosing component's credentials verbatim.
	resolvedAuthContext, resolvedAuthManager := resolveNestedOutputAuth(
		atmosConfig, component, stack, authContext, authManager, resolveAuthManagerForNestedComponent,
	)
	return tfoutput.GetOutput(atmosConfig, stack, component, output, skipCache, resolvedAuthContext, resolvedAuthManager)
}

// resolveNestedOutputAuth resolves the auth used to fetch a nested component's terraform outputs,
// mirroring GetTerraformState: the target component's own auth section (when it declares a default
// identity) overrides the enclosing component's propagated auth via resolveAuthManagerForNestedComponent,
// and the resolved manager's AuthContext drives the `terraform output` subprocess environment. The
// enclosing auth is kept unchanged when the target has no default identity of its own, when the
// resolver fails, or when the enclosing component has auth disabled. A non-AuthManager authManager
// value passes through untouched so the pkg/terraform/output layer surfaces ErrInvalidAuthManagerType.
//
//nolint:revive // argument-limit: mirrors the GetTerraformState auth plumbing; the resolver is injected for tests
func resolveNestedOutputAuth(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	authContext *schema.AuthContext,
	authManager any,
	resolve componentFuncAuthResolver,
) (*schema.AuthContext, any) {
	defer perf.Track(atmosConfig, "exec.resolveNestedOutputAuth")()

	var parentAuthMgr auth.AuthManager
	if authManager != nil {
		var ok bool
		parentAuthMgr, ok = authManager.(auth.AuthManager)
		if !ok {
			return authContext, authManager
		}
	}

	if parentAuthMgr != nil {
		if si := parentAuthMgr.GetStackInfo(); si != nil && si.AuthDisabled {
			return authContext, authManager
		}
	}

	resolvedAuthMgr, err := resolve(atmosConfig, component, stack, parentAuthMgr)
	if err != nil {
		log.Debug(
			"Auth does not exist for nested component, using the enclosing component's auth",
			logKeyComponent, component,
			logKeyStack, stack,
			"error", err,
		)
		return authContext, authManager
	}
	if resolvedAuthMgr == nil {
		return authContext, authManager
	}

	resolvedAuthContext := authContext
	if si := resolvedAuthMgr.GetStackInfo(); si != nil && si.AuthContext != nil {
		resolvedAuthContext = si.AuthContext
	}
	return resolvedAuthContext, resolvedAuthMgr
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
	authContext *schema.AuthContext,
	authManager any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.GetAllTerraformOutputs")()

	return tfoutput.GetComponentOutputs(atmosConfig, component, stack, skipInit, authContext, authManager)
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
