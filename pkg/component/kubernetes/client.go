package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	fieldManager = "atmos"
	// The metadataField const is the top-level Kubernetes object field that holds object metadata.
	metadataField = "metadata"
	// The dirPerm const is the permission mode used when creating directories.
	dirPerm = 0o755
	// The filePerm const is the permission mode used when writing rendered manifest files.
	filePerm = 0o600
)

type sdkClient struct {
	dynamicClient dynamic.Interface
	mapper        meta.RESTMapper
	namespace     string
}

type objectResult struct {
	Action    string
	Resource  string
	Namespace string
	Name      string
}

func newSDKClient() (*sdkClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load Kubernetes client config: %w", err)
	}

	namespace, _, err := clientConfig.Namespace()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes discovery client: %w", err)
	}

	return &sdkClient{
		dynamicClient: dynamicClient,
		mapper:        restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
		namespace:     namespace,
	}, nil
}

func (c *sdkClient) Apply(ctx context.Context, objects []*unstructured.Unstructured) ([]objectResult, error) {
	defer perf.Track(nil, "kubernetes.sdkClient.Apply")()

	results := make([]objectResult, 0, len(objects))
	for _, obj := range objects {
		resource, namespace, err := c.resourceFor(obj)
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %s/%s for server-side apply: %w", obj.GetKind(), obj.GetName(), err)
		}

		force := true
		_, err = resource.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		results = append(results, objectResult{
			Action:    "applied",
			Resource:  resourceID(obj),
			Namespace: namespace,
			Name:      obj.GetName(),
		})
	}
	return results, nil
}

func (c *sdkClient) Delete(ctx context.Context, objects []*unstructured.Unstructured) ([]objectResult, error) {
	defer perf.Track(nil, "kubernetes.sdkClient.Delete")()

	results := make([]objectResult, 0, len(objects))
	for _, obj := range objects {
		resource, namespace, err := c.resourceFor(obj)
		if err != nil {
			return nil, err
		}

		if err := resource.Delete(ctx, obj.GetName(), metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		action := "deleted"
		if err != nil && errors.IsNotFound(err) {
			action = "not-found"
		}
		results = append(results, objectResult{
			Action:    action,
			Resource:  resourceID(obj),
			Namespace: namespace,
			Name:      obj.GetName(),
		})
	}
	return results, nil
}

func (c *sdkClient) Diff(ctx context.Context, objects []*unstructured.Unstructured) ([]objectResult, error) {
	defer perf.Track(nil, "kubernetes.sdkClient.Diff")()

	results := make([]objectResult, 0, len(objects))
	for _, obj := range objects {
		resource, namespace, err := c.resourceFor(obj)
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %s/%s for server dry-run: %w", obj.GetKind(), obj.GetName(), err)
		}

		force := true
		dryRunObject, err := resource.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
			DryRun:       []string{metav1.DryRunAll},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to server dry-run apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		liveObject, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
		action := "changed"
		switch {
		case errors.IsNotFound(err):
			action = "create"
		case err != nil:
			return nil, fmt.Errorf("failed to read live %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		case objectsEqualForDiff(liveObject, dryRunObject):
			action = "no-change"
		}

		results = append(results, objectResult{
			Action:    action,
			Resource:  resourceID(obj),
			Namespace: namespace,
			Name:      obj.GetName(),
		})
	}
	return results, nil
}

func (c *sdkClient) resourceFor(obj *unstructured.Unstructured) (dynamic.ResourceInterface, string, error) {
	if obj.GetName() == "" {
		return nil, "", fmt.Errorf("%w: %s", errUtils.ErrKubernetesMissingMetadataName, resourceID(obj))
	}

	gvk := obj.GroupVersionKind()
	if gvk.Empty() {
		return nil, "", fmt.Errorf("%w: %s", errUtils.ErrKubernetesMissingGVK, obj.GetName())
	}

	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve GVK %s to a Kubernetes resource: %w", gvk.String(), err)
	}

	namespaceableResource := c.dynamicClient.Resource(mapping.Resource)
	resource := dynamic.ResourceInterface(namespaceableResource)
	namespace := ""
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		namespace = obj.GetNamespace()
		if namespace == "" {
			namespace = c.namespace
			obj.SetNamespace(namespace)
		}
		resource = namespaceableResource.Namespace(namespace)
	}

	return resource, namespace, nil
}

func objectsEqualForDiff(live *unstructured.Unstructured, dryRun *unstructured.Unstructured) bool {
	liveCopy := live.DeepCopy()
	dryRunCopy := dryRun.DeepCopy()
	normalizeForDiff(liveCopy)
	normalizeForDiff(dryRunCopy)
	return reflect.DeepEqual(liveCopy.Object, dryRunCopy.Object)
}

func normalizeForDiff(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, metadataField, "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, metadataField, "generation")
	unstructured.RemoveNestedField(obj.Object, metadataField, "managedFields")
	unstructured.RemoveNestedField(obj.Object, metadataField, "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, metadataField, "uid")
	unstructured.RemoveNestedField(obj.Object, "status")
}

func resourceID(obj *unstructured.Unstructured) string {
	gvk := obj.GroupVersionKind()
	if gvk.Group == "" {
		return fmt.Sprintf("%s/%s", gvk.Version, gvk.Kind)
	}
	return fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
}
