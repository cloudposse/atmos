package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newListCmd must reject unexpected positional arguments (e.g. a typo'd
// component name) rather than silently ignoring them and running the
// account-wide ListStacks request anyway.
func TestNewListCmd_RejectsPositionalArgs(t *testing.T) {
	cmd := newListCmd()
	require.NotNil(t, cmd.Args, "list must set an Args validator")

	err := cmd.Args(cmd, []string{"unexpected"})
	require.Error(t, err)
}

// newListCmd must accept zero positional arguments — the common case.
func TestNewListCmd_AcceptsNoArgs(t *testing.T) {
	cmd := newListCmd()
	require.NotNil(t, cmd.Args)

	require.NoError(t, cmd.Args(cmd, []string{}))
}

// cloudFormationComponentStackName must extract
// components.aws/cloudformation.<component>.stack_name, returning ("", false)
// gracefully at every level where the expected shape is absent instead of
// panicking on a bad type assertion.
func TestCloudFormationComponentStackName(t *testing.T) {
	validStacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"vpc": map[string]any{
						"stack_name": "my-vpc-stack",
					},
					"no-stack-name": map[string]any{},
					"empty-stack-name": map[string]any{
						"stack_name": "",
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		stacksMap     map[string]any
		stack         string
		componentName string
		wantName      string
		wantOK        bool
	}{
		{
			name:          "resolves configured stack_name",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "vpc",
			wantName:      "my-vpc-stack",
			wantOK:        true,
		},
		{
			name:          "missing stack key",
			stacksMap:     validStacksMap,
			stack:         "does-not-exist",
			componentName: "vpc",
		},
		{
			name:          "missing components key",
			stacksMap:     map[string]any{"dev": map[string]any{}},
			stack:         "dev",
			componentName: "vpc",
		},
		{
			name: "missing type key",
			stacksMap: map[string]any{
				"dev": map[string]any{"components": map[string]any{}},
			},
			stack:         "dev",
			componentName: "vpc",
		},
		{
			name:          "missing component key",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "does-not-exist",
		},
		{
			name:          "missing stack_name field",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "no-stack-name",
		},
		{
			name:          "empty stack_name field",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "empty-stack-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := cloudFormationComponentStackName(tt.stacksMap, tt.stack, tt.componentName)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

// configuredCloudFormationStackNames must build the set of configured
// stack_name values (only including components that actually configured one)
// from the described stacks map.
func TestConfiguredCloudFormationStackNames(t *testing.T) {
	origDescribe, origList := cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnDescribeStacks = origDescribe
		cfnListAllComponents = origList
	})

	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"vpc":           map[string]any{"stack_name": "my-vpc-stack"},
					"no-stack-name": map[string]any{},
				},
			},
		},
	}

	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return stacksMap, nil
	}
	type ctxKey string
	const key ctxKey = "test-marker"
	ctx := context.WithValue(context.Background(), key, "expected")

	var receivedCtx context.Context
	cfnListAllComponents = func(ctx context.Context, _ string, _ map[string]any) ([]string, error) {
		receivedCtx = ctx
		return []string{"vpc", "no-stack-name"}, nil
	}

	names, err := configuredCloudFormationStackNames(ctx, &schema.AtmosConfiguration{}, "dev", nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"my-vpc-stack": true}, names)
	require.NotNil(t, receivedCtx, "configuredCloudFormationStackNames must propagate its context to ListAllComponents")
	assert.Equal(t, "expected", receivedCtx.Value(key), "the caller-supplied context must reach ListAllComponents unchanged, not context.Background()")
}

// configuredCloudFormationStackNames must propagate a describe-stacks failure.
func TestConfiguredCloudFormationStackNames_DescribeError(t *testing.T) {
	origDescribe := cfnDescribeStacks
	t.Cleanup(func() { cfnDescribeStacks = origDescribe })

	sentinel := errors.New("describe failed")
	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return nil, sentinel
	}

	_, err := configuredCloudFormationStackNames(context.Background(), &schema.AtmosConfiguration{}, "dev", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// configuredCloudFormationStackNames must propagate a list-components failure.
func TestConfiguredCloudFormationStackNames_ListError(t *testing.T) {
	origDescribe, origList := cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnDescribeStacks = origDescribe
		cfnListAllComponents = origList
	})

	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{}}, nil
	}
	sentinel := errors.New("list failed")
	cfnListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return nil, sentinel
	}

	_, err := configuredCloudFormationStackNames(context.Background(), &schema.AtmosConfiguration{}, "dev", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// twoStackFixture is a describe-stacks result with two Atmos stacks, each configuring
// one aws/cloudformation component with its own stack_name plus an abstract component the
// provider's component listing excludes.
func twoStackFixture() map[string]any {
	component := func(name string) map[string]any {
		return map[string]any{
			"aws/cloudformation": map[string]any{
				"vpc":  map[string]any{"stack_name": name},
				"base": map[string]any{"stack_name": "abstract-" + name},
			},
		}
	}
	return map[string]any{
		"dev":  map[string]any{"components": component("dev-vpc")},
		"prod": map[string]any{"components": component("prod-vpc")},
	}
}

// installListSeams stubs the describe-stacks and component-listing seams for the configured
// stack-name tests, and records the stack filter passed to describe-stacks.
func installListSeams(t *testing.T, stacksMap map[string]any, gotFilter *string) {
	t.Helper()
	origDescribe, origList := cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnDescribeStacks = origDescribe
		cfnListAllComponents = origList
	})
	cfnDescribeStacks = func(_ *schema.AtmosConfiguration, filter string, _ []string, _ []string, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		if gotFilter != nil {
			*gotFilter = filter
		}
		if filter == "" {
			return stacksMap, nil
		}
		if section, ok := stacksMap[filter]; ok {
			return map[string]any{filter: section}, nil
		}
		return map[string]any{}, nil
	}
	// The stand-in mirrors the provider: concrete components only, never the abstract "base".
	cfnListAllComponents = func(_ context.Context, _ string, stacks map[string]any) ([]string, error) {
		if len(stacks) != 1 {
			return nil, fmt.Errorf("expected one stack per listing, got %d", len(stacks))
		}
		return []string{"vpc"}, nil
	}
}

