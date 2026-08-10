package helm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/hooks"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

const helmExecutorManifest = `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: demo
data:
  value: new
`

func TestRunOperationDispatchesWithSummaries(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	originalRender := renderChartManifest
	originalApply := applyHelmRelease
	originalDelete := deleteHelmRelease
	t.Cleanup(func() {
		renderChartManifest = originalRender
		applyHelmRelease = originalApply
		deleteHelmRelease = originalDelete
	})

	renderChartManifest = func(_ context.Context, _ *chartSpec) (string, error) {
		return helmExecutorManifest, nil
	}
	applyHelmRelease = func(_ context.Context, _ *chartSpec, dryRun bool) (string, error) {
		require.False(t, dryRun)
		return helmExecutorManifest, nil
	}
	var deletedRelease, deletedNamespace string
	deleteHelmRelease = func(releaseName, namespace string) error {
		deletedRelease = releaseName
		deletedNamespace = namespace
		return nil
	}

	spec := &chartSpec{
		Chart:       "demo",
		ReleaseName: "app",
		Namespace:   "demo",
	}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "apps/app",
		Stack:            "dev",
		SubCommand:       "template",
		ComponentSection: map[string]any{},
	}
	ctx := &component.ExecutionContext{Flags: map[string]any{"target": "git"}}

	summary, err := runOperation(ctx, &schema.AtmosConfiguration{}, info, OperationTemplate, spec)
	require.NoError(t, err)
	assert.Equal(t, "apps/app", summary["component"])
	assert.Equal(t, "dev", summary["stack"])
	assert.Equal(t, "git", summary["target"])
	assert.Equal(t, 1, summary["object_count"])
	assert.Equal(t, []string{"ConfigMap"}, summary["object_kinds"])

	diffBase := filepath.Join(t.TempDir(), "base.yaml")
	require.NoError(t, os.WriteFile(diffBase, []byte(baseConfigMap), 0o600))
	info.SubCommand = "diff"
	summary, err = runOperation(
		&component.ExecutionContext{Flags: map[string]any{flagFromManifest: diffBase}},
		&schema.AtmosConfiguration{},
		info,
		OperationDiff,
		spec,
	)
	require.NoError(t, err)
	assert.Contains(t, summary["diff"], "app-config")

	info.SubCommand = "apply"
	summary, err = runOperation(&component.ExecutionContext{Flags: map[string]any{}}, &schema.AtmosConfiguration{}, info, OperationApply, spec)
	require.NoError(t, err)
	assert.Equal(t, "cluster", summary["target"])
	assert.Equal(t, len(helmExecutorManifest), summary["manifest_bytes"])
	assert.Equal(t, 1, summary["object_count"])

	info.SubCommand = "delete"
	summary, err = runOperation(ctx, &schema.AtmosConfiguration{}, info, OperationDelete, spec)
	require.NoError(t, err)
	assert.Equal(t, "app", deletedRelease)
	assert.Equal(t, "demo", deletedNamespace)
	assert.Equal(t, "app", summary["release_name"])
}

