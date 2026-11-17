package helm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helmchart "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	rcommon "helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"

	"github.com/cloudposse/atmos/pkg/schema"
)

const planTestCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  scope: Namespaced
  names:
    kind: Widget
    plural: widgets
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`

func crdPlanSpec(t *testing.T, managed bool) *chartSpec {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "crds"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("apiVersion: v2\nname: demo\nversion: 0.1.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crds", "widgets.yaml"), []byte(planTestCRD), 0o600))
	if managed {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "templates", "managed.yaml"), []byte(strings.ReplaceAll(planTestCRD, "widgets", "managedwidgets")), 0o600))
	}
	return &chartSpec{Chart: dir, ReleaseName: "demo", Namespace: "default", IncludeCRDs: true}
}

func TestCRDPlanExistingRelease(t *testing.T) {
	for _, managed := range []bool{false, true} {
		name := "CRD-only release"
		if managed {
			name = "managed template CRD"
		}
		t.Run(name, func(t *testing.T) {
			spec := crdPlanSpec(t, managed)
			storedSpec := *spec
			storedSpec.IncludeCRDs = false
			baseline, err := renderManifest(context.Background(), &storedSpec)
			require.NoError(t, err)
			actx := memoryActionContext(t)
			stubActionContext(t, actx)
			require.NoError(t, actx.cfg.Releases.Create(&release.Release{Name: "demo", Namespace: "default", Version: 1, Manifest: baseline, Info: &release.Info{Status: rcommon.StatusDeployed}}))
			old, prepared, err := prepareDiff(context.Background(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, spec)
			require.NoError(t, err)
			require.False(t, prepared.IncludeCRDs)
			require.Nil(t, prepared.planCRDs)
			require.True(t, spec.IncludeCRDs, "do not mutate the shared spec")
			desired, err := renderManifest(context.Background(), prepared)
			require.NoError(t, err)
			_, changed, err := unifiedDiff(old, desired, "default", 3)
			require.NoError(t, err)
			assert.False(t, changed)
			if managed {
				require.NoError(t, os.WriteFile(filepath.Join(spec.Chart, "templates", "managed.yaml"), []byte(strings.ReplaceAll(strings.ReplaceAll(planTestCRD, "widgets", "managedwidgets"), "scope: Namespaced", "scope: Cluster")), 0o600))
				desired, err = renderManifest(context.Background(), prepared)
				require.NoError(t, err)
				diff, changed, err := unifiedDiff(old, desired, "default", 3)
				require.NoError(t, err)
				assert.True(t, changed)
				assert.Contains(t, diff, "scope: Cluster")
			}
		})
	}
}

func TestCRDPlanNewReleaseAndOffline(t *testing.T) {
	spec := crdPlanSpec(t, false)
	stubActionContext(t, memoryActionContext(t))
	_, prepared, err := prepareDiff(context.Background(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, spec)
	require.NoError(t, err)
	require.NotNil(t, prepared.planCRDs)
	require.False(t, prepared.IncludeCRDs)
	for _, found := range []bool{false, true} {
		prepared.planCRDs = func(ctx context.Context, chart helmchart.Charter) (string, error) {
			return missingCRDManifests(ctx, chart, func(context.Context, string) (bool, error) { return found, nil })
		}
		rendered, err := renderManifest(context.Background(), prepared)
		require.NoError(t, err)
		assert.Equal(t, !found, strings.Contains(rendered, "widgets.example.com"))
	}
	baselineFile := filepath.Join(t.TempDir(), "baseline.yaml")
	require.NoError(t, os.WriteFile(baselineFile, []byte(""), 0o600))
	_, prepared, err = prepareDiff(context.Background(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, map[string]any{flagFromManifest: baselineFile, flagAgainst: againstRelease}, spec)
	require.NoError(t, err)
	require.True(t, prepared.IncludeCRDs)
	require.Nil(t, prepared.planCRDs)
	rendered, err := renderManifest(context.Background(), prepared)
	require.NoError(t, err)
	assert.Contains(t, rendered, "widgets.example.com")
}

func TestMissingCRDManifests(t *testing.T) {
	chart := &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "parent"}}
	child := &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "child"}, Files: []*common.File{{Name: "crds/widgets.yaml", Data: []byte(planTestCRD + "\n---\n" + strings.ReplaceAll(planTestCRD, "widgets", "gadgets"))}}}
	chart.AddDependency(child)
	var names []string
	rendered, err := missingCRDManifests(context.Background(), chart, func(_ context.Context, name string) (bool, error) {
		names = append(names, name)
		return name == "widgets.example.com", nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"widgets.example.com", "gadgets.example.com"}, names)
	assert.NotContains(t, rendered, "widgets.example.com")
	assert.Contains(t, rendered, "gadgets.example.com")
	denied := errors.New("forbidden")
	_, err = missingCRDManifests(context.Background(), chart, func(context.Context, string) (bool, error) { return false, denied })
	require.ErrorIs(t, err, denied)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = missingCRDManifests(ctx, chart, func(context.Context, string) (bool, error) { t.Fatal("unexpected lookup"); return false, nil })
	require.ErrorIs(t, err, context.Canceled)
}

func TestCRDExistenceLookup(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				if code == http.StatusOK {
					_, _ = fmt.Fprint(w, `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"widgets.example.com"}}`)
				} else {
					_, _ = fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","code":%d}`, code)
				}
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "kubeconfig")
			require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
