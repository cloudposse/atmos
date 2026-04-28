package exec

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestShouldResolvePerComponentAuth covers the truth table for the per-component
// auth guard. The regression being guarded against: before
// docs/fixes/2026-04-24-list-instances-per-component-auth.md, the guard was
// `processYamlFunctions` only, so the (templates=true, yaml=false) quadrant
// returned false and `atmos list instances --upload` ran terraform init with an
// empty AuthContext.
func TestShouldResolvePerComponentAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		processTemplates bool
		processYaml      bool
		want             bool
	}{
		{
			name:             "neither_templates_nor_yaml_runs",
			processTemplates: false,
			processYaml:      false,
			want:             false,
		},
		{
			name:             "yaml_only_runs",
			processTemplates: false,
			processYaml:      true,
			want:             true,
		},
		{
			// Regression: this is the `atmos list instances` shape.
			// Must return true so the per-component auth resolver fires
			// and atmos.Component template calls get AWS credentials.
			name:             "templates_only_runs",
			processTemplates: true,
			processYaml:      false,
			want:             true,
		},
		{
			name:             "templates_and_yaml_run",
			processTemplates: true,
			processYaml:      true,
			want:             true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shouldResolvePerComponentAuth(tc.processTemplates, tc.processYaml)
			assert.Equal(t, tc.want, got,
				"shouldResolvePerComponentAuth(processTemplates=%v, processYamlFunctions=%v)",
				tc.processTemplates, tc.processYaml,
			)
		})
	}
}

// componentAuthResolverSpy records the number of times the per-component auth
// resolver was invoked. Used in TestResolveComponentAuthManager to verify that
// the resolver is called in the expected quadrants of the truth table without
// running real OIDC / STS authentication.
type componentAuthResolverSpy struct {
	calls      atomic.Int64
	lastCompID string
	lastStack  string
	returnMgr  auth.AuthManager
	returnErr  error
}

func (s *componentAuthResolverSpy) resolver() componentAuthManagerResolver {
	return func(
		_ *schema.AtmosConfiguration,
		_ map[string]any,
		component string,
		stack string,
		_ auth.AuthManager,
	) (auth.AuthManager, error) {
		s.calls.Add(1)
		s.lastCompID = component
		s.lastStack = stack
		return s.returnMgr, s.returnErr
	}
}

// authSectionWithDefault returns an auth section map that will make
// hasDefaultIdentity return true. The identity name is deliberately arbitrary;
// the spy never looks at it because resolveComponentAuthManager only delegates.
func authSectionWithDefault() map[string]any {
	return map[string]any{
		"identities": map[string]any{
			"ci-default": map[string]any{
				"default": true,
				// hasDefaultIdentity does not read kind/chain; omitted.
			},
		},
	}
}

// authSectionWithoutDefault returns an auth section that declares identities
// but none marked `default: true`, so hasDefaultIdentity returns false.
func authSectionWithoutDefault() map[string]any {
	return map[string]any{
		"identities": map[string]any{
			"ci-ambient": map[string]any{
				"default": false,
			},
		},
	}
}

// componentSectionWithAuth wraps an auth section into a component section map
// with the key that matches cfg.AuthSectionName ("auth").
func componentSectionWithAuth(authSection map[string]any) map[string]any {
	section := map[string]any{
		"vars": map[string]any{"stage": "test"},
	}
	if authSection != nil {
		section["auth"] = authSection
	}
	return section
}

