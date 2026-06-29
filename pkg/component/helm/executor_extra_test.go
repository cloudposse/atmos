package helm

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	release "helm.sh/helm/v4/pkg/release/v1"
)

func TestExecuteSingle_HappyPath(t *testing.T) {
	originalInit := initCliConfig
	originalProcess := processStacks
	originalProvision := provisionAndResolveComponentPath
	originalDeps := dependenciesForComponent
	originalHooks := getHooks
	originalCI := runCIHooks
	originalDelete := deleteHelmRelease
	t.Cleanup(func() {
		initCliConfig = originalInit
		processStacks = originalProcess
		provisionAndResolveComponentPath = originalProvision
		dependenciesForComponent = originalDeps
		getHooks = originalHooks
		runCIHooks = originalCI
		deleteHelmRelease = originalDelete
	})

	initCliConfig = func(info schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.ComponentIsEnabled = true
		info.ComponentFromArg = "apps/app"
		info.FinalComponent = "app"
		info.ComponentSection = map[string]any{"chart": "bitnami/nginx", "name": "app", "namespace": "demo"}
		return info, nil
	}
	provisionAndResolveComponentPath = func(_ context.Context, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ string, fallback string) (string, bool, error) {
		return fallback, true, nil
	}
	dependenciesForComponent = func(*schema.AtmosConfiguration, string, map[string]any, map[string]any) (*dependencies.ToolchainEnvironment, error) {
		return &dependencies.ToolchainEnvironment{}, nil
	}
	getHooks = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	runCIHooks = func(*hooks.RunCIHooksOptions) error { return nil }
	var deleted string
	deleteHelmRelease = func(releaseName, _ string) error {
		deleted = releaseName
		return nil
	}

	err := Execute(&component.ExecutionContext{
		SubCommand: "delete",
		Flags:      map[string]any{},
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{
			ComponentFromArg: "apps/app",
			Stack:            "dev",
		},
	}, OperationDelete)
	require.NoError(t, err)
	assert.Equal(t, "app", deleted)
}

func TestRunHelmCIHook(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	calls := 0
	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		calls++
		captured = opts
		return nil
	}

	// An empty event short-circuits without invoking the CI hook.
	runHelmCIHook(helmCIHookParams{ctx: &component.ExecutionContext{Flags: map[string]any{}}, event: ""})
	assert.Zero(t, calls)

	// A real event invokes the hook; a nil summary defaults to an empty map.
	runHelmCIHook(helmCIHookParams{
		ctx:         &component.ExecutionContext{Flags: map[string]any{"ci": true}},
		atmosConfig: &schema.AtmosConfiguration{},
		info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "app"},
		event:       hooks.AfterHelmApply,
	})
	require.Equal(t, 1, calls)
	require.NotNil(t, captured)
	assert.Equal(t, hooks.AfterHelmApply, captured.Event)
	assert.True(t, captured.ForceCIMode)
	assert.NotNil(t, captured.Aggregate)

	// An error from the CI hook is swallowed (logged), never propagated.
	runCIHooks = func(*hooks.RunCIHooksOptions) error { return errors.New("hook failed") }
	runHelmCIHook(helmCIHookParams{
		ctx:   &component.ExecutionContext{Flags: map[string]any{}},
		info:  &schema.ConfigAndStacksInfo{},
		event: hooks.AfterHelmApply,
	})
}

func TestRunWithHooks_DeleteSuccess(t *testing.T) {
	originalHooks := getHooks
	originalDelete := deleteHelmRelease
	originalCI := runCIHooks
	t.Cleanup(func() {
		getHooks = originalHooks
		deleteHelmRelease = originalDelete
		runCIHooks = originalCI
	})

	getHooks = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	runCIHooks = func(*hooks.RunCIHooksOptions) error { return nil }
	var deleted string
	deleteHelmRelease = func(releaseName, _ string) error {
		deleted = releaseName
		return nil
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "apps/app",
		SubCommand:       "delete",
		ComponentSection: map[string]any{"chart": "bitnami/nginx", "name": "app"},
	}
	err := runWithHooks(&component.ExecutionContext{Flags: map[string]any{}}, &schema.AtmosConfiguration{}, info, OperationDelete, "")
	require.NoError(t, err)
	assert.Equal(t, "app", deleted)
}

func TestRunWithHooks_GetHooksError(t *testing.T) {
	originalHooks := getHooks
	t.Cleanup(func() { getHooks = originalHooks })

	sentinel := errors.New("hook discovery failed")
	getHooks = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return nil, sentinel
	}

	err := runWithHooks(&component.ExecutionContext{Flags: map[string]any{}}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationDelete, "")
	require.ErrorIs(t, err, sentinel)
}

func TestResolveDiffBaseline_AgainstTarget(t *testing.T) {
	ft := &fakeTarget{fetchArtifact: target.ProvisionArtifact{
		Files: map[string][]byte{"cm.yaml": []byte(baseConfigMap)},
	}}
	registerFakeTarget(t, "diff-against-target", ft)

	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"provision": map[string]any{
			"default": "repo",
			"targets": map[string]any{"repo": map[string]any{"kind": "diff-against-target"}},
		},
	}}
	got, err := resolveDiffBaseline(
		&schema.AtmosConfiguration{},
		info,
		map[string]any{flagAgainst: "target"},
		&chartSpec{ReleaseName: "app", Namespace: "demo"},
	)
	require.NoError(t, err)
	assert.Contains(t, got, "app-config")
	require.NotNil(t, ft.fetched)
	assert.Equal(t, "repo", ft.fetched.TargetName)
}