func TestRunOperationUnsupported(t *testing.T) {
	summary, err := runOperation(
		&component.ExecutionContext{Flags: map[string]any{}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		Operation("bogus"),
		&chartSpec{},
	)
	require.ErrorIs(t, err, errUtils.ErrHelmUnsupportedOperation)
	assert.NotNil(t, summary)
}

func TestExecuteBulkInitializesConfigAndGraph(t *testing.T) {
	originalInit := initCliConfig
	originalDescribe := executeDescribeStacks
	originalGraph := executeGraph
	t.Cleanup(func() {
		initCliConfig = originalInit
		executeDescribeStacks = originalDescribe
		executeGraph = originalGraph
	})

	var initInfo schema.ConfigAndStacksInfo
	initCliConfig = func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
		initInfo = info
		assert.True(t, processStacks)
		return schema.AtmosConfiguration{}, nil
	}

	stacks := map[string]any{"dev": map[string]any{}}
	executeDescribeStacks = func(
		_ *schema.AtmosConfiguration,
		filterByStack string,
		_ []string,
		componentTypes []string,
		_ []string,
		_, processTemplates, processYamlFunctions, includeEmptyStacks bool,
		_ []string,
		_ auth.AuthManager,
	) (map[string]any, error) {
		assert.Equal(t, "dev", filterByStack)
		assert.Equal(t, []string{cfg.HelmComponentType}, componentTypes)
		assert.True(t, processTemplates)
		assert.True(t, processYamlFunctions)
		assert.True(t, includeEmptyStacks)
		return stacks, nil
	}

	var graphOpts *component.GraphExecutionOptions
	executeGraph = func(_ context.Context, opts *component.GraphExecutionOptions) error {
		graphOpts = opts
		return nil
	}

	ctx := &component.ExecutionContext{
		SubCommand: "template",
		Flags:      map[string]any{"include-dependents": true},
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{
			All:   true,
			Stack: "dev",
		},
	}
	require.NoError(t, Execute(ctx, OperationTemplate))

	assert.Equal(t, cfg.HelmComponentType, initInfo.ComponentType)
	assert.Equal(t, "template", initInfo.SubCommand)
	assert.Equal(t, []string{cfg.HelmComponentType, "template"}, initInfo.CliArgs)
	require.NotNil(t, graphOpts)
	assert.Equal(t, stacks, graphOpts.Stacks)
	assert.Equal(t, cfg.HelmComponentType, graphOpts.ComponentType)
	assert.Equal(t, "template", graphOpts.SubCommand)
	assert.Equal(t, ctx.Flags, graphOpts.Flags)
}

func TestExecuteSingleSkipsDisabledComponent(t *testing.T) {
	originalInit := initCliConfig
	originalProcessStacks := processStacks
	originalDependencies := dependenciesForComponent
	t.Cleanup(func() {
		initCliConfig = originalInit
		processStacks = originalProcessStacks
		dependenciesForComponent = originalDependencies
	})

	initCliConfig = func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
		assert.Equal(t, cfg.HelmComponentType, info.ComponentType)
		assert.True(t, processStacks)
		return schema.AtmosConfiguration{}, nil
	}

	processStacksCalled := false
	processStacks = func(
		_ *schema.AtmosConfiguration,
		info schema.ConfigAndStacksInfo,
		checkStack, processTemplates, processYamlFunctions bool,
		_ []string,
		_ auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error) {
		processStacksCalled = true
		assert.True(t, checkStack)
		assert.True(t, processTemplates)
		assert.True(t, processYamlFunctions)
		info.ComponentIsEnabled = false
		info.ComponentFromArg = "app"
		return info, nil
	}

	dependenciesForComponent = func(_ *schema.AtmosConfiguration, _ string, _ map[string]any, _ map[string]any) (*dependencies.ToolchainEnvironment, error) {
		t.Fatal("dependencies should not be resolved for disabled components")
		return nil, nil
	}

	err := Execute(&component.ExecutionContext{
		SubCommand: "apply",
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{
			ComponentFromArg: "app",
			Stack:            "dev",
		},
	}, OperationApply)
	require.NoError(t, err)
	assert.True(t, processStacksCalled)
}

func TestExecutorHelpers(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		operation Operation
		before    hooks.HookEvent
		after     hooks.HookEvent
	}{
		{"deploy command uses deploy events", "deploy", OperationApply, hooks.BeforeHelmDeploy, hooks.AfterHelmDeploy},
		{"template", "template", OperationTemplate, hooks.BeforeHelmTemplate, hooks.AfterHelmTemplate},
		{"diff", "diff", OperationDiff, hooks.BeforeHelmDiff, hooks.AfterHelmDiff},
		{"apply", "apply", OperationApply, hooks.BeforeHelmApply, hooks.AfterHelmApply},
		{"delete", "delete", OperationDelete, hooks.BeforeHelmDelete, hooks.AfterHelmDelete},
		{"unknown", "unknown", Operation("unknown"), hooks.HookEvent(""), hooks.HookEvent("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := eventsFor(tt.command, tt.operation)
			assert.Equal(t, tt.before, before)
			assert.Equal(t, tt.after, after)
		})
	}

	assert.Equal(t, 9, diffContextFromFlags(map[string]any{flagContext: 9}))
	assert.Zero(t, diffContextFromFlags(map[string]any{flagContext: "9"}))

	atmosConfig := &schema.AtmosConfiguration{}
	normalizeGlobalConfig(atmosConfig)
	assert.Equal(t, DefaultConfig().BasePath, atmosConfig.Components.Helm.BasePath)
	atmosConfig.Components.Helm.BasePath = "charts"
	normalizeGlobalConfig(atmosConfig)
	assert.Equal(t, "charts", atmosConfig.Components.Helm.BasePath)
}

