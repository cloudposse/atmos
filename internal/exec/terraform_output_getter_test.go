package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// newStackInfoAuthManager returns a generated MockAuthManager whose GetStackInfo yields the given
// ConfigAndStacksInfo. Only GetStackInfo is stubbed, which keeps the tests honest about what the
// code under test uses.
func newStackInfoAuthManager(t *testing.T, stackInfo *schema.ConfigAndStacksInfo) *types.MockAuthManager {
	t.Helper()

	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetStackInfo().Return(stackInfo).AnyTimes()
	return mgr
}

// TestResolveNestedOutputAuth verifies that `!terraform.output` resolves the nested target's own
// auth section (when it declares a default identity) instead of always fetching the target's
// outputs with the enclosing component's credentials verbatim — the same defect fixed for
// atmos.Component() and long since handled by !terraform.state.
func TestResolveNestedOutputAuth(t *testing.T) {
	t.Parallel()

	enclosingContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "enclosing-identity"}}
	targetContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "target-identity"}}

	t.Run("target's own auth section overrides the enclosing component's auth", func(t *testing.T) {
		t.Parallel()
		enclosingMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: enclosingContext})
		targetMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: targetContext})

		var gotParent auth.AuthManager
		resolve := func(_ *schema.AtmosConfiguration, component, stack string, parent auth.AuthManager) (auth.AuthManager, error) {
			gotParent = parent
			assert.Equal(t, "target-component", component)
			assert.Equal(t, "target-stack", stack)
			return targetMgr, nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, enclosingMgr, resolve,
		)

		assert.Same(t, targetContext, gotCtx, "the target's own AuthContext must drive the output fetch")
		assert.Same(t, targetMgr, gotMgr, "the target's own resolved AuthManager must be used")
		assert.Same(t, auth.AuthManager(enclosingMgr), gotParent, "the resolver must receive the enclosing manager as parent")
	})

	t.Run("target without its own default identity inherits the enclosing auth unchanged", func(t *testing.T) {
		t.Parallel()
		enclosingMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: enclosingContext})

		resolve := func(_ *schema.AtmosConfiguration, _, _ string, parent auth.AuthManager) (auth.AuthManager, error) {
			return parent, nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, enclosingMgr, resolve,
		)

		assert.Same(t, enclosingContext, gotCtx)
		assert.Same(t, auth.AuthManager(enclosingMgr), gotMgr)
	})

	t.Run("resolver error falls back to the enclosing auth", func(t *testing.T) {
		t.Parallel()
		enclosingMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: enclosingContext})

		resolve := func(_ *schema.AtmosConfiguration, _, _ string, _ auth.AuthManager) (auth.AuthManager, error) {
			return nil, assert.AnError
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, enclosingMgr, resolve,
		)

		assert.Same(t, enclosingContext, gotCtx)
		assert.Same(t, auth.AuthManager(enclosingMgr), gotMgr)
	})

	t.Run("auth disabled on the enclosing component skips resolution entirely", func(t *testing.T) {
		t.Parallel()

		disabledMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthDisabled: true})
		called := false
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, _ auth.AuthManager) (auth.AuthManager, error) {
			called = true
			return newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: targetContext}), nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, disabledMgr, resolve,
		)

		assert.False(t, called, "resolver must not run when auth is disabled")
		assert.Same(t, enclosingContext, gotCtx)
		assert.Same(t, auth.AuthManager(disabledMgr), gotMgr)
	})

	t.Run("nil enclosing auth still lets the target resolve its own auth", func(t *testing.T) {
		t.Parallel()
		targetMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: targetContext})

		var gotParent auth.AuthManager
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, parent auth.AuthManager) (auth.AuthManager, error) {
			gotParent = parent
			return targetMgr, nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			nil, nil, resolve,
		)

		assert.Nil(t, gotParent, "with no enclosing auth, the resolver must receive a nil parent")
		assert.Same(t, targetContext, gotCtx)
		assert.Same(t, auth.AuthManager(targetMgr), gotMgr)
	})

	t.Run("resolver returning nil keeps the enclosing auth", func(t *testing.T) {
		t.Parallel()

		resolve := func(_ *schema.AtmosConfiguration, _, _ string, parent auth.AuthManager) (auth.AuthManager, error) {
			return parent, nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, nil, resolve,
		)

		assert.Same(t, enclosingContext, gotCtx)
		assert.Nil(t, gotMgr)
	})

	t.Run("resolved manager without an AuthContext keeps the enclosing context", func(t *testing.T) {
		t.Parallel()

		enclosingMgr := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: enclosingContext})
		bareMgr := newStackInfoAuthManager(t, nil)
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, _ auth.AuthManager) (auth.AuthManager, error) {
			return bareMgr, nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, enclosingMgr, resolve,
		)

		assert.Same(t, enclosingContext, gotCtx)
		assert.Same(t, auth.AuthManager(bareMgr), gotMgr)
	})

	t.Run("non-AuthManager value passes through untouched for the output layer to reject", func(t *testing.T) {
		t.Parallel()

		called := false
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, _ auth.AuthManager) (auth.AuthManager, error) {
			called = true
			return newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: targetContext}), nil
		}

		gotCtx, gotMgr := resolveNestedOutputAuth(
			&schema.AtmosConfiguration{}, "target-component", "target-stack",
			enclosingContext, "not-an-auth-manager", resolve,
		)

		assert.False(t, called, "resolver must not run for an invalid authManager type")
		assert.Same(t, enclosingContext, gotCtx)
		require.Equal(t, "not-an-auth-manager", gotMgr)
	})
}

func TestGetAllTerraformOutputs_PanicWhenNoExecutor(t *testing.T) {
	// Save and restore original executor.
	originalExecutor := tfoutput.GetDefaultExecutor()
	defer tfoutput.SetDefaultExecutor(originalExecutor)

	tfoutput.SetDefaultExecutor(nil)

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(
		t,
		"output.SetDefaultExecutor must be called before GetComponentOutputs",
		func() {
			_, _ = GetAllTerraformOutputs(atmosConfig, "component", "stack", false, nil, nil)
		},
	)
}

func TestGetTerraformOutput_PanicWhenNoExecutor(t *testing.T) {
	// Save and restore original executor.
	originalExecutor := tfoutput.GetDefaultExecutor()
	defer tfoutput.SetDefaultExecutor(originalExecutor)

	tfoutput.SetDefaultExecutor(nil)

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(
		t,
		"output.SetDefaultExecutor must be called before GetOutput",
		func() {
			_, _, _ = GetTerraformOutput(atmosConfig, "stack", "component", "output", false, nil, nil)
		},
	)
}
