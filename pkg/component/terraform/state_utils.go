package terraform

import (
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	componentpkg "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	"github.com/cloudposse/atmos/pkg/terraform/refcache"
)

var terraformStateCache = sync.Map{}

type terraformStateNotProvisionedCacheEntry struct{}

// Static values preserve absent-output errors on warm reads as well as cold reads.
type terraformStateStaticCacheEntry struct {
	outputs map[string]any
}

type TerraformStateLookup struct {
	YamlFunc  string
	Stack     string
	Component string
	Output    string
	kinds     refcache.Kinds
}

// invalidateTerraformStateCache removes the cached outputs for one component.
// Terraform can create an empty state file while selecting a workspace, then later
// populate that same file during apply or deploy. Keeping the empty map cached would
// make downstream !terraform.state expressions continue to use their fallback values.
func invalidateTerraformStateCache(stack, component string) {
	refcache.DeleteTarget(&terraformStateCache, stack, component)
}

// ResetStateCache clears the terraform state cache and the nested-component AuthManager cache.
// This is exported for use in tests to ensure cache isolation between test functions. The two caches
// are cleared together because the managers in nestedAuthManagerCache are what read the state held in
// terraformStateCache: clearing state to force a fresh backend read while reusing a stale auth manager
// would be inconsistent. Neither cache is reset in production.
func ResetStateCache() {
	defer perf.Track(nil, "exec.ResetStateCache")()

	terraformStateCache.Range(func(key, _ any) bool {
		terraformStateCache.Delete(key)
		return true
	})

	ResetNestedAuthManagerCache()
}