func TestSummaryHelpers(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev", SubCommand: "apply"}
	spec := &chartSpec{Chart: "chart", ReleaseName: "release", Namespace: "ns"}

	summary := helmSummary(info, spec, map[string]any{})
	assert.Equal(t, "kubernetes", summary["target"])
	assert.Equal(t, "release", summary["release_name"])

	summary = helmSummary(info, spec, map[string]any{"target": "git"})
	assert.Equal(t, "git", summary["target"])

	mergeSummary(summary, map[string]any{"target": "gitops", "extra": true})
	assert.Equal(t, "gitops", summary["target"])
	assert.Equal(t, true, summary["extra"])

	addObjectsToSummary(summary, []*unstructured.Unstructured{
		nil,
		{Object: map[string]any{"kind": "Service"}},
		{Object: map[string]any{"kind": "ConfigMap"}},
		{Object: map[string]any{"kind": "Service"}},
		{Object: map[string]any{}},
	})
	assert.Equal(t, 5, summary["object_count"])
	assert.Equal(t, []string{"ConfigMap", "Service"}, summary["object_kinds"])
}

func TestHelmCIModeEnabled(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	assert.True(t, helmCIModeEnabled(map[string]any{"ci": true}))
	assert.False(t, helmCIModeEnabled(map[string]any{"ci": false}))
	assert.False(t, helmCIModeEnabled(map[string]any{}))

	t.Setenv("ATMOS_CI", "1")
	assert.True(t, helmCIModeEnabled(map[string]any{}))

	t.Setenv("ATMOS_CI", "false")
	t.Setenv("CI", "yes")
	assert.True(t, helmCIModeEnabled(map[string]any{}))

	t.Setenv("CI", "0")
	assert.False(t, helmCIModeEnabled(map[string]any{}))
}

func TestApplyEnvironmentRestoresOriginalValues(t *testing.T) {
	t.Setenv("HELM_KEEP", "original")
	require.NoError(t, os.Unsetenv("HELM_NEW"))

	restore := applyEnvironment(
		map[string]any{"HELM_KEEP": "component", "HELM_COMPONENT_ONLY": 42},
		[]string{"HELM_NEW=toolchain", "MALFORMED"},
	)
	assert.Equal(t, "component", os.Getenv("HELM_KEEP"))
	assert.Equal(t, "42", os.Getenv("HELM_COMPONENT_ONLY"))
	assert.Equal(t, "toolchain", os.Getenv("HELM_NEW"))

	restore()
	assert.Equal(t, "original", os.Getenv("HELM_KEEP"))
	assert.Empty(t, os.Getenv("HELM_COMPONENT_ONLY"))
	assert.Empty(t, os.Getenv("HELM_NEW"))
}

