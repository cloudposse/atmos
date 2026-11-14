//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_node_hooks_test.go -package=terraform github.com/cloudposse/atmos/pkg/schema ComponentNodeHooksWithPreparation

package terraform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/refcache"
)

// Compile-time sentinel for the schema field the hook tests depend on.
var _ = schema.ConfigAndStacksInfo{NodeHooks: nil}

var errRejectedByHook = errors.New("rejected by hook")

func preparedHookMock(t *testing.T) *MockComponentNodeHooksWithPreparation {
	t.Helper()
	hooks := NewMockComponentNodeHooksWithPreparation(gomock.NewController(t))
	hooks.EXPECT().NeedsPreAuth(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	return hooks
}

func TestExecuteTerragruntUnitRunsPreparedBeforeHooksAfterAuthentication(t *testing.T) {
	harness, info, _ := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.SubCommand = "destroy"
	hooks := preparedHookMock(t)
	hooks.EXPECT().BeforePrepared(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, schema.ComponentNodeHookWriters) error {
			harness.events = append(harness.events, "before-hook")
			return nil
		},
	).Times(1)
	info.NodeHooks = hooks
	// Stand in for the context and manager a real identity attaches during authentication.
	resolvedContext, resolvedManager := &schema.AuthContext{}, &struct{ name string }{name: "development"}
	harness.afterAuth = func(info *schema.ConfigAndStacksInfo) {
		info.AuthContext, info.AuthManager = resolvedContext, resolvedManager
	}
	t.Cleanup(ClearLastAuthContext)
	ClearLastAuthContext()

	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun))

	require.Equal(t, []string{"authenticate", "toolchain", "before-hook"}, harness.events, "the hook runs with the unit's identity, before terragrunt")
	require.Len(t, readToolchainProbes(t, probePath), 2, "the version probe and the unit command ran after the hook")
	authContext, authManager := GetLastAuthContext()
	require.Same(t, resolvedContext, authContext, "after-hooks reuse the identity the unit ran with")
	require.Same(t, resolvedManager, authManager)
}

func TestExecuteTerragruntUnitStopsWhenABeforeHookRejectsTheOperation(t *testing.T) {
	harness, info, _ := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.SubCommand = "destroy"
	hooks := preparedHookMock(t)
	hooks.EXPECT().BeforePrepared(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, schema.ComponentNodeHookWriters) error {
			harness.events = append(harness.events, "before-hook")
			return errRejectedByHook
		},
	).Times(1)
	info.NodeHooks = hooks
	t.Cleanup(ClearLastAuthContext)

	err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)

	require.ErrorIs(t, err, errRejectedByHook)
	require.Empty(t, readToolchainProbes(t, probePath), "terragrunt never runs")
}

func TestExecuteTerragruntUnitSkipsHooksThatTestFixturesAlreadyRan(t *testing.T) {
	harness, info, _ := unitHarness(t)
	hooks := preparedHookMock(t)
	info.NodeHooks = hooks
	t.Cleanup(ClearLastAuthContext)

	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, terragruntUnitRun{shouldProcess: true}))
}

func TestExecuteTerragruntUnitInvalidatesStateAfterFailure(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry-run=%t", dryRun), func(t *testing.T) {
			harness, info, _ := unitHarness(t)
			info.DryRun = dryRun
			info.ComponentEnvSection[testEnvProbeExit] = "1"
			key := refcache.NewKey(nil, info.Stack, info.ComponentFromArg)
			terraformStateCache.Store(key, terraformStateStaticCacheEntry{outputs: map[string]any{"value": "stale"}})
			t.Cleanup(func() { terraformStateCache.Delete(key); ClearLastAuthContext() })
			err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)
			if dryRun {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			_, found := terraformStateCache.Load(key)
			require.Equal(t, dryRun, found)
		})
	}
}

func TestExecuteTerragruntUnitHonorsInitModeForPlan(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     schema.TerraformInitMode
		command  string
		explicit bool
		skip     bool
	}{
		{name: "never", mode: schema.TerraformInitModeNever, skip: true},
		{name: "auto", mode: schema.TerraformInitModeAuto},
		{name: "always", mode: schema.TerraformInitModeAlways},
		{name: "explicit skip", mode: schema.TerraformInitModeAuto, explicit: true, skip: true},
		{name: "other command", mode: schema.TerraformInitModeNever, command: "apply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness, info, _ := unitHarness(t)
			harness.config.Components.Terraform.Init.Mode = tc.mode
			info.SkipInit = tc.explicit
			if tc.command != "" {
				info.SubCommand = tc.command
			}
			probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
			require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun))
			probes := readToolchainProbes(t, probePath)
			require.Len(t, probes, 2)
			if tc.skip {
				require.Contains(t, probes[1].Arguments, "--no-auto-init")
			} else {
				require.NotContains(t, probes[1].Arguments, "--no-auto-init")
			}
		})
	}
}

func TestResolveTerragruntPathUsesInvocationEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a Unix executable")
	}
	ambient := t.TempDir()
	invocation := t.TempDir()
	for _, dir := range []string{ambient, invocation} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt"), []byte("#!/bin/sh\n"), 0o700))
	}
	t.Setenv("PATH", ambient)
	got, err := resolveTerragruntPath("terragrunt", nil, []string{"PATH=" + invocation})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(invocation, "terragrunt"), got)
	got, err = resolveTerragruntPath("terragrunt", nil, nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(ambient, "terragrunt"), got)
	_, err = resolveTerragruntPath("terragrunt", []string{"PATH="}, nil)
	require.ErrorIs(t, err, exec.ErrNotFound)
}
