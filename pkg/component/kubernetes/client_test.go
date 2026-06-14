package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResourceID(t *testing.T) {
	assert.Equal(t, "v1/ConfigMap", resourceID(&unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		},
	}))

	assert.Equal(t, "apps/v1/Deployment", resourceID(&unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
		},
	}))
}

func TestObjectsEqualForDiffIgnoresKubernetesManagedFields(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":              "settings",
			"creationTimestamp": "now",
			"generation":        int64(2),
			"managedFields":     []any{map[string]any{"manager": "kubectl"}},
			"resourceVersion":   "10",
			"uid":               "abc",
		},
		"data":   map[string]any{"key": "value"},
		"status": map[string]any{"observedGeneration": int64(2)},
	}}
	dryRun := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "settings",
		},
		"data": map[string]any{"key": "value"},
	}}

	assert.True(t, objectsEqualForDiff(live, dryRun))

	dryRun.Object["data"] = map[string]any{"key": "changed"}
	assert.False(t, objectsEqualForDiff(live, dryRun))
}

func TestNewSDKClientReturnsConfigLoadError(t *testing.T) {
	t.Setenv("KUBECONFIG", "/path/that/does/not/exist")

	client, err := newSDKClient()

	require.Nil(t, client)
	require.ErrorIs(t, err, errUtils.ErrKubernetesClientInit)
	require.ErrorContains(t, err, "client config")
}

func TestResourceForResolvesNamespacedAndClusterResources(t *testing.T) {
	client := newFakeSDKClient()

	configMap := kubernetesObject("v1", "ConfigMap", "settings", "")
	resource, namespace, err := client.resourceFor(configMap)
	require.NoError(t, err)
	assert.NotNil(t, resource)
	assert.Equal(t, "default", namespace)
	assert.Equal(t, "default", configMap.GetNamespace())

	namespaceObject := kubernetesObject("v1", "Namespace", "demo", "")
	resource, namespace, err = client.resourceFor(namespaceObject)
	require.NoError(t, err)
	assert.NotNil(t, resource)
	assert.Empty(t, namespace)
	assert.Empty(t, namespaceObject.GetNamespace())
}

func TestResourceForErrors(t *testing.T) {
	client := newFakeSDKClient()

	_, _, err := client.resourceFor(kubernetesObject("v1", "ConfigMap", "", ""))
	require.ErrorContains(t, err, "is missing metadata.name")

	_, _, err = client.resourceFor(&unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "settings"},
	}})
	require.ErrorContains(t, err, "missing group/version/kind")

	_, _, err = client.resourceFor(kubernetesObject("apps/v1", "Deployment", "api", ""))
	require.ErrorIs(t, err, errUtils.ErrKubernetesResolveResource)
	require.ErrorContains(t, err, "GVK")
}

func TestSDKClientDeleteReportsDeletedObjects(t *testing.T) {
	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	client := newFakeSDKClient(object)

	results, err := client.Delete(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Equal(t, []objectResult{{
		Action:    "deleted",
		Resource:  "v1/ConfigMap",
		Namespace: "default",
		Name:      "settings",
	}}, results)
}

func TestSDKClientDeleteReportsNotFoundObjects(t *testing.T) {
	// The object does not exist in the fake cluster, so Delete returns NotFound and
	// the result must report "not-found" (regression test for the shadowed-err bug
	// that always reported "deleted").
	client := newFakeSDKClient()

	results, err := client.Delete(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "missing", ""),
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "not-found", results[0].Action)
	assert.Equal(t, "missing", results[0].Name)
}

func TestSDKClientApplyAndDiffWrapFakeDynamicClientErrors(t *testing.T) {
	object := kubernetesObject("v1", "ConfigMap", "settings", "default")
	object.Object["data"] = map[string]any{"key": "value"}
	client := newFakeSDKClient(object.DeepCopy())

	_, err := client.Apply(context.Background(), []*unstructured.Unstructured{object})
	require.ErrorIs(t, err, errUtils.ErrKubernetesApply)
	require.ErrorContains(t, err, "ConfigMap/settings")

	_, err = client.Diff(context.Background(), []*unstructured.Unstructured{object})
	require.ErrorIs(t, err, errUtils.ErrKubernetesDiff)
	require.ErrorContains(t, err, "ConfigMap/settings")
}

func newFakeSDKClient(objects ...runtime.Object) *sdkClient {
	mapper := meta.NewDefaultRESTMapper([]runtimeschema.GroupVersion{{Version: "v1"}})
	mapper.Add(
		runtimeschema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		meta.RESTScopeNamespace,
	)
	mapper.Add(
		runtimeschema.GroupVersionKind{Version: "v1", Kind: "Namespace"},
		meta.RESTScopeRoot,
	)

	return &sdkClient{
		dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), objects...),
		mapper:        mapper,
		namespace:     "default",
	}
}

func kubernetesObject(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name": name,
		},
	}}
	if namespace != "" {
		obj.SetNamespace(namespace)
	}
	return obj
}
