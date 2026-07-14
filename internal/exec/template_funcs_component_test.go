package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestComponentFunc(t *testing.T) {
	// Skip if terraform is not installed
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("Terraform not found in PATH, skipping test")
	}

	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-3"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Test terraform component `component-2`
	d, err := componentFunc(&atmosConfig, nil, "component-2", "nonprod")
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-b--component-1-c")

	// Test helmfile component `component-3`
	d, err = componentFunc(&atmosConfig, nil, "component-3", "nonprod")
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-b")

	// Test helmfile component `component-4`
	d, err = componentFunc(&atmosConfig, nil, "component-4", "nonprod")
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	// Helmfile components don't have `outputs` (terraform output) - this should result in `<no value>`
	assert.Contains(t, y, "baz: <no value>")
}

// TestResolveComponentFuncAuthManager verifies that atmos.Component()'s auth resolution mirrors
// !terraform.state / !terraform.output: the nested target's own auth section (when it declares a
// default identity) overrides the enclosing component's propagated AuthContext, instead of atmos.Component()
// always reusing the enclosing component's credentials verbatim regardless of the target's own identity.
func TestResolveComponentFuncAuthManager(t *testing.T) {
	t.Parallel()

	parentContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "enclosing-identity"}}
	type sentinelAuthManager struct{ auth.AuthManager }
	targetMgr := &sentinelAuthManager{}

	t.Run("target's own auth section overrides the enclosing component's AuthManager", func(t *testing.T) {
		t.Parallel()

		var gotParent auth.AuthManager
		resolve := func(_ *schema.AtmosConfiguration, component, stack string, parent auth.AuthManager) (auth.AuthManager, error) {
			gotParent = parent
			assert.Equal(t, "target-component", component)
			assert.Equal(t, "target-stack", stack)
			return targetMgr, nil
		}

		got := resolveComponentFuncAuthManager(
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{AuthContext: parentContext},
			"target-component", "target-stack",
			resolve,
		)

		assert.Same(t, targetMgr, got, "the target's own resolved AuthManager must be used")
		require.NotNil(t, gotParent, "the resolver must receive the enclosing component's AuthContext as parent")
		assert.Same(t, parentContext, gotParent.GetStackInfo().AuthContext,
			"the parent passed to the resolver must wrap the enclosing component's AuthContext")
	})

	t.Run("resolver error falls back to the enclosing component's AuthManager", func(t *testing.T) {
		t.Parallel()

		resolve := func(_ *schema.AtmosConfiguration, _, _ string, parent auth.AuthManager) (auth.AuthManager, error) {
			return nil, assert.AnError
		}

		got := resolveComponentFuncAuthManager(
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{AuthContext: parentContext},
			"target-component", "target-stack",
			resolve,
		)

		require.NotNil(t, got, "must fall back to a non-nil wrapper around the enclosing component's AuthContext")
		assert.Same(t, parentContext, got.GetStackInfo().AuthContext)
	})

	t.Run("auth disabled skips resolution and keeps the enclosing component's AuthManager", func(t *testing.T) {
		t.Parallel()

		called := false
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, _ auth.AuthManager) (auth.AuthManager, error) {
			called = true
			return targetMgr, nil
		}

		got := resolveComponentFuncAuthManager(
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{AuthContext: parentContext, AuthDisabled: true},
			"target-component", "target-stack",
			resolve,
		)

		assert.False(t, called, "resolver must not run when auth is disabled")
		require.NotNil(t, got)
		assert.Same(t, parentContext, got.GetStackInfo().AuthContext)
	})

	t.Run("nil enclosing context still lets the target resolve its own auth", func(t *testing.T) {
		t.Parallel()

		var gotParent auth.AuthManager
		resolve := func(_ *schema.AtmosConfiguration, _, _ string, parent auth.AuthManager) (auth.AuthManager, error) {
			gotParent = parent
			return targetMgr, nil
		}

		got := resolveComponentFuncAuthManager(&schema.AtmosConfiguration{}, nil, "target-component", "target-stack", resolve)

		assert.Nil(t, gotParent, "with no enclosing AuthContext, the resolver must receive a nil parent")
		assert.Same(t, targetMgr, got)
	})
}

// TestWrapComponentDescribeError_BreaksErrInvalidComponentChain tests that WrapComponentDescribeError
// correctly breaks the ErrInvalidComponent chain to prevent component type fallback.
// This is a regression test for https://github.com/cloudposse/atmos/issues/1030.
func TestWrapComponentDescribeError_BreaksErrInvalidComponentChain(t *testing.T) {
	tests := []struct {
		name            string
		inputErr        error
		wantErrDescribe bool
		wantErrInvalid  bool
		wantMsgContains string
	}{
		{
			name: "ErrInvalidComponent chain is broken",
			// Use fmt.Errorf with %w for proper error wrapping (causality chain).
			inputErr:        fmt.Errorf("component not found: %w", errUtils.ErrInvalidComponent),
			wantErrDescribe: true,
			wantErrInvalid:  false, // Chain should be broken
			wantMsgContains: "atmos.Component",
		},
		{
			name: "wrapped ErrInvalidComponent chain is broken",
			// Use fmt.Errorf with %w to express "this happened because of that".
			inputErr:        fmt.Errorf("outer error: %w", errUtils.ErrInvalidComponent),
			wantErrDescribe: true,
			wantErrInvalid:  false, // Chain should be broken
			wantMsgContains: "atmos.Component",
		},
		{
			name:            "other errors preserve chain",
			inputErr:        errors.New("network timeout"),
			wantErrDescribe: false,
			wantErrInvalid:  false,
			wantMsgContains: "network timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errUtils.WrapComponentDescribeError("test-comp", "test-stack", tt.inputErr, "atmos.Component")
			require.Error(t, result)

			// Check ErrDescribeComponent.
			if tt.wantErrDescribe {
				assert.ErrorIs(t, result, errUtils.ErrDescribeComponent,
					"Expected ErrDescribeComponent in error chain")
			}

			// Check ErrInvalidComponent - should NOT be in chain for broken cases.
			if tt.wantErrInvalid {
				assert.ErrorIs(t, result, errUtils.ErrInvalidComponent,
					"Expected ErrInvalidComponent in error chain")
			} else {
				assert.NotErrorIs(t, result, errUtils.ErrInvalidComponent,
					"ErrInvalidComponent should NOT be in error chain (chain should be broken)")
			}

			// Check message content.
			if tt.wantMsgContains != "" {
				assert.Contains(t, result.Error(), tt.wantMsgContains)
			}
		})
	}
}