// GetTerraformState retrieves a specified Terraform output variable for a given component within a stack.
// It optionally uses a cache to avoid redundant state retrievals and supports both static and dynamic backends.
// Parameters:
//   - atmosConfig: Atmos configuration pointer
//   - yamlFunc: Name of the calling YAML function for error context
//   - stack: Stack identifier
//   - component: Component identifier
//   - output: Output variable key to retrieve
//   - skipCache: Flag to bypass cache lookup
//   - authContext: Optional auth context containing Atmos-managed credentials
//   - authManager: Optional auth manager for nested operations that need authentication
//
// Returns the output value or nil if the component is not provisioned.
//
//nolint:gocognit,revive,cyclop,funlen // Preserve the existing auth-scoped cache and backend lookup branches during extraction.
func (e *Executor) GetTerraformState(
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
	defer perf.Track(atmosConfig, "exec.GetTerraformState")()

	maskOnly := terraform.ResolveLookupOptions(options).SecretsMaskOnly
	kinds := refcache.KindsFor(maskOnly)
	cacheKey := refcache.NewKey(atmosConfig, stack, component)
	cacheKey.Auth = refcache.AuthScope(authContext, authManager, authdeferred.AuthDisabled(atmosConfig.AuthManager))
	cacheKey.Inspection = maskOnly
	cacheKey.Skip = strings.Join(terraform.ResolveLookupOptions(options).Skip, "\x00")
	lookup := &TerraformStateLookup{YamlFunc: yamlFunc, Stack: stack, Component: component, Output: output, kinds: kinds}

	// Keep inspection placeholders and resolved execution values out of each other's lookups.
	var cachedBackend any
	var found bool
	if !skipCache && !authdeferred.IsDeferred(atmosConfig.AuthManager) {
		cachedBackend, found = terraformStateCache.Load(cacheKey)
	}
	if found {
		if _, notProvisioned := cachedBackend.(terraformStateNotProvisionedCacheEntry); notProvisioned {
			return nil, fmt.Errorf("%w for component `%s` in stack `%s`", errUtils.ErrTerraformStateNotProvisioned, component, stack)
		}
		if cached, static := cachedBackend.(terraformStateStaticCacheEntry); static {
			return staticTerraformStateOutput(atmosConfig, lookup, cached.outputs)
		}
		log.Debug(
			"Cache hit",
			"function", yamlFunc,
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
			"output", output,
		)
		result, err := tb.GetTerraformBackendVariable(atmosConfig, cachedBackend.(map[string]any), output)
		if err != nil {
			er := fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%w", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
			return nil, er
		}
		return result, nil
	}

	// Cast authManager from 'any' to auth.AuthManager if provided.
	var parentAuthMgr auth.AuthManager
	if authManager != nil {
		var ok bool
		parentAuthMgr, ok = authManager.(auth.AuthManager)
		if !ok {
			return nil, fmt.Errorf("%w: expected auth.AuthManager", errUtils.ErrInvalidAuthManagerType)
		}
	}

	authDisabled := authdeferred.AuthDisabled(atmosConfig.AuthManager)
	if parentAuthMgr != nil {
		if stackInfo := parentAuthMgr.GetStackInfo(); stackInfo != nil {
			authDisabled = authDisabled || stackInfo.AuthDisabled
		}
	}

	// Resolve AuthManager for this nested component.
	// Checks if component has auth config defined:
	//   - If yes: creates component-specific AuthManager with merged auth config
	//   - If no: uses parent AuthManager (inherits authentication)
	// This enables each nested level to optionally override auth settings.
	resolvedAuthMgr := parentAuthMgr
	var valueCache *deferred.ValueCache
	if authdeferred.IsDeferred(atmosConfig.AuthManager) {
		var err error
		resolvedAuthMgr, valueCache, err = e.services.DeferredTargetAuthAndCache(atmosConfig, component, stack, parentAuthMgr)
		if err != nil {
			return nil, err
		}
	} else if !authDisabled {
		var err error
		resolvedAuthMgr, err = e.ResolveAuthManagerForNestedComponent(atmosConfig, component, stack, parentAuthMgr)
		if err != nil {
			log.Debug(
				"Auth does not exist for nested component, using parent AuthManager",
				"component", component,
				"stack", stack,
				"error", err,
			)
			resolvedAuthMgr = parentAuthMgr
		}
	}
	// Invocation-local caches have no skip partition; bypass them for skipped evaluations.
	if len(terraform.ResolveLookupOptions(options).Skip) > 0 {
		valueCache = nil
	}
	if !skipCache {
		if result, cached, err := cachedTerraformStateOutput(atmosConfig, lookup, valueCache); cached {
			return result, err
		}
	}

	// Derive the effective AuthContext for backend reads.
	// If we resolved a component-specific AuthManager, use its AuthContext instead of the
	// passed-in one (which may be nil when the parent didn't propagate auth).
	resolvedAuthContext := e.services.ResolvedTargetAuthContext(atmosConfig, resolvedAuthMgr, authContext, authDisabled)

	componentSections, err := e.services.ExecuteDescribeComponent(&componentpkg.ExecuteDescribeComponentParams{
		ResolveSecrets:       !maskOnly,
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 terraform.ResolveLookupOptions(options).Skip,
		AuthManager:          resolvedAuthMgr, // Use resolved AuthManager (may be component-specific or inherited)
		AuthDisabled:         authDisabled,
	})
	if err != nil {
		// Use double %w so that errors.Is can match both ErrDescribeComponent and
		// any sentinel propagated from the inner describe (e.g., ErrCircularDependency
		// from a !terraform.state cycle — see #2457).
		er := fmt.Errorf("%w `%s` in stack `%s`\nin YAML function: `%s`\n%w", errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
		return nil, er
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`.
	remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(&componentSections)

	// Read static remote state backend outputs.
	if remoteStateBackendStaticTypeOutputs != nil {
		valueCache.Store(kinds.StaticState, remoteStateBackendStaticTypeOutputs)
		// Cache the result.
		if !authdeferred.IsDeferred(atmosConfig.AuthManager) && (!maskOnly || deferred.IsCacheable(remoteStateBackendStaticTypeOutputs)) {
			terraformStateCache.Store(cacheKey, terraformStateStaticCacheEntry{outputs: remoteStateBackendStaticTypeOutputs})
		}
		return staticTerraformStateOutput(atmosConfig, lookup, remoteStateBackendStaticTypeOutputs)
	}

	// Read Terraform backend using resolved auth context.
	backend, err := tb.GetTerraformBackend(atmosConfig, &componentSections, resolvedAuthContext)
	if err != nil {
		er := fmt.Errorf("%w for component `%s` in stack `%s`\nin YAML function: `%s`\n%w", errUtils.ErrReadTerraformState, component, stack, yamlFunc, err)
		return nil, er
	}

	// Cache a missing state until its component succeeds. ExecuteTerraform invalidates this exact
	// entry after every successful node, so later dependents still see freshly-created state.
	if backend == nil {
		if !authdeferred.IsDeferred(atmosConfig.AuthManager) {
			terraformStateCache.Store(cacheKey, terraformStateNotProvisionedCacheEntry{})
		}
		return nil, fmt.Errorf("%w for component `%s` in stack `%s`", errUtils.ErrTerraformStateNotProvisioned, component, stack)
	}

	// Cache the result now that we know it reflects a real, provisioned backend.
	valueCache.Store(kinds.State, backend)
	if !authdeferred.IsDeferred(atmosConfig.AuthManager) && (!maskOnly || deferred.IsCacheable(backend)) {
		terraformStateCache.Store(cacheKey, backend)
	}

	// Get the output.
	result, err := tb.GetTerraformBackendVariable(atmosConfig, backend, output)
	if err != nil {
		er := fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err) //nolint:errorlint // Preserve the existing public error chain; backend details remain message-only.
		return nil, er
	}

	return result, nil
}

func cachedTerraformStateOutput(ac *schema.AtmosConfiguration, lookup *TerraformStateLookup, cache *deferred.ValueCache) (any, bool, error) {
	if cached, ok := cache.Load(lookup.kinds.StaticState); ok {
		result, err := staticTerraformStateOutput(ac, lookup, cached.(map[string]any))
		return result, true, err
	}
	cached, ok := cache.Load(lookup.kinds.State)
	if !ok {
		return nil, false, nil
	}
	result, err := tb.GetTerraformBackendVariable(ac, cached.(map[string]any), lookup.Output)
	if err != nil {
		return nil, true, fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%w", errUtils.ErrEvaluateTerraformBackendVariable, lookup.Output, lookup.Component, lookup.Stack, lookup.YamlFunc, err)
	}
	return result, true, nil
}

// Static outputs distinguish absent keys from legitimate null values on both cold
// and warm reads, preserving Terraform/YQ fallback behavior.
func staticTerraformStateOutput(ac *schema.AtmosConfiguration, lookup *TerraformStateLookup, outputs map[string]any) (any, error) {
	result, exists, err := tfoutput.GetStaticRemoteStateOutput(ac, lookup.Component, lookup.Stack, outputs, lookup.Output)
	if err != nil {
		return nil, fmt.Errorf("%w for component `%s` in stack `%s`\nin YAML function: `%s`\n%w", errUtils.ErrReadTerraformState, lookup.Component, lookup.Stack, lookup.YamlFunc, err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: output `%s` does not exist for component `%s` in stack `%s`\nin YAML function: `%s`", errUtils.ErrReadTerraformState, lookup.Output, lookup.Component, lookup.Stack, lookup.YamlFunc)
	}
	return result, nil
}