contexts:
- name: test
  context:
    cluster: test
current-context: test
`, server.URL)), 0o600))
			t.Setenv("KUBECONFIG", path)
			lookup := newCRDExistenceLookup(&chartSpec{Namespace: "default"})
			found, err := lookup(context.Background(), "widgets.example.com")
			if code == http.StatusOK || code == http.StatusNotFound {
				require.NoError(t, err)
				assert.Equal(t, code == http.StatusOK, found)
			} else {
				require.Error(t, err)
				assert.False(t, found)
			}
		})
	}
}

func TestCRDPlanRespectsSubchartCondition(t *testing.T) {
	spec := crdPlanSpec(t, false)
	child := filepath.Join(spec.Chart, "charts", "child")
	require.NoError(t, os.MkdirAll(filepath.Join(child, "crds"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(child, "Chart.yaml"), []byte("apiVersion: v2\nname: child\nversion: 0.1.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(child, "crds", "widgets.yaml"), []byte(strings.ReplaceAll(planTestCRD, "widgets", "childwidgets")), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(spec.Chart, "Chart.yaml"), []byte("apiVersion: v2\nname: demo\nversion: 0.1.0\ndependencies:\n- name: child\n  version: 0.1.0\n  condition: child.enabled\n"), 0o600))
	stubActionContext(t, memoryActionContext(t))
	for _, enabled := range []bool{false, true} {
		spec.Values = map[string]any{"child": map[string]any{"enabled": enabled}}
		_, prepared, err := prepareDiff(context.Background(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, spec)
		require.NoError(t, err)
		prepared.planCRDs = func(ctx context.Context, chart helmchart.Charter) (string, error) {
			return missingCRDManifests(ctx, chart, func(context.Context, string) (bool, error) { return false, nil })
		}
		rendered, err := renderManifest(context.Background(), prepared)
		require.NoError(t, err)
		assert.Equal(t, enabled, strings.Contains(rendered, "childwidgets.example.com"))
	}
}

func TestMissingCRDList(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			items := "apiVersion: v1\nkind: List\nitems:\n"
			for _, name := range []string{"widgets", "gadgets"} {
				items += "- " + strings.ReplaceAll(strings.TrimSpace(strings.ReplaceAll(planTestCRD, "widgets", name)), "\n", "\n  ") + "\n"
			}
			if nested {
				items = "apiVersion: v1\nkind: List\nitems:\n- " + strings.ReplaceAll(strings.TrimSpace(items), "\n", "\n  ") + "\n"
			}
			chart := &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "lists"}, Files: []*common.File{{Name: "crds/list.yaml", Data: []byte(items)}}}
			var names []string
			rendered, err := missingCRDManifests(context.Background(), chart, func(_ context.Context, name string) (bool, error) {
				names = append(names, name)
				return name == "widgets.example.com", nil
			})
			require.NoError(t, err)
			assert.Equal(t, []string{"widgets.example.com", "gadgets.example.com"}, names)
			assert.NotContains(t, rendered, "widgets.example.com")
			assert.Contains(t, rendered, "gadgets.example.com")
			assert.NotContains(t, rendered, `"kind":"List"`)
		})
	}
}
