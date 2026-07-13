package kubernetes

import (
	"bytes"
	"context"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/hooks"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNormalizeGlobalConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	normalizeGlobalConfig(atmosConfig)

	assert.Equal(t, DefaultConfig().BasePath, atmosConfig.Components.Kubernetes.BasePath)
	assert.Equal(t, DefaultConfig().Provider, atmosConfig.Components.Kubernetes.Provider)

	atmosConfig.Components.Kubernetes.BasePath = "custom"
	atmosConfig.Components.Kubernetes.Provider = ProviderKustomize
	normalizeGlobalConfig(atmosConfig)
	assert.Equal(t, "custom", atmosConfig.Components.Kubernetes.BasePath)
	assert.Equal(t, ProviderKustomize, atmosConfig.Components.Kubernetes.Provider)
}

func TestAuthManagerForBulkSkipsEmptyIdentity(t *testing.T) {
	manager, err := authManagerForBulk(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Nil(t, manager)
}

func TestAuthManagerForBulkReturnsErrorForUnconfiguredAuth(t *testing.T) {
	// An identity is requested but the auth config has no identities, so manager
	// creation must fail rather than silently returning a nil manager.
	info := &schema.ConfigAndStacksInfo{Identity: "aws-admin"}
	manager, err := authManagerForBulk(&schema.AtmosConfiguration{}, info)

	require.Error(t, err)
	assert.Nil(t, manager)
}

func TestGraphSelectionForBulkSkipsWhenNotAffected(t *testing.T) {
	selection, err := graphSelectionForBulk(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
	)

	require.NoError(t, err)
	assert.Nil(t, selection)
}

func TestGraphSelectionForBulkBuildsAffectedSelection(t *testing.T) {
	original := affectedKubernetesComponentsFunc
	t.Cleanup(func() { affectedKubernetesComponentsFunc = original })

	affectedKubernetesComponentsFunc = func(*component.ExecutionContext, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return []schema.Affected{
			{Component: "api", ComponentType: cfg.KubernetesComponentType, Stack: "dev"},
			{Component: "deleted", ComponentType: cfg.KubernetesComponentType, Stack: "dev", Deleted: true},
			{Component: "vpc", ComponentType: "terraform", Stack: "dev"},
		}, nil
	}

	selection, err := graphSelectionForBulk(
		&component.ExecutionContext{Flags: map[string]any{"include-dependents": true}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{Affected: true},
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, []string{component.GraphNodeID("api", "dev")}, selection.NodeIDs)
	assert.True(t, selection.IncludeDependencies)
	assert.True(t, selection.IncludeDependents)
}

func TestGraphSelectionForBulkReturnsAffectedError(t *testing.T) {
	original := affectedKubernetesComponentsFunc
	t.Cleanup(func() { affectedKubernetesComponentsFunc = original })

	affectedKubernetesComponentsFunc = func(*component.ExecutionContext, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return nil, errors.New("affected failed")
	}

	selection, err := graphSelectionForBulk(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{Affected: true},
	)

	require.ErrorContains(t, err, "affected failed")
	assert.Nil(t, selection)
}

func TestAffectedKubernetesComponentsUsesTargetRepoPath(t *testing.T) {
	restoreAffectedExecutionStubs := stubAffectedExecutionFailures(t)
	t.Cleanup(restoreAffectedExecutionStubs)

	executeAffectedWithRepoPath = func(
		atmosConfig *schema.AtmosConfiguration,
		targetRefPath string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
		authManager auth.AuthManager,
		authDisabled bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		assert.Equal(t, "/repo", targetRefPath)
		assert.False(t, includeSpaceliftAdminStacks)
		assert.False(t, includeSettings)
		assert.Equal(t, "dev", stack)
		assert.True(t, processTemplates)
		assert.True(t, processYamlFunctions)
		assert.Equal(t, []string{"excluded"}, skip)
		assert.True(t, authDisabled)
		return []schema.Affected{{Component: "api", Stack: "dev", ComponentType: cfg.KubernetesComponentType}}, nil, nil, "", nil
	}

	affected, err := affectedKubernetesComponents(
		&component.ExecutionContext{Flags: map[string]any{"repo-path": "/repo"}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{Stack: "dev", Skip: []string{"excluded"}, AuthDisabled: true},
	)

	require.NoError(t, err)
	require.Equal(t, []schema.Affected{{Component: "api", Stack: "dev", ComponentType: cfg.KubernetesComponentType}}, affected)
}

func TestAffectedKubernetesComponentsUsesTargetRefClone(t *testing.T) {
	restoreAffectedExecutionStubs := stubAffectedExecutionFailures(t)
	t.Cleanup(restoreAffectedExecutionStubs)

	executeAffectedWithRefClone = func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		sshKeyPath string,
		sshKeyPassword string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
		authManager auth.AuthManager,
		authDisabled bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		assert.Equal(t, "feature", ref)
		assert.Equal(t, "abc123", sha)
		assert.Equal(t, "/tmp/key", sshKeyPath)
		assert.Equal(t, "password", sshKeyPassword)
		return []schema.Affected{{Component: "api"}}, nil, nil, "", nil
	}

	affected, err := affectedKubernetesComponents(
		&component.ExecutionContext{Flags: map[string]any{
			"clone-target-ref": true,
			"ref":              "feature",
			"sha":              "abc123",
			"ssh-key":          "/tmp/key",
			"ssh-key-password": "password",
		}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
	)

	require.NoError(t, err)
	require.Equal(t, []schema.Affected{{Component: "api"}}, affected)
}

func TestAffectedKubernetesComponentsUsesTargetRefCheckout(t *testing.T) {
	restoreAffectedExecutionStubs := stubAffectedExecutionFailures(t)
	t.Cleanup(restoreAffectedExecutionStubs)

	executeAffectedWithRefCheckout = func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		targetBranch string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
		authManager auth.AuthManager,
		authDisabled bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		assert.Equal(t, "main", ref)
		assert.Empty(t, sha)
		assert.Empty(t, targetBranch)
		return []schema.Affected{{Component: "api"}}, nil, nil, "", nil
	}

	affected, err := affectedKubernetesComponents(
		&component.ExecutionContext{Flags: map[string]any{"base": "main"}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
	)

	require.NoError(t, err)
	require.Equal(t, []schema.Affected{{Component: "api"}}, affected)
}

func TestAffectedKubernetesComponentsTreatsBaseCommitAsSHA(t *testing.T) {
	restoreAffectedExecutionStubs := stubAffectedExecutionFailures(t)
	t.Cleanup(restoreAffectedExecutionStubs)

	commit := "0123456789abcdef0123456789abcdef01234567"
	executeAffectedWithRefCheckout = func(
		_ *schema.AtmosConfiguration,
		ref string,
		sha string,
		_ string,
		_ bool,
		_ bool,
		_ string,
		_ bool,
		_ bool,
		_ []string,
		_ bool,
		_ auth.AuthManager,
		_ bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		assert.Empty(t, ref)
		assert.Equal(t, commit, sha)
		return []schema.Affected{{Component: "api", Stack: commit}}, nil, nil, "", nil
	}

	affected, err := affectedKubernetesComponents(
		&component.ExecutionContext{Flags: map[string]any{"base": commit}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
	)

	require.NoError(t, err)
	require.Equal(t, commit, affected[0].Stack)
}

func TestResolveProvider(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	assert.Equal(t, DefaultConfig().Provider, resolveProvider(atmosConfig, nil))

	atmosConfig.Components.Kubernetes.Provider = ProviderKustomize
	assert.Equal(t, ProviderKustomize, resolveProvider(atmosConfig, nil))

	assert.Equal(t, ProviderKubectl, resolveProvider(atmosConfig, map[string]any{"provider": ProviderKubectl}))
}

func TestMaybeAutoGenerateFilesSkipsWhenDisabledOrUnset(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}

	require.NoError(t, maybeAutoGenerateFiles(atmosConfig, info, t.TempDir()))

	atmosConfig.Components.Kubernetes.AutoGenerateFiles = true
	require.NoError(t, maybeAutoGenerateFiles(atmosConfig, info, t.TempDir()))
}

func TestProcessStacksWithAuthUsesProcessStacksWhenIdentityUnset(t *testing.T) {
	original := processStacks
	t.Cleanup(func() { processStacks = original })

	processStacks = func(
		atmosConfig *schema.AtmosConfiguration,
		info schema.ConfigAndStacksInfo,
		checkStack bool,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		authManager auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error) {
		assert.True(t, checkStack)
		assert.True(t, processTemplates)
		assert.True(t, processYamlFunctions)
		assert.Nil(t, authManager)
		info.ComponentIsEnabled = true
		return info, nil
	}

	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "api"}
	require.NoError(t, processStacksWithAuth(&schema.AtmosConfiguration{}, &info))
	assert.True(t, info.ComponentIsEnabled)
}

func TestResolveComponentPathUsesProvisionResolver(t *testing.T) {
	original := provisionAndResolveComponentPath
	t.Cleanup(func() { provisionAndResolveComponentPath = original })

	expectedPath := filepath.Join(t.TempDir(), "resolved")
	provisionAndResolveComponentPath = func(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		info *schema.ConfigAndStacksInfo,
		componentType string,
		fallbackComponentPath string,
	) (string, bool, error) {
		assert.Equal(t, cfg.KubernetesComponentType, componentType)
		assert.Contains(t, fallbackComponentPath, filepath.Join("components", "kubernetes", "api"))
		return expectedPath, true, nil
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Components: schema.Components{
			Kubernetes: schema.Kubernetes{BasePath: filepath.Join("components", "kubernetes")},
		},
	}
	path, exists, err := resolveComponentPath(atmosConfig, &schema.ConfigAndStacksInfo{FinalComponent: "api"})

	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, expectedPath, path)
}

func TestExecuteBulkDelegatesDescribeStacksToGraphExecutor(t *testing.T) {
	originalDescribeStacks := executeDescribeStacks
	originalExecuteGraph := executeGraph
	t.Cleanup(func() {
		executeDescribeStacks = originalDescribeStacks
		executeGraph = originalExecuteGraph
	})

	stacks := map[string]any{"dev": map[string]any{}}
	executeDescribeStacks = func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error) {
		assert.Equal(t, "dev", filterByStack)
		assert.Equal(t, []string{cfg.KubernetesComponentType}, componentTypes)
		assert.True(t, processTemplates)
		assert.True(t, processYamlFunctions)
		assert.True(t, includeEmptyStacks)
		return stacks, nil
	}

	var graphOptions component.GraphExecutionOptions
	executeGraph = func(ctx context.Context, opts *component.GraphExecutionOptions) error {
		graphOptions = *opts
		return nil
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", All: true}
	flags := map[string]any{"include-dependents": true}

	err := executeBulk(&component.ExecutionContext{Flags: flags}, atmosConfig, info, OperationApply)

	require.NoError(t, err)
	assert.Equal(t, stacks, graphOptions.Stacks)
	assert.Equal(t, cfg.KubernetesComponentType, graphOptions.ComponentType)
	assert.Equal(t, "apply", graphOptions.SubCommand)
	assert.Equal(t, flags, graphOptions.Flags)
	assert.Nil(t, graphOptions.Selection)
}

func TestExecuteBulkReturnsDescribeStacksError(t *testing.T) {
	original := executeDescribeStacks
	t.Cleanup(func() { executeDescribeStacks = original })

	executeDescribeStacks = func(
		*schema.AtmosConfiguration,
		string,
		[]string,
		[]string,
		[]string,
		bool,
		bool,
		bool,
		bool,
		[]string,
		auth.AuthManager,
	) (map[string]any, error) {
		return nil, errors.New("describe failed")
	}

	err := executeBulk(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationApply)
	require.ErrorContains(t, err, "describe failed")
}

func TestExecuteRenderWithStubbedExternalDependencies(t *testing.T) {
	originalInit := initCliConfig
	originalProcessStacks := processStacks
	originalProvision := provisionAndResolveComponentPath
	originalDependencies := dependenciesForComponent
	originalHooks := getHooks
	t.Cleanup(func() {
		initCliConfig = originalInit
		processStacks = originalProcessStacks
		provisionAndResolveComponentPath = originalProvision
		dependenciesForComponent = originalDependencies
		getHooks = originalHooks
	})

	basePath := t.TempDir()
	output := filepath.Join(t.TempDir(), "rendered.yaml")
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{
			BasePath: basePath,
			Components: schema.Components{
				Kubernetes: schema.Kubernetes{BasePath: "components/kubernetes", Provider: ProviderKubectl},
			},
		}, nil
	}
	processStacks = func(
		atmosConfig *schema.AtmosConfiguration,
		info schema.ConfigAndStacksInfo,
		checkStack bool,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		authManager auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error) {
		info.ComponentIsEnabled = true
		info.FinalComponent = "api"
		info.ComponentSection = map[string]any{
			"provider": ProviderKubectl,
			"manifests": []any{map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "settings",
				},
			}},
		}
		info.StackSection = map[string]any{}
		return info, nil
	}
	provisionAndResolveComponentPath = func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
		return filepath.Join(basePath, "components", "kubernetes", "api"), false, nil
	}
	dependenciesForComponent = func(atmosConfig *schema.AtmosConfiguration, componentType string, stackSection map[string]any, componentSection map[string]any) (*dependencies.ToolchainEnvironment, error) {
		return dependencies.NewEnvironmentFromDeps(atmosConfig, nil)
	}
	getHooks = func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}

	err := Execute(&component.ExecutionContext{
		Flags: map[string]any{"output": output},
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "api",
		},
	}, OperationRender)

	require.NoError(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kind: ConfigMap")
}

