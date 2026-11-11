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

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/refcache"
)

// Compile-time sentinel for the schema field the hook tests depend on.
var _ = schema.ConfigAndStacksInfo{NodeHooks: nil}

var errRejectedByHook = errors.New("rejected by hook")

// recordingPreparedHooks stands in for the cmd/terraform hooks of init, destroy, and test, which
// wait for preparation before they run.
type recordingPreparedHooks struct {
	events *[]string
	err    error
	ran    int
}

func (h *recordingPreparedHooks) Before(context.Context, *schema.ConfigAndStacksInfo) error {
	return nil
}

func (h *recordingPreparedHooks) After(context.Context, *schema.ConfigAndStacksInfo, string, error) error {
	return nil
}

func (h *recordingPreparedHooks) NeedsPreAuth(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (bool, error) {
	return false, nil
}

func (h *recordingPreparedHooks) BeforePrepared(_ context.Context, _ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, _ schema.ComponentNodeHookWriters) error {
	h.ran++
	*h.events = append(*h.events, "before-hook")
	return h.err
}

func TestExecuteTerragruntUnitRunsPreparedBeforeHooksAfterAuthentication(t *testing.T) {
	harness, info, _ := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.SubCommand = "destroy"
	hooks := &recordingPreparedHooks{events: &harness.events}
	info.NodeHooks = hooks
	// Stand in for the context and manager a real identity attaches during authentication.
	resolvedContext, resolvedManager := &schema.AuthContext{}, &struct{ name string }{name: "development"}
	harness.afterAuth = func(info *schema.ConfigAndStacksInfo) {
		info.AuthContext, info.AuthManager = resolvedContext, resolvedManager
	}
	t.Cleanup(ClearLastAuthContext)
	ClearLastAuthContext()

	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun))

	require.Equal(t, 1, hooks.ran)
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
	info.NodeHooks = &recordingPreparedHooks{events: &harness.events, err: errRejectedByHook}
	t.Cleanup(ClearLastAuthContext)

	err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)

	require.ErrorIs(t, err, errRejectedByHook)
	require.Empty(t, readToolchainProbes(t, probePath), "terragrunt never runs")
}

func TestExecuteTerragruntUnitSkipsHooksThatTestFixturesAlreadyRan(t *testing.T) {
	harness, info, _ := unitHarness(t)
	hooks := &recordingPreparedHooks{events: &harness.events, err: errRejectedByHook}
	info.NodeHooks = hooks
	t.Cleanup(ClearLastAuthContext)

	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, terragruntUnitRun{shouldProcess: true}))
	require.Zero(t, hooks.ran)
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