// TestResolveComponentAuthManager exercises describeStacksProcessor.resolveComponentAuthManager
// across all four (processTemplates, processYamlFunctions) quadrants with two
// component shapes: one declaring a default identity in its auth section, one
// without. It uses a spy resolver so no real authentication runs.
//
// Regression assertion: with (processTemplates=true, processYamlFunctions=false)
// and a component that declares a default identity, the spy resolver MUST be
// called. Before the fix this quadrant silently returned the parent AuthManager
// and `atmos list instances --upload` failed against remote backends.
func TestResolveComponentAuthManager(t *testing.T) {
	t.Parallel()

	parentMgr := auth.AuthManager(nil) // parent stays nil; test does not need a real manager.

	// A sentinel component AuthManager returned by the spy. We only check
	// identity by pointer so the test does not depend on AuthManager internals.
	type sentinelAuthManager struct {
		auth.AuthManager
	}
	componentMgr := &sentinelAuthManager{}

	tests := []struct {
		name               string
		processTemplates   bool
		processYaml        bool
		componentSection   map[string]any
		expectResolverCall bool
		expectSentinel     bool // true when result should be the spy's componentMgr
	}{
		{
			name:               "both_off__resolver_skipped",
			processTemplates:   false,
			processYaml:        false,
			componentSection:   componentSectionWithAuth(authSectionWithDefault()),
			expectResolverCall: false,
			expectSentinel:     false,
		},
		{
			name:               "yaml_only__resolver_runs",
			processTemplates:   false,
			processYaml:        true,
			componentSection:   componentSectionWithAuth(authSectionWithDefault()),
			expectResolverCall: true,
			expectSentinel:     true,
		},
		{
			// Regression case: this is the `atmos list instances` shape.
			name:               "templates_only__resolver_runs",
			processTemplates:   true,
			processYaml:        false,
			componentSection:   componentSectionWithAuth(authSectionWithDefault()),
			expectResolverCall: true,
			expectSentinel:     true,
		},
		{
			name:               "both_on__resolver_runs",
			processTemplates:   true,
			processYaml:        true,
			componentSection:   componentSectionWithAuth(authSectionWithDefault()),
			expectResolverCall: true,
			expectSentinel:     true,
		},
		{
			// Component without its own auth section: the resolver must NOT
			// run even when templates are enabled, because there is no
			// component-level identity to resolve. The parent AuthManager
			// is used as-is.
			name:               "templates_on_no_component_auth__resolver_skipped",
			processTemplates:   true,
			processYaml:        false,
			componentSection:   componentSectionWithAuth(nil),
			expectResolverCall: false,
			expectSentinel:     false,
		},
		{
			// Component with an auth section but NO default identity: the
			// resolver must NOT run. This guards against accidentally running
			// auth for components that did not opt in via `default: true`.
			name:               "templates_on_auth_no_default__resolver_skipped",
			processTemplates:   true,
			processYaml:        false,
			componentSection:   componentSectionWithAuth(authSectionWithoutDefault()),
			expectResolverCall: false,
			expectSentinel:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spy := &componentAuthResolverSpy{returnMgr: componentMgr}
			p := &describeStacksProcessor{
				atmosConfig:           &schema.AtmosConfiguration{},
				processTemplates:      tc.processTemplates,
				processYamlFunctions:  tc.processYaml,
				authManager:           parentMgr,
				componentAuthResolver: spy.resolver(),
				finalStacksMap:        make(map[string]any),
			}

			got := p.resolveComponentAuthManager(tc.componentSection, "test-component", "test-stack")

			if tc.expectResolverCall {
				assert.EqualValues(t, 1, spy.calls.Load(),
					"per-component resolver should have been invoked exactly once")
				assert.Equal(t, "test-component", spy.lastCompID, "resolver should receive component name")
				assert.Equal(t, "test-stack", spy.lastStack, "resolver should receive stack name")
			} else {
				assert.EqualValues(t, 0, spy.calls.Load(),
					"per-component resolver should NOT have been invoked")
			}

			if tc.expectSentinel {
				assert.Same(t, componentMgr, got,
					"result should be the component-specific AuthManager returned by the resolver")
			} else {
				// parentMgr is an untyped nil interface; compare to nil directly.
				assert.Nil(t, got,
					"result should fall back to the parent AuthManager (nil in this test)")
			}
		})
	}
}

// TestResolveComponentAuthManager_ResolverErrorFallsBackToParent verifies that
// when the per-component resolver returns an error, we silently fall back to
// the parent AuthManager. This preserves the original swallow-on-error behavior
// of the inline code that was refactored in
// docs/fixes/2026-04-24-list-instances-per-component-auth.md.
func TestResolveComponentAuthManager_ResolverErrorFallsBackToParent(t *testing.T) {
	t.Parallel()

	parentMgr := auth.AuthManager(nil)

	spy := &componentAuthResolverSpy{
		// Return a nil manager + an error: both signal failure in the
		// original code. Either alone is sufficient to trigger fallback.
		returnMgr: nil,
		returnErr: assert.AnError,
	}

	p := &describeStacksProcessor{
		atmosConfig:           &schema.AtmosConfiguration{},
		processTemplates:      true,
		processYamlFunctions:  false,
		authManager:           parentMgr,
		componentAuthResolver: spy.resolver(),
		finalStacksMap:        make(map[string]any),
	}

	got := p.resolveComponentAuthManager(
		componentSectionWithAuth(authSectionWithDefault()),
		"test-component",
		"test-stack",
	)

	require.EqualValues(t, 1, spy.calls.Load(), "resolver should still be called on error path")
	assert.Nil(t, got, "error path should fall back to the parent AuthManager (nil in this test)")
	_ = parentMgr // documented intent: parentMgr is the untyped-nil fallback value.
}