type kubernetesCIHookCall struct {
	event  hooks.HookEvent
	result *schema.KubernetesCIResult
	err    error
}

func TestRunWithHooksEmitsCISummaryWhenBeforeHookFails(t *testing.T) {
	originalGetHooks := getHooks
	originalRunAllHooks := runAllHooks
	originalRunKubernetesCIHook := runKubernetesCIHookFunc
	t.Cleanup(func() {
		getHooks = originalGetHooks
		runAllHooks = originalRunAllHooks
		runKubernetesCIHookFunc = originalRunKubernetesCIHook
	})

	beforeErr := errors.New("before hook failed")
	var events []hooks.HookEvent
	var ciCalls []kubernetesCIHookCall

	getHooks = func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	runAllHooks = func(hookSet *hooks.Hooks, event hooks.HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
		events = append(events, event)
		if event == hooks.BeforeKubernetesPlan {
			return beforeErr
		}
		return nil
	}
	runKubernetesCIHookFunc = func(
		event hooks.HookEvent,
		flags map[string]any,
		atmosConfig *schema.AtmosConfiguration,
		info *schema.ConfigAndStacksInfo,
		result *schema.KubernetesCIResult,
		commandErr error,
	) {
		ciCalls = append(ciCalls, kubernetesCIHookCall{event: event, result: result, err: commandErr})
	}

	err := runWithHooks(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{
			ComponentFromArg: "api",
			Stack:            "dev",
			SubCommand:       "plan",
		},
		OperationDiff,
		manifestSource{},
	)

	require.ErrorIs(t, err, beforeErr)
	assert.Equal(t, []hooks.HookEvent{hooks.BeforeKubernetesPlan}, events)
	require.Len(t, ciCalls, 1)
	assert.Equal(t, hooks.AfterKubernetesPlan, ciCalls[0].event)
	assert.Nil(t, ciCalls[0].result)
	assert.ErrorIs(t, ciCalls[0].err, beforeErr)
}

