package helm

import (
	"context"
	"path/filepath"
	"strings"
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
	tests := []struct {
		name    string
		section map[string]any
		want    bool
	}{
		{name: "absent defaults to true (preserves existing native-helm behavior)", section: map[string]any{}, want: true},
		{name: "explicit true", section: map[string]any{"create_namespace": true}, want: true},
		{name: "explicit false installs into a pre-existing namespace", section: map[string]any{"create_namespace": false}, want: false},
		{name: "wrong type falls back to the default, not false", section: map[string]any{"create_namespace": "false"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveCreateNamespace(tt.section))
		})
	}
}

func TestBoolFieldDefault(t *testing.T) {
	tests := []struct {
		name     string
		section  map[string]any
		key      string
		fallback bool
		want     bool
	}{
		{name: "present true", section: map[string]any{"k": true}, key: "k", fallback: false, want: true},
		{name: "present false", section: map[string]any{"k": false}, key: "k", fallback: true, want: false},
		{name: "absent returns fallback true", section: map[string]any{}, key: "k", fallback: true, want: true},
		{name: "absent returns fallback false", section: map[string]any{}, key: "k", fallback: false, want: false},
		{name: "non-bool value returns fallback (unset must not read as explicit false)", section: map[string]any{"k": "true"}, key: "k", fallback: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, boolFieldDefault(tt.section, tt.key, tt.fallback))
		})
	}
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

// create_namespace controls whether Helm's install path issues a Namespace create.
// With it false, no Namespace object is built or created, so a namespace-scoped
// identity is never asked to create one; with it true, the namespace is created.
// This is verified against a recording kube client, because a plain install would
// "succeed" either way against the in-memory fake — success alone does not prove
// the namespace-create call was skipped.
func TestApplyRelease_CreateNamespaceControlsNamespaceCreate(t *testing.T) {
	chartPath, err := filepath.Abs(filepath.Join("testdata", "chart"))
	require.NoError(t, err)

	tests := []struct {
		name               string
		createNamespace    bool
		wantNamespaceBuilt bool
	}{
		{name: "false skips the namespace create", createNamespace: false, wantNamespaceBuilt: false},
		{name: "true issues the namespace create", createNamespace: true, wantNamespaceBuilt: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actx, rec := recordingActionContext(t)
			stubActionContext(t, actx)

			spec := &chartSpec{
				Chart:           chartPath,
				ReleaseName:     "ns-toggle",
				Namespace:       "preexisting",
				CreateNamespace: tt.createNamespace,
				Values:          map[string]any{"replicaCount": 1, "image": map[string]any{"tag": "1.0"}},
			}

			manifest, err := applyRelease(context.Background(), spec, false)
			require.NoError(t, err)
			assert.Contains(t, manifest, "kind: ConfigMap")
			assert.Contains(t, manifest, "name: ns-toggle")

			namespaceBuilt := false
			for _, doc := range rec.BuiltDocs {
				if strings.Contains(doc, "kind: Namespace") {
					namespaceBuilt = true
					break
				}
			}
			assert.Equal(t, tt.wantNamespaceBuilt, namespaceBuilt,
				"Helm should build and create a Namespace only when create_namespace is true")
		})
	}
}
