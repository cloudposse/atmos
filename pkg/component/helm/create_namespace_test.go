package helm

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// A namespace-scoped CI identity cannot create Kubernetes namespaces, so Helm's
// default `--create-namespace` behavior makes the first install fail with a 403.
// The `create_namespace` component setting turns that off so the platform can
// pre-create the namespace and CI installs into it. These tests lock in the
// setting's parsing, propagation into the chart spec, and wiring onto the Helm
// Install action.

func TestResolveCreateNamespace(t *testing.T) {
	// Absent -> defaults to true (Helm's historical behavior, back-compat).
	assert.True(t, resolveCreateNamespace(map[string]any{}))
	// Explicit true.
	assert.True(t, resolveCreateNamespace(map[string]any{"create_namespace": true}))
	// Explicit false -> Helm must not attempt to create the namespace.
	assert.False(t, resolveCreateNamespace(map[string]any{"create_namespace": false}))
	// Wrong type -> falls back to the default rather than treating it as false.
	assert.True(t, resolveCreateNamespace(map[string]any{"create_namespace": "false"}))
}

func TestBoolFieldDefault(t *testing.T) {
	assert.True(t, boolFieldDefault(map[string]any{"k": true}, "k", false))
	assert.False(t, boolFieldDefault(map[string]any{"k": false}, "k", true))
	// Absent key -> fallback.
	assert.True(t, boolFieldDefault(map[string]any{}, "k", true))
	assert.False(t, boolFieldDefault(map[string]any{}, "k", false))
	// Non-bool value -> fallback (an unset key must not read as an explicit false).
	assert.True(t, boolFieldDefault(map[string]any{"k": "true"}, "k", true))
}

// buildChartSpec must carry the component's create_namespace setting through to
// the resolved chart spec.
func TestBuildChartSpec_CreateNamespacePropagates(t *testing.T) {
	base := map[string]any{
		"chart":     "./chart",
		"namespace": "apps",
	}

	// Default (key absent) -> true.
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/demo", ComponentSection: base}
	spec, err := buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.NoError(t, err)
	assert.True(t, spec.CreateNamespace)

	// Explicit false -> false.
	section := map[string]any{"chart": "./chart", "namespace": "apps", "create_namespace": false}
	info = &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/demo", ComponentSection: section}
	spec, err = buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.NoError(t, err)
	assert.False(t, spec.CreateNamespace)
}

// newInstallClient must mirror spec.CreateNamespace onto the Helm Install action
// (and not hardcode it), alongside the release name, namespace, and version.
func TestNewInstallClient_WiresCreateNamespace(t *testing.T) {
	actx := memoryActionContext(t)

	for _, createNS := range []bool{true, false} {
		spec := &chartSpec{
			ReleaseName:     "rel",
			Namespace:       "testns",
			Version:         "1.2.3",
			CreateNamespace: createNS,
		}
		client := newInstallClient(actx, spec, false)
		assert.Equal(t, createNS, client.CreateNamespace)
		assert.Equal(t, "rel", client.ReleaseName)
		assert.Equal(t, "testns", client.Namespace)
		assert.Equal(t, "1.2.3", client.Version)
	}
}

// With create_namespace disabled, the install path must still install the
// release into the (pre-existing) namespace. Exercised end-to-end against the
// in-memory storage driver.
func TestApplyRelease_CreateNamespaceFalseInstalls(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)

	chartPath, err := filepath.Abs(filepath.Join("testdata", "chart"))
	require.NoError(t, err)
	spec := &chartSpec{
		Chart:           chartPath,
		ReleaseName:     "no-create-ns",
		Namespace:       "preexisting",
		CreateNamespace: false,
		Values:          map[string]any{"replicaCount": 1, "image": map[string]any{"tag": "1.0"}},
	}

	manifest, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	assert.Contains(t, manifest, "kind: ConfigMap")
	assert.Contains(t, manifest, "name: no-create-ns")

	deployed, err := getDeployedManifest("no-create-ns", "preexisting")
	require.NoError(t, err)
	assert.Contains(t, deployed, "kind: ConfigMap")
}