func TestRunWithHooksEmitsCISummaryWhenAfterHookFails(t *testing.T) {
	originalGetHooks := getHooks
	originalRunAllHooks := runAllHooks
	originalRunKubernetesCIHook := runKubernetesCIHookFunc
	t.Cleanup(func() {
		getHooks = originalGetHooks
		runAllHooks = originalRunAllHooks
		runKubernetesCIHookFunc = originalRunKubernetesCIHook
	})

	afterErr := errors.New("after hook failed")
	var events []hooks.HookEvent
	var ciCalls []kubernetesCIHookCall

	getHooks = func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	runAllHooks = func(hookSet *hooks.Hooks, event hooks.HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
		events = append(events, event)
		if event == hooks.AfterKubernetesRender {
			return afterErr
		}
		return nil
	}
	runKubernetesCIHookFunc = func(
		event hooks.HookEvent,
		flags map[string]any,
		atmosConfig *schema.AtmosConfiguration,
		info *schema.ConfigAndStacksInfo,
		result *schema.KubernetesCIResult,
		commandErr error,
	) {
		ciCalls = append(ciCalls, kubernetesCIHookCall{event: event, result: result, err: commandErr})
	}

	output := filepath.Join(t.TempDir(), "rendered.yaml")
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "api",
		Stack:            "dev",
		SubCommand:       "render",
		ComponentSection: map[string]any{
			"manifests": []any{map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "settings",
				},
			}},
		},
	}

	err := runWithHooks(
		&component.ExecutionContext{Flags: map[string]any{"output": output}},
		&schema.AtmosConfiguration{},
		info,
		OperationRender,
		manifestSource{provider: ProviderKubectl},
	)

	require.ErrorIs(t, err, afterErr)
	assert.Equal(t, []hooks.HookEvent{hooks.BeforeKubernetesRender, hooks.AfterKubernetesRender}, events)
	require.Len(t, ciCalls, 1)
	assert.Equal(t, hooks.AfterKubernetesRender, ciCalls[0].event)
	require.NotNil(t, ciCalls[0].result)
	assert.Equal(t, map[string]int{"rendered": 1}, ciCalls[0].result.ActionCounts)
	assert.ErrorIs(t, ciCalls[0].err, afterErr)
}

