package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
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
		{Operation("unknown"), hooks.HookEvent(""), hooks.HookEvent("")},
	}

	for _, tt := range tests {
		before, after := eventsFor(tt.operation)
		assert.Equal(t, tt.before, before)
		assert.Equal(t, tt.after, after)
	}
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
	output := captureKubernetesStdout(t, func() {
		printResults([]objectResult{
			{Action: "applied", Resource: "v1/Namespace", Name: "demo"},
			{Action: "changed", Resource: "apps/v1/Deployment", Namespace: "default", Name: "api"},
		})
	})

	assert.Equal(t, "applied v1/Namespace demo\nchanged apps/v1/Deployment default/api\n", output)
}

func TestRunOperationsUseSDKClientFactory(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	newKubernetesSDKClient = func() (*sdkClient, error) {
		return nil, errors.New("client failed")
	}
	require.ErrorContains(t, runApply(nil), "client failed")
	require.ErrorContains(t, runDelete(nil), "client failed")
	require.ErrorContains(t, runDiff(nil), "client failed")

	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	newKubernetesSDKClient = func() (*sdkClient, error) {
		return newFakeSDKClient(object.DeepCopy()), nil
	}
	require.NoError(t, runDelete([]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "settings", "")}))
	require.ErrorContains(t, runApply([]*unstructured.Unstructured{object}), "failed to apply")
	require.ErrorContains(t, runDiff([]*unstructured.Unstructured{object}), "failed to server dry-run apply")
}

func captureKubernetesStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = old })

	fn()

	require.NoError(t, writer.Close())
	os.Stdout = old

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, reader)
	require.NoError(t, err)
	return buffer.String()
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
