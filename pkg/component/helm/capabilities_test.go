package helm

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Charts that gate objects on `.Capabilities.APIVersions.Has` (a ServiceMonitor
// behind the Prometheus Operator CRD, for example) silently drop those objects
// when rendered offline, because a client-side render only knows Helm's built-in
// API versions. `kube_version` and `api_versions` are the native-Helm equivalent
// of `helm template --kube-version/--api-versions`. These tests lock in parsing,
// CLI precedence, propagation into the chart spec, and the rendered effect.

const serviceMonitorAPIVersion = "monitoring.coreos.com/v1/ServiceMonitor"

func TestResolveKubeVersion(t *testing.T) {
	tests := []struct {
		name        string
		section     map[string]any
		wantVersion string
		wantMinor   string
		wantErr     bool
	}{
		{name: "absent keeps the Helm default", section: map[string]any{}},
		{name: "explicit null keeps the Helm default", section: map[string]any{cfg.HelmKubeVersionSectionName: nil}},
		{name: "plain semver", section: map[string]any{cfg.HelmKubeVersionSectionName: "1.37.0"}, wantVersion: "v1.37.0", wantMinor: "37"},
		{name: "v-prefixed semver", section: map[string]any{cfg.HelmKubeVersionSectionName: "v1.30.2"}, wantVersion: "v1.30.2", wantMinor: "30"},
		{name: "distribution suffix is preserved", section: map[string]any{cfg.HelmKubeVersionSectionName: "v1.33.4+k3s1"}, wantVersion: "v1.33.4+k3s1", wantMinor: "33"},
		{name: "unquoted YAML float is rejected, not misread as 1.3", section: map[string]any{cfg.HelmKubeVersionSectionName: 1.30}, wantErr: true},
		{name: "unparseable string is rejected", section: map[string]any{cfg.HelmKubeVersionSectionName: "latest"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveKubeVersion(tt.section)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrHelmKubeVersionInvalid)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if tt.wantVersion == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantVersion, got.Version)
			assert.Equal(t, "1", got.Major)
			assert.Equal(t, tt.wantMinor, got.Minor)
		})
	}
}

func TestApplyCapabilityFlags(t *testing.T) {
	componentVersion, err := parseKubeVersion("1.30.0")
	require.NoError(t, err)

	t.Run("flags override the component settings", func(t *testing.T) {
		spec := &chartSpec{KubeVersion: componentVersion, APIVersions: []string{"example.com/v1"}}
		err := applyCapabilityFlags(spec, map[string]any{
			cfg.HelmKubeVersionSectionName: "1.37.0",
			cfg.HelmAPIVersionsSectionName: []string{serviceMonitorAPIVersion, "autoscaling.k8s.io/v1"},
		})
		require.NoError(t, err)
		require.NotNil(t, spec.KubeVersion)
		assert.Equal(t, "v1.37.0", spec.KubeVersion.Version)
		assert.Equal(t, []string{serviceMonitorAPIVersion, "autoscaling.k8s.io/v1"}, spec.APIVersions,
			"--api-versions replaces the component list (CLI precedence)")
	})

	t.Run("absent flags keep the component settings", func(t *testing.T) {
		spec := &chartSpec{KubeVersion: componentVersion, APIVersions: []string{"example.com/v1"}}
		require.NoError(t, applyCapabilityFlags(spec, map[string]any{}))
		assert.Same(t, componentVersion, spec.KubeVersion)
		assert.Equal(t, []string{"example.com/v1"}, spec.APIVersions)
	})

	t.Run("invalid --kube-version is rejected", func(t *testing.T) {
		spec := &chartSpec{KubeVersion: componentVersion}
		err := applyCapabilityFlags(spec, map[string]any{cfg.HelmKubeVersionSectionName: "not-a-version"})
		require.ErrorIs(t, err, errUtils.ErrHelmKubeVersionInvalid)
		assert.Same(t, componentVersion, spec.KubeVersion)
	})
}