func TestRunOperationApplyGateRejectsInvalidManifest(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })
	newKubernetesSDKClient = func() (*sdkClient, error) {
		t.Fatal("apply gate must reject invalid manifests before contacting the cluster")
		return nil, nil
	}

	objects := []*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "", "")}
	result, err := runOperation(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		OperationApply,
		objects,
	)
	require.ErrorIs(t, err, errUtils.ErrKubernetesValidationFailed)
	assert.Equal(t, 1, result.ObjectsTotal)
}

func TestRunOperationValidateDispatches(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })
	newKubernetesSDKClient = func() (*sdkClient, error) {
		t.Fatal("offline validate must not contact the cluster")
		return nil, nil
	}

	objects := []*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")}
	result, err := runOperation(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		OperationValidate,
		objects,
	)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"valid": 1}, result.ActionCounts)
}

func TestRunOperationValidateReportsInvalidCount(t *testing.T) {
	objects := []*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "", "")}

	result, err := runOperation(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		OperationValidate,
		objects,
	)

	require.ErrorIs(t, err, errUtils.ErrKubernetesValidationFailed)
	assert.Equal(t, map[string]int{"invalid": 1}, result.ActionCounts)
}

func TestRunOperationRenderDispatches(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })
	newKubernetesSDKClient = func() (*sdkClient, error) {
		t.Fatal("render must not contact the cluster")
		return nil, nil
	}

	objects := []*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")}
	output := captureKubernetesStdout(t, func() {
		result, err := runOperation(
			&component.ExecutionContext{},
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
			OperationRender,
			objects,
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]int{"rendered": 1}, result.ActionCounts)
	})
	assert.Contains(t, output, "kind: ConfigMap")
}

