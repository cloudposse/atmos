package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

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

// prependApplyDryRunReactor makes the fake server-side apply (the ApplyPatchType
// dry-run patch the Diff/Apply paths issue) succeed by returning the supplied
// object, instead of the fake client's default "dynamic patch fail" error.
func prependApplyDryRunReactor(fakeClient *fake.FakeDynamicClient, returned *unstructured.Unstructured) {
	fakeClient.PrependReactor("patch", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, returned, nil
	})
}

func TestSDKClientDiffReportsCreateForMissingLiveObject(t *testing.T) {
	// The object does not exist in the cluster, so the live Get returns NotFound
	// and Diff must report a "create" action.
	client, fakeClient := newFakeSDKClientWithFake()
	prependApplyDryRunReactor(fakeClient, kubernetesObject("v1", "ConfigMap", "settings", "default"))

	results, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "create", results[0].Action)
	assert.Equal(t, "settings", results[0].Name)
}

func TestSDKClientDiffReportsNoChangeForEqualObjects(t *testing.T) {
	// The live object equals the dry-run result (after normalization), so Diff
	// must report a "no-change" action.
	live := kubernetesObject("v1", "ConfigMap", "settings", "default")
	live.Object["data"] = map[string]any{"key": "value"}
	client, fakeClient := newFakeSDKClientWithFake(live.DeepCopy())
	prependApplyDryRunReactor(fakeClient, live.DeepCopy())

	results, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "no-change", results[0].Action)
}

func TestSDKClientDiffReportsChangedForDifferingObjects(t *testing.T) {
	// The dry-run result differs from the live object, so Diff must report
	// "changed".
	live := kubernetesObject("v1", "ConfigMap", "settings", "default")
	live.Object["data"] = map[string]any{"key": "value"}
	dryRun := live.DeepCopy()
	dryRun.Object["data"] = map[string]any{"key": "changed"}

	client, fakeClient := newFakeSDKClientWithFake(live.DeepCopy())
	prependApplyDryRunReactor(fakeClient, dryRun)

	results, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "changed", results[0].Action)
}

func TestSDKClientDiffWrapsLiveReadError(t *testing.T) {
	// The dry-run patch succeeds, but reading the live object fails with a
	// non-NotFound error, which Diff must wrap as ErrKubernetesDiff.
	client, fakeClient := newFakeSDKClientWithFake()
	prependApplyDryRunReactor(fakeClient, kubernetesObject("v1", "ConfigMap", "settings", "default"))
	fakeClient.PrependReactor("get", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewServiceUnavailable("cluster down")
	})

	_, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.ErrorIs(t, err, errUtils.ErrKubernetesDiff)
	require.ErrorContains(t, err, "read live")
}

func TestSDKClientDiffReturnsResourceForError(t *testing.T) {
	// An unresolvable GVK must surface from resourceFor before any cluster call.
	client := newFakeSDKClient()

	_, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("apps/v1", "Deployment", "api", ""),
	})

	require.ErrorIs(t, err, errUtils.ErrKubernetesResolveResource)
}

func TestSDKClientApplyReportsAppliedObjects(t *testing.T) {
	// With the server-side apply reactor succeeding, Apply must report the
	// "applied" action for each object (the success append path).
	client, fakeClient := newFakeSDKClientWithFake()
	prependApplyDryRunReactor(fakeClient, kubernetesObject("v1", "ConfigMap", "settings", "default"))

	results, err := client.Apply(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Equal(t, []objectResult{{
		Action:    "applied",
		Resource:  "v1/ConfigMap",
		Namespace: "default",
		Name:      "settings",
	}}, results)
}

func TestSDKClientApplyReturnsResourceForError(t *testing.T) {
	// resourceFor failures are returned immediately (Apply stops at the first error).
	client := newFakeSDKClient()

	_, err := client.Apply(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "", ""),
	})

	require.ErrorIs(t, err, errUtils.ErrKubernetesMissingMetadataName)
}

func TestSDKClientValidateReportsValidAndAggregatesErrors(t *testing.T) {
	// The first object's dry-run succeeds (reactor) and is reported "valid"; the
	// second has no metadata.name so resourceFor fails — Validate must aggregate
	// the failure while still returning the valid result.
	client, fakeClient := newFakeSDKClientWithFake()
	prependApplyDryRunReactor(fakeClient, kubernetesObject("v1", "ConfigMap", "settings", "default"))

	results, err := client.Validate(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
		kubernetesObject("v1", "ConfigMap", "", ""),
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrKubernetesMissingMetadataName)
	require.Len(t, results, 1)
	assert.Equal(t, "valid", results[0].Action)
	assert.Equal(t, "settings", results[0].Name)
}

func newFakeSDKClient(objects ...runtime.Object) *sdkClient {
	client, _ := newFakeSDKClientWithFake(objects...)
	return client
}

// newFakeSDKClientWithFake returns the sdkClient together with the underlying
// fake dynamic client so tests can install reactors (e.g. to make a server-side
// apply dry-run succeed, which the fake otherwise rejects).
func newFakeSDKClientWithFake(objects ...runtime.Object) (*sdkClient, *fake.FakeDynamicClient) {
	mapper := meta.NewDefaultRESTMapper([]runtimeschema.GroupVersion{{Version: "v1"}})
	mapper.Add(
		runtimeschema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		meta.RESTScopeNamespace,
	)
	mapper.Add(
		runtimeschema.GroupVersionKind{Version: "v1", Kind: "Namespace"},
		meta.RESTScopeRoot,
	)

	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	return &sdkClient{
		dynamicClient: dynamicClient,
		mapper:        mapper,
		namespace:     "default",
	}, dynamicClient
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