// buildChartSpec must carry the component's kube_version and api_versions
// through to the resolved chart spec.
func TestBuildChartSpec_CapabilitiesPropagate(t *testing.T) {
	section := map[string]any{
		"chart":                        "./chart",
		cfg.HelmKubeVersionSectionName: "1.37.0",
		cfg.HelmAPIVersionsSectionName: []any{"monitoring.coreos.com/v1", serviceMonitorAPIVersion},
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/demo", ComponentSection: section}
	spec, err := buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.NoError(t, err)
	require.NotNil(t, spec.KubeVersion)
	assert.Equal(t, "v1.37.0", spec.KubeVersion.Version)
	assert.Equal(t, []string{"monitoring.coreos.com/v1", serviceMonitorAPIVersion}, spec.APIVersions)

	// Unset -> Helm defaults.
	info = &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/demo", ComponentSection: map[string]any{"chart": "./chart"}}
	spec, err = buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.NoError(t, err)
	assert.Nil(t, spec.KubeVersion)
	assert.Empty(t, spec.APIVersions)

	// Invalid -> error, not a silent fallback.
	section = map[string]any{"chart": "./chart", cfg.HelmKubeVersionSectionName: 1.3}
	info = &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/demo", ComponentSection: section}
	_, err = buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.ErrorIs(t, err, errUtils.ErrHelmKubeVersionInvalid)
}

// The rendered effect: a ServiceMonitor gated on the CRD's API version appears
// only when api_versions declares it, and .Capabilities.KubeVersion reflects
// kube_version. This uses the real Helm SDK client-side render.
func TestRenderManifest_Capabilities(t *testing.T) {
	chartPath, err := filepath.Abs(filepath.Join("testdata", "chart-capabilities"))
	require.NoError(t, err)
	kubeVersion, err := parseKubeVersion("1.37.0")
	require.NoError(t, err)

	tests := []struct {
		name               string
		kubeVersion        bool
		apiVersions        []string
		wantServiceMonitor bool
	}{
		{name: "defaults drop the gated object"},
		{name: "api_versions renders the gated object", apiVersions: []string{"monitoring.coreos.com/v1", serviceMonitorAPIVersion}, wantServiceMonitor: true},
		{name: "unrelated api_versions still drop it", apiVersions: []string{"autoscaling.k8s.io/v1"}},
		{name: "kube_version alone", kubeVersion: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &chartSpec{
				Chart:       chartPath,
				ReleaseName: "caps",
				Namespace:   "testns",
				APIVersions: tt.apiVersions,
			}
			if tt.kubeVersion {
				spec.KubeVersion = kubeVersion
			}
			rendered, err := renderManifest(context.Background(), spec)
			require.NoError(t, err)
			assert.Contains(t, rendered, "kind: ConfigMap")
			if tt.wantServiceMonitor {
				assert.Contains(t, rendered, "kind: ServiceMonitor")
			} else {
				assert.NotContains(t, rendered, "kind: ServiceMonitor")
			}
			if tt.kubeVersion {
				assert.Contains(t, rendered, `kubeVersion: "v1.37.0"`)
			} else {
				assert.NotContains(t, rendered, `kubeVersion: "v1.37.0"`)
			}
		})
	}
}

// End to end through the executor: the component's settings reach the render,
// and explicit CLI flags win over them.
func TestRunWithHooks_TemplateCapabilities(t *testing.T) {
	originalHooks := getHooks
	originalRender := renderChartManifest
	originalCI := runCIHooks
	originalSetup := setupRepositories
	t.Cleanup(func() {
		getHooks = originalHooks
		renderChartManifest = originalRender
		runCIHooks = originalCI
		setupRepositories = originalSetup
	})
	getHooks = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	runCIHooks = func(*hooks.RunCIHooksOptions) error { return nil }
	setupRepositories = func([]chartRepository) error { return nil }
	var renderedSpec *chartSpec
	renderChartManifest = func(_ context.Context, spec *chartSpec) (string, error) {
		renderedSpec = spec
		return helmExecutorManifest, nil
	}

	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{
			ComponentFromArg: "apps/app",
			SubCommand:       "template",
			ComponentSection: map[string]any{
				"chart":                        "./chart",
				"name":                         "app",
				cfg.HelmKubeVersionSectionName: "1.30.0",
				cfg.HelmAPIVersionsSectionName: []any{serviceMonitorAPIVersion},
			},
		}
	}

	t.Run("component settings reach the render", func(t *testing.T) {
		renderedSpec = nil
		err := runWithHooks(&component.ExecutionContext{Flags: map[string]any{}}, &schema.AtmosConfiguration{}, newInfo(), OperationTemplate, "testdata")
		require.NoError(t, err)
		require.NotNil(t, renderedSpec)
		require.NotNil(t, renderedSpec.KubeVersion)
		assert.Equal(t, "v1.30.0", renderedSpec.KubeVersion.Version)
		assert.Equal(t, []string{serviceMonitorAPIVersion}, renderedSpec.APIVersions)
	})

	t.Run("CLI flags override the component", func(t *testing.T) {
		renderedSpec = nil
		err := runWithHooks(&component.ExecutionContext{Flags: map[string]any{
			cfg.HelmKubeVersionSectionName: "1.37.0",
			cfg.HelmAPIVersionsSectionName: []string{"autoscaling.k8s.io/v1"},
		}}, &schema.AtmosConfiguration{}, newInfo(), OperationTemplate, "testdata")
		require.NoError(t, err)
		require.NotNil(t, renderedSpec)
		require.NotNil(t, renderedSpec.KubeVersion)
		assert.Equal(t, "v1.37.0", renderedSpec.KubeVersion.Version)
		assert.Equal(t, []string{"autoscaling.k8s.io/v1"}, renderedSpec.APIVersions)
	})

	t.Run("invalid --kube-version fails before rendering", func(t *testing.T) {
		renderedSpec = nil
		err := runWithHooks(&component.ExecutionContext{Flags: map[string]any{
			cfg.HelmKubeVersionSectionName: "not-a-version",
		}}, &schema.AtmosConfiguration{}, newInfo(), OperationTemplate, "testdata")
		require.ErrorIs(t, err, errUtils.ErrHelmKubeVersionInvalid)
		assert.Nil(t, renderedSpec)
	})
}