func TestRunOperationDiffDispatchesToClient(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })
	newKubernetesSDKClient = func() (*sdkClient, error) {
		return nil, errors.New("diff client failed")
	}

	_, err := runOperation(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		OperationDiff,
		[]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")},
	)
	require.ErrorContains(t, err, "diff client failed")
}

func TestRunOperationDeleteDispatchesToClient(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	newKubernetesSDKClient = func() (*sdkClient, error) {
		return newFakeSDKClient(object.DeepCopy()), nil
	}

	output := captureKubernetesStderr(t, func() {
		result, err := runOperation(
			&component.ExecutionContext{},
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{},
			OperationDelete,
			[]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")},
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]int{"deleted": 1}, result.ActionCounts)
	})
	assert.Contains(t, ansi.Strip(output), "deleted v1/ConfigMap default/settings")
}

func TestRunOperationRejectsUnsupportedOperation(t *testing.T) {
	_, err := runOperation(
		&component.ExecutionContext{},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		Operation("frobnicate"),
		nil,
	)
	require.ErrorIs(t, err, errUtils.ErrKubernetesUnsupportedOperation)
}

func TestValidateAndResolveComponentRejectsInvalidProviderType(t *testing.T) {
	// A non-string provider fails ValidateComponent before any path resolution.
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"provider": 42}}
	_, err := validateAndResolveComponent(&schema.AtmosConfiguration{}, info)
	require.ErrorIs(t, err, errUtils.ErrComponentValidationFailed)
}