func TestResolveDiffBaseline_DeployedRelease(t *testing.T) {
	actx := memoryActionContext(t)
	require.NoError(t, actx.cfg.Releases.Create(release.Mock(&release.MockReleaseOptions{
		Name:      "app",
		Namespace: "demo",
	})))
	stubActionContext(t, actx)

	got, err := resolveDiffBaseline(
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		map[string]any{},
		&chartSpec{ReleaseName: "app", Namespace: "demo"},
	)
	require.NoError(t, err)
	assert.Contains(t, got, "kind: Secret")
}

func TestFetchTargetBaseline_RejectsKubernetesTarget(t *testing.T) {
	// No provision section + no name resolves to the implicit cluster target.
	_, err := fetchTargetBaseline(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}, "target")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrHelmDiffFailed)
}

func TestFetchTargetBaseline_SelectTargetError(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"provision": map[string]any{
			"targets": map[string]any{"repo": map[string]any{"kind": "git"}},
		},
	}}
	// "target:nope" requests a named target that is not configured.
	_, err := fetchTargetBaseline(&schema.AtmosConfiguration{}, info, "target:nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
}

func TestFetchTargetBaseline_FetchError(t *testing.T) {
	ft := &fakeTarget{fetchErr: errors.New("fetch boom")}
	registerFakeTarget(t, "diff-fetch-err", ft)
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"provision": map[string]any{
			"default": "repo",
			"targets": map[string]any{"repo": map[string]any{"kind": "diff-fetch-err"}},
		},
	}}
	_, err := fetchTargetBaseline(&schema.AtmosConfiguration{}, info, "target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch boom")
}

func TestResolveComponentPath(t *testing.T) {
	originalProvision := provisionAndResolveComponentPath
	t.Cleanup(func() { provisionAndResolveComponentPath = originalProvision })

	provisionAndResolveComponentPath = func(_ context.Context, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ string, fallback string) (string, bool, error) {
		return fallback, true, nil
	}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Helm.BasePath = "components/helm"
	path, err := resolveComponentPath(atmosConfig, &schema.ConfigAndStacksInfo{FinalComponent: "app"})
	require.NoError(t, err)
	assert.Contains(t, filepath.ToSlash(path), "components/helm")

	sentinel := errors.New("provision failed")
	provisionAndResolveComponentPath = func(context.Context, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
		return "", false, sentinel
	}
	_, err = resolveComponentPath(atmosConfig, &schema.ConfigAndStacksInfo{FinalComponent: "app"})
	require.ErrorIs(t, err, sentinel)
}

func TestMaybeAutoGenerateFiles(t *testing.T) {
	// Auto-generation disabled is a no-op.
	require.NoError(t, maybeAutoGenerateFiles(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, t.TempDir()))

	// Enabled but no generate section is a no-op.
	enabled := &schema.AtmosConfiguration{}
	enabled.Components.Helm.AutoGenerateFiles = true
	require.NoError(t, maybeAutoGenerateFiles(enabled, &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}, t.TempDir()))

	// Enabled with a generate section creates the directory and writes the file.
	componentPath := filepath.Join(t.TempDir(), "comp")
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"generate": map[string]any{"config.txt": "hello"},
	}}
	require.NoError(t, maybeAutoGenerateFiles(enabled, info, componentPath))
	assert.FileExists(t, filepath.Join(componentPath, "config.txt"))
}

func TestRenderObjects_Errors(t *testing.T) {
	originalRender := renderChartManifest
	t.Cleanup(func() { renderChartManifest = originalRender })

	// A render failure propagates.
	renderChartManifest = func(context.Context, *chartSpec) (string, error) {
		return "", errors.New("render boom")
	}
	_, err := renderObjects(&chartSpec{Chart: "demo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render boom")

	// An empty render yields ErrHelmRenderFailed.
	renderChartManifest = func(context.Context, *chartSpec) (string, error) {
		return "", nil
	}
	_, err = renderObjects(&chartSpec{Chart: "demo"})
	require.ErrorIs(t, err, errUtils.ErrHelmRenderFailed)
}

func TestProcessStacksWithAuth(t *testing.T) {
	originalProcess := processStacks
	originalAuth := setupComponentAuthForCLI
	t.Cleanup(func() {
		processStacks = originalProcess
		setupComponentAuthForCLI = originalAuth
	})

	// Without an identity, no auth manager is created and processStacks runs.
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		assert.Nil(t, authManager)
		info.ComponentIsEnabled = true
		return info, nil
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app"}
	require.NoError(t, processStacksWithAuth(&schema.AtmosConfiguration{}, info))
	assert.True(t, info.ComponentIsEnabled)

	// With an identity, a setup failure propagates before processStacks runs.
	sentinel := errors.New("auth failed")
	setupComponentAuthForCLI = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
		return nil, sentinel
	}
	err := processStacksWithAuth(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{Identity: "admin"})
	require.ErrorIs(t, err, sentinel)
}