// Without --stack, every Atmos stack's configured stack_names must be collected, so Atmos-defined
// stacks are not all reported as "unmanaged". Before the fix the lookup used stacksMap[""] and
// returned an empty set.
func TestConfiguredCloudFormationStackNames_AllStacksWhenNoStack(t *testing.T) {
	var gotFilter string
	installListSeams(t, twoStackFixture(), &gotFilter)

	names, err := configuredCloudFormationStackNames(context.Background(), &schema.AtmosConfiguration{}, "", nil)
	require.NoError(t, err)
	assert.Empty(t, gotFilter, "no --stack must describe every stack")
	assert.Equal(t, map[string]bool{"dev-vpc": true, "prod-vpc": true}, names)
}

// With --stack, only that stack's stack_names count.
func TestConfiguredCloudFormationStackNames_SingleStackScope(t *testing.T) {
	installListSeams(t, twoStackFixture(), nil)

	names, err := configuredCloudFormationStackNames(context.Background(), &schema.AtmosConfiguration{}, "prod", nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"prod-vpc": true}, names)
}

// An unknown --stack must be an ErrStackNotFound error (with a hint to list stacks), not an
// rc-0 listing of everything as unmanaged.
func TestConfiguredCloudFormationStackNames_UnknownStack(t *testing.T) {
	installListSeams(t, twoStackFixture(), nil)

	_, err := configuredCloudFormationStackNames(context.Background(), &schema.AtmosConfiguration{}, "nosuchstack", nil)
	require.ErrorIs(t, err, errUtils.ErrStackNotFound)
	assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "atmos list stacks")
}

// A bogus --status must fail with ErrInvalidFlag before config loading, auth or any AWS call.
func TestRunList_InvalidStatusFailsFirst(t *testing.T) {
	origInit, origAuth := cfnInitCliConfig, cfnCreateListAuthManager
	t.Cleanup(func() { cfnInitCliConfig, cfnCreateListAuthManager = origInit, origAuth })
	cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		t.Fatal("config must not be loaded for an invalid --status")
		return schema.AtmosConfiguration{}, nil
	}

	cmd := newListCmd()
	require.NoError(t, cmd.Flags().Set("status", "bogus"))
	require.ErrorIs(t, runList(cmd, nil), errUtils.ErrInvalidFlag)
}

// list must resolve its identity through the stack-scanning auth helper (so a component-level
// default identity is honoured without --identity), passing the explicit --identity value through.
func TestRunList_UsesStackScanAuth(t *testing.T) {
	origInit, origAuth, origDescribe := cfnInitCliConfig, cfnCreateListAuthManager, cfnDescribeStacks
	t.Cleanup(func() {
		cfnInitCliConfig, cfnCreateListAuthManager, cfnDescribeStacks = origInit, origAuth, origDescribe
	})
	cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	sentinel := errors.New("stop after describe")
	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return nil, sentinel
	}
	var gotIdentity, gotSelect string
	calls := 0
	cfnCreateListAuthManager = func(identity string, _ *schema.AuthConfig, selectValue string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		calls++
		gotIdentity, gotSelect = identity, selectValue
		return nil, nil
	}

	err := runList(newListCmd(), nil)
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls)
	assert.Empty(t, gotIdentity, "no --identity must hand an empty identity to the scanning helper")
	assert.Equal(t, cfg.IdentityFlagSelectValue, gotSelect)
}

// An auth-creation failure must be wrapped with ErrFailedToInitializeAuthManager.
func TestRunList_AuthManagerFailure(t *testing.T) {
	origInit, origAuth := cfnInitCliConfig, cfnCreateListAuthManager
	t.Cleanup(func() { cfnInitCliConfig, cfnCreateListAuthManager = origInit, origAuth })
	cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	cause := errors.New("sso expired")
	cfnCreateListAuthManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, cause
	}

	err := runList(newListCmd(), nil)
	require.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
	require.ErrorIs(t, err, cause)
}

// withIdentityHint must add the --identity hint only to credential failures when no identity was
// resolved; every other combination passes the error through untouched.
func TestWithIdentityHint(t *testing.T) {
	imds := errors.New("operation error CloudFormation: ListStacks, failed to retrieve credentials: failed to refresh cached credentials, no EC2 IMDS role found")
	other := errors.New("operation error CloudFormation: ListStacks, AccessDenied")

	tests := []struct {
		name             string
		err              error
		identityResolved bool
		wantHint         bool
	}{
		{name: "credential failure without identity", err: imds, wantHint: true},
		{name: "credential failure with identity", err: imds, identityResolved: true},
		{name: "other failure without identity", err: other},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withIdentityHint(tt.err, tt.identityResolved)
			require.ErrorIs(t, got, tt.err)
			hasHint := len(errUtils.Format(got, errUtils.FormatterConfig{})) > len(tt.err.Error()) &&
				containsIdentityFlag(errUtils.Format(got, errUtils.FormatterConfig{}))
			assert.Equal(t, tt.wantHint, hasHint)
		})
	}
}

func containsIdentityFlag(formatted string) bool {
	return strings.Contains(formatted, "--identity")
}