func TestValidateAndResolveComponentRejectsUnknownProvider(t *testing.T) {
	// ValidateComponent passes (no component-level provider), but the global
	// provider resolves to an unsupported value.
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Kubernetes.Provider = "helm"
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}

	_, err := validateAndResolveComponent(atmosConfig, info)
	require.ErrorIs(t, err, errUtils.ErrComponentValidationFailed)
	require.ErrorContains(t, err, "provider must be")
}

func TestValidateAndResolveComponentResolvesValidComponent(t *testing.T) {
	original := provisionAndResolveComponentPath
	t.Cleanup(func() { provisionAndResolveComponentPath = original })

	componentDir := t.TempDir()
	provisionAndResolveComponentPath = func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
		return componentDir, true, nil
	}

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	atmosConfig.Components.Kubernetes.BasePath = filepath.Join("components", "kubernetes")
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "api",
		ComponentSection: map[string]any{"provider": ProviderKubectl},
	}

	source, err := validateAndResolveComponent(atmosConfig, info)
	require.NoError(t, err)
	assert.Equal(t, ProviderKubectl, source.provider)
	assert.Equal(t, componentDir, source.componentPath)
}

func TestValidateAndResolveComponentReturnsResolvePathError(t *testing.T) {
	original := provisionAndResolveComponentPath
	t.Cleanup(func() { provisionAndResolveComponentPath = original })
	provisionAndResolveComponentPath = func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
		return "", false, errors.New("provision failed")
	}

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	atmosConfig.Components.Kubernetes.BasePath = filepath.Join("components", "kubernetes")
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "api",
		ComponentSection: map[string]any{"provider": ProviderKubectl},
	}

	_, err := validateAndResolveComponent(atmosConfig, info)
	require.ErrorContains(t, err, "provision failed")
}

func TestValidateAndResolveComponentRequiresInputSource(t *testing.T) {
	original := provisionAndResolveComponentPath
	t.Cleanup(func() { provisionAndResolveComponentPath = original })
	// The resolved path does not exist and no manifests/paths are configured, so
	// ensureComponentInputExists must reject the component as missing input.
	missingPath := filepath.Join(t.TempDir(), "missing")
	provisionAndResolveComponentPath = func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
		return missingPath, false, nil
	}

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	atmosConfig.Components.Kubernetes.BasePath = filepath.Join("components", "kubernetes")
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "api",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{"provider": ProviderKubectl},
	}

	_, err := validateAndResolveComponent(atmosConfig, info)
	require.ErrorIs(t, err, errUtils.ErrInvalidComponent)
}