func TestBuildChartSpecAndValueHelpers(t *testing.T) {
	dir := t.TempDir()
	chartDir := filepath.Join(dir, "chart")
	require.NoError(t, os.MkdirAll(chartDir, 0o755))
	valuesFile := filepath.Join(dir, "values.yaml")
	require.NoError(t, os.WriteFile(valuesFile, []byte("image:\n  tag: file\nreplicas: 1\n"), 0o600))

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "apps/demo",
		ComponentSection: map[string]any{
			cfg.ChartSectionName:       "chart",
			cfg.ValuesFilesSectionName: []string{"values.yaml"},
			cfg.ValuesSectionName:      map[string]any{"image": map[string]any{"tag": "inline"}},
			cfg.RepositoriesSectionName: []any{
				map[string]any{"name": "bitnami", "url": "https://charts.bitnami.com/bitnami"},
			},
			"repository": "https://example.com/charts",
			"version":    "1.2.3",
			"name":       "demo",
			"namespace":  "apps",
		},
	}

	spec, err := buildChartSpec(&schema.AtmosConfiguration{}, info, dir)
	require.NoError(t, err)
	assert.Equal(t, chartDir, spec.Chart)
	assert.Equal(t, "https://example.com/charts", spec.RepoURL)
	assert.Equal(t, "1.2.3", spec.Version)
	assert.Equal(t, "demo", spec.ReleaseName)
	assert.Equal(t, "apps", spec.Namespace)
	assert.True(t, spec.IncludeCRDs)
	repo, found := findRepository(spec.Repositories, "bitnami")
	require.True(t, found)
	assert.Equal(t, "https://charts.bitnami.com/bitnami", repo.URL)
	assert.Equal(t, "inline", spec.Values["image"].(map[string]any)["tag"])
	assert.Equal(t, float64(1), spec.Values["replicas"])

	assert.Equal(t, []any{"one"}, asAnySlice("one"))
	assert.Equal(t, []any{"one", "two"}, asAnySlice([]string{"one", "two"}))
	assert.Nil(t, asAnySlice(3))
	assert.Equal(t, filepath.Join(dir, "chart"), resolveLocalChart("./chart", dir))
	assert.Equal(t, filepath.Join(dir, "values.yaml"), resolveLocalChart("values.yaml", dir))
	assert.Equal(t, "bitnami/nginx", resolveLocalChart("bitnami/nginx", dir))
}

func TestRenderInputTemplates(t *testing.T) {
	section := map[string]any{
		"name":                     "demo",
		"namespace":                "{{ .name }}-ns",
		cfg.ChartSectionName:       "./{{ .name }}",
		cfg.ValuesFilesSectionName: []string{"{{ .name }}.yaml"},
		cfg.ValuesSectionName: map[string]any{
			"image": "{{ .name }}:1.0",
		},
		cfg.RepositoriesSectionName: []any{
			map[string]any{"name": "{{ .name }}", "url": "https://example.com/{{ .name }}"},
		},
		cfg.RenderSectionName: map[string]any{
			"output": map[string]any{"path": "{{ .name }}.rendered.yaml"},
		},
		cfg.ProvisionSectionName: map[string]any{
			"targets": []any{map[string]any{"name": "{{ .name }}", "kind": "git"}},
		},
		"version":    "1.{{ 2 }}.3",
		"repository": "https://repo.example.com/{{ .name }}",
	}

	require.NoError(t, renderInputTemplates(&schema.AtmosConfiguration{}, section))
	assert.Equal(t, "./demo", section[cfg.ChartSectionName])
	assert.Equal(t, "demo-ns", section["namespace"])
	assert.Equal(t, "1.2.3", section["version"])
	assert.Equal(t, "https://repo.example.com/demo", section["repository"])
	assert.Equal(t, []any{"demo.yaml"}, section[cfg.ValuesFilesSectionName])
	assert.Equal(t, "demo:1.0", section[cfg.ValuesSectionName].(map[string]any)["image"])
	assert.Equal(t, "demo.rendered.yaml", section[cfg.RenderSectionName].(map[string]any)["output"].(map[string]any)["path"])
	assert.Equal(t, "demo", section[cfg.RepositoriesSectionName].([]any)[0].(map[string]any)["name"])
}