func TestMaybeAutoGenerateFilesGeneratesConfiguredFiles(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Kubernetes.AutoGenerateFiles = true

	componentPath := filepath.Join(t.TempDir(), "generated")
	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"generate": map[string]any{
				// String content is rendered as a Go template and written verbatim.
				"manifest.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n",
			},
		},
	}

	require.NoError(t, maybeAutoGenerateFiles(atmosConfig, info, componentPath))

	written, err := os.ReadFile(filepath.Join(componentPath, "manifest.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "kind: ConfigMap")
}

func TestLoadManifestObjectsErrorsWhenNoManifestsFound(t *testing.T) {
	// An empty component directory yields no objects, which must be reported as
	// an invalid component rather than succeeding with an empty set.
	source := manifestSource{provider: ProviderKubectl, componentPath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "api", ComponentSection: map[string]any{}}

	_, err := loadManifestObjects(source, info)
	require.ErrorIs(t, err, errUtils.ErrInvalidComponent)
	require.ErrorContains(t, err, "no Kubernetes manifests found")
}

func TestRunApplyAndDiffPrintResultsOnSuccess(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	object.Object["data"] = map[string]any{"key": "value"}
	newKubernetesSDKClient = func() (*sdkClient, error) {
		client, fakeClient := newFakeSDKClientWithFake(object.DeepCopy())
		prependApplyDryRunReactor(fakeClient, object.DeepCopy())
		return client, nil
	}

	applyOut := captureKubernetesStderr(t, func() {
		_, err := runApply([]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")})
		require.NoError(t, err)
	})
	assert.Contains(t, ansi.Strip(applyOut), "applied v1/ConfigMap default/settings")

	diffOut := captureKubernetesStderr(t, func() {
		_, err := runDiff([]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")})
		require.NoError(t, err)
	})
	assert.Contains(t, ansi.Strip(diffOut), "v1/ConfigMap default/settings")
}

func TestObjectCIResultsCarriesDiff(t *testing.T) {
	in := []objectResult{
		{Action: "changed", Resource: "v1/ConfigMap", Namespace: "default", Name: "settings", Diff: "-old\n+new\n"},
		{Action: "no-change", Resource: "v1/ConfigMap", Namespace: "default", Name: "other"},
	}

	out := objectCIResults(in)

	require.Len(t, out, 2)
	assert.Equal(t, "-old\n+new\n", out[0].Diff)
	assert.Empty(t, out[1].Diff)
}

func TestPrintResultsEmitsDiffBodyWhenPresent(t *testing.T) {
	out := captureKubernetesStdout(t, func() {
		printResults([]objectResult{
			{Action: "changed", Resource: "v1/ConfigMap", Namespace: "default", Name: "settings", Diff: "-  key: old\n+  key: new"},
		})
	})

	assert.Contains(t, out, "changed v1/ConfigMap default/settings")
	assert.Contains(t, out, "-  key: old")
	assert.Contains(t, out, "+  key: new")
}

func TestPrintResultsUsesUIForStatusWithoutDiff(t *testing.T) {
	out := captureKubernetesStderr(t, func() {
		printResults([]objectResult{
			{Action: "applied", Resource: "v1/ConfigMap", Namespace: "default", Name: "settings"},
		})
	})

	assert.Contains(t, ansi.Strip(out), "applied v1/ConfigMap default/settings")
}

func TestEventsFor(t *testing.T) {
	tests := []struct {
		operation Operation
		before    hooks.HookEvent
		after     hooks.HookEvent
	}{
		{OperationRender, hooks.BeforeKubernetesRender, hooks.AfterKubernetesRender},
		{OperationDiff, hooks.BeforeKubernetesDiff, hooks.AfterKubernetesDiff},
		{OperationApply, hooks.BeforeKubernetesApply, hooks.AfterKubernetesApply},
		{OperationDelete, hooks.BeforeKubernetesDelete, hooks.AfterKubernetesDelete},
		{OperationValidate, hooks.BeforeKubernetesValidate, hooks.AfterKubernetesValidate},
		{Operation("unknown"), hooks.HookEvent(""), hooks.HookEvent("")},
	}

	for _, tt := range tests {
		before, after := eventsFor(tt.operation)
		assert.Equal(t, tt.before, before)
		assert.Equal(t, tt.after, after)
	}
}

func TestEventsForCommandPreservesPlanAndDeploy(t *testing.T) {
	before, after := eventsForCommand("plan", OperationDiff)
	assert.Equal(t, hooks.BeforeKubernetesPlan, before)
	assert.Equal(t, hooks.AfterKubernetesPlan, after)

	before, after = eventsForCommand("deploy", OperationApply)
	assert.Equal(t, hooks.BeforeKubernetesDeploy, before)
	assert.Equal(t, hooks.AfterKubernetesDeploy, after)
}

func TestApplyEnvironmentSetsAndRestoresValues(t *testing.T) {
	t.Setenv("EXISTING_ENV", "original")
	require.NoError(t, os.Unsetenv("NEW_ENV"))
	require.NoError(t, os.Unsetenv("TOOL_ENV"))

	restore := applyEnvironment(
		map[string]any{"EXISTING_ENV": "override", "NEW_ENV": 42},
		[]string{"TOOL_ENV=enabled", "INVALID"},
	)

	assert.Equal(t, "override", os.Getenv("EXISTING_ENV"))
	assert.Equal(t, "42", os.Getenv("NEW_ENV"))
	assert.Equal(t, "enabled", os.Getenv("TOOL_ENV"))

	restore()

	assert.Equal(t, "original", os.Getenv("EXISTING_ENV"))
	assert.Empty(t, os.Getenv("NEW_ENV"))
	assert.Empty(t, os.Getenv("TOOL_ENV"))
}

func TestPrintResults(t *testing.T) {
	output := captureKubernetesStderr(t, func() {
		printResults([]objectResult{
			{Action: "applied", Resource: "v1/Namespace", Name: "demo"},
			{Action: "changed", Resource: "apps/v1/Deployment", Namespace: "default", Name: "api"},
		})
	})

	plainOutput := ansi.Strip(output)
	assert.Contains(t, plainOutput, "applied v1/Namespace demo")
	assert.Contains(t, plainOutput, "changed apps/v1/Deployment default/api")
}

func TestRunOperationsUseSDKClientFactory(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	newKubernetesSDKClient = func() (*sdkClient, error) {
		return nil, errors.New("client failed")
	}
	_, err := runApply(nil)
	require.ErrorContains(t, err, "client failed")
	_, err = runDelete(nil)
	require.ErrorContains(t, err, "client failed")
	_, err = runDiff(nil)
	require.ErrorContains(t, err, "client failed")

	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	newKubernetesSDKClient = func() (*sdkClient, error) {
		return newFakeSDKClient(object.DeepCopy()), nil
	}
	_, err = runDelete([]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")})
	require.NoError(t, err)
	_, err = runApply([]*unstructured.Unstructured{object})
	require.ErrorContains(t, err, "failed to apply")
	_, err = runDiff([]*unstructured.Unstructured{object})
	require.ErrorContains(t, err, "server dry-run apply")
}

type kubernetesTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *kubernetesTestStreams) Input() stdio.Reader     { return s.stdin }
func (s *kubernetesTestStreams) Output() stdio.Writer    { return s.stdout }
func (s *kubernetesTestStreams) Error() stdio.Writer     { return s.stderr }
func (s *kubernetesTestStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *kubernetesTestStreams) RawError() stdio.Writer  { return s.stderr }

func captureKubernetesStdout(t *testing.T, fn func()) string {
	t.Helper()

	stdout, _ := captureKubernetesOutput(t, fn)
	return stdout
}

func captureKubernetesStderr(t *testing.T, fn func()) string {
	t.Helper()

	_, stderr := captureKubernetesOutput(t, fn)
	return stderr
}

func captureKubernetesOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	streams := &kubernetesTestStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		initKubernetesTestOutput(t)
	})

	fn()

	return stdout.String(), stderr.String()
}

func initKubernetesTestOutput(t *testing.T) {
	t.Helper()

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

func stubAffectedExecutionFailures(t *testing.T) func() {
	t.Helper()

	originalRepoPath := executeAffectedWithRepoPath
	originalClone := executeAffectedWithRefClone
	originalCheckout := executeAffectedWithRefCheckout

	unexpected := func(name string) error {
		t.Helper()
		return errors.New("unexpected affected execution path: " + name)
	}

	executeAffectedWithRepoPath = func(
		*schema.AtmosConfiguration,
		string,
		bool,
		bool,
		string,
		bool,
		bool,
		[]string,
		bool,
		auth.AuthManager,
		bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return nil, nil, nil, "", unexpected("repo-path")
	}
	executeAffectedWithRefClone = func(
		*schema.AtmosConfiguration,
		string,
		string,
		string,
		string,
		bool,
		bool,
		string,
		bool,
		bool,
		[]string,
		bool,
		auth.AuthManager,
		bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return nil, nil, nil, "", unexpected("clone")
	}
	executeAffectedWithRefCheckout = func(
		*schema.AtmosConfiguration,
		string,
		string,
		string,
		bool,
		bool,
		string,
		bool,
		bool,
		[]string,
		bool,
		auth.AuthManager,
		bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return nil, nil, nil, "", unexpected("checkout")
	}

	return func() {
		executeAffectedWithRepoPath = originalRepoPath
		executeAffectedWithRefClone = originalClone
		executeAffectedWithRefCheckout = originalCheckout
	}
}