func TestBulkAffectedFlagsAndSelection(t *testing.T) {
	args := &e.DescribeAffectedCmdArgs{}
	applyAffectedFlags(args, map[string]any{
		"repo-path":         "/repo",
		"base":              "main",
		"ref":               "feature",
		"sha":               "abc",
		"ssh-key":           "id_rsa",
		"ssh-key-password":  "secret",
		"clone-target-ref":  true,
		"ignored-nonstring": 3,
	})
	assert.Equal(t, "/repo", args.RepoPath)
	assert.Equal(t, "feature", args.Ref)
	assert.Equal(t, "abc", args.SHA)
	assert.Equal(t, "id_rsa", args.SSHKeyPath)
	assert.Equal(t, "secret", args.SSHKeyPassword)
	assert.True(t, args.CloneTargetRef)

	args = &e.DescribeAffectedCmdArgs{}
	applyAffectedBaseFlag(args, map[string]any{"base": "0123456789abcdef0123456789abcdef01234567"})
	assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", args.SHA)
	assert.Empty(t, args.Ref)

	args = &e.DescribeAffectedCmdArgs{}
	applyAffectedBaseFlag(args, map[string]any{"base": "origin/main"})
	assert.Equal(t, "origin/main", args.Ref)
	assert.Empty(t, args.SHA)

	originalAffected := affectedHelmComponentsFunc
	t.Cleanup(func() { affectedHelmComponentsFunc = originalAffected })

	selection, err := graphSelectionForBulk(
		&component.ExecutionContext{Flags: map[string]any{}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
	)
	require.NoError(t, err)
	assert.Nil(t, selection)

	affectedHelmComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return []schema.Affected{
			{Component: "api", Stack: "dev", ComponentType: cfg.HelmComponentType},
			{Component: "web", Stack: "dev", ComponentType: cfg.HelmComponentType, Deleted: true},
			{Component: "db", Stack: "dev", ComponentType: cfg.TerraformComponentType},
		}, nil
	}
	selection, err = graphSelectionForBulk(
		&component.ExecutionContext{Flags: map[string]any{"include-dependents": true}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{Affected: true},
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, []string{component.GraphNodeID("api", "dev")}, selection.NodeIDs)
	assert.True(t, selection.IncludeDependencies)
	assert.True(t, selection.IncludeDependents)

	wantErr := errors.New("affected failed")
	affectedHelmComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return nil, wantErr
	}
	_, err = graphSelectionForBulk(
		&component.ExecutionContext{Flags: map[string]any{}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{Affected: true},
	)
	require.ErrorIs(t, err, wantErr)
}

func TestDispatchAffectedSelectsBaselineSource(t *testing.T) {
	originalRepo := executeAffectedWithRepoPath
	originalClone := executeAffectedWithRefClone
	originalCheckout := executeAffectedWithRefCheckout
	t.Cleanup(func() {
		executeAffectedWithRepoPath = originalRepo
		executeAffectedWithRefClone = originalClone
		executeAffectedWithRefCheckout = originalCheckout
	})

	var called string
	executeAffectedWithRepoPath = func(_ *schema.AtmosConfiguration, repoPath string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		called = "repo:" + repoPath
		return []schema.Affected{{Component: "repo"}}, nil, nil, "", nil
	}
	executeAffectedWithRefClone = func(_ *schema.AtmosConfiguration, ref, sha, sshKeyPath, sshKeyPassword string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		called = "clone:" + ref + ":" + sha + ":" + sshKeyPath + ":" + sshKeyPassword
		return []schema.Affected{{Component: "clone"}}, nil, nil, "", nil
	}
	executeAffectedWithRefCheckout = func(_ *schema.AtmosConfiguration, ref, sha, targetBranch string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		called = "checkout:" + ref + ":" + sha + ":" + targetBranch
		return []schema.Affected{{Component: "checkout"}}, nil, nil, "", nil
	}

	affected, err := dispatchAffected(&schema.AtmosConfiguration{}, &e.DescribeAffectedCmdArgs{RepoPath: "/repo"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "repo:/repo", called)
	assert.Equal(t, "repo", affected[0].Component)

	affected, err = dispatchAffected(&schema.AtmosConfiguration{}, &e.DescribeAffectedCmdArgs{CloneTargetRef: true, Ref: "main", SHA: "abc", SSHKeyPath: "key", SSHKeyPassword: "pw"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "clone:main:abc:key:pw", called)
	assert.Equal(t, "clone", affected[0].Component)

	affected, err = dispatchAffected(&schema.AtmosConfiguration{}, &e.DescribeAffectedCmdArgs{Ref: "main", SHA: "abc", TargetBranch: "target"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "checkout:main:abc:target", called)
	assert.Equal(t, "checkout", affected[0].Component)
}
