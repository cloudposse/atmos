package kubernetes

import (
	"context"
	"encoding/json"
	stderrors "errors"
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
	// Diff holds the unified diff (GitHub ```diff syntax) between the live and
	// desired object for plan/diff operations. Empty for no-change objects,
	// Secret objects (deliberately omitted), and operations that do not diff.
	Diff string
}

func newSDKClient() (*sdkClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load client config: %w", errUtils.ErrKubernetesClientInit, err)
	}

	namespace, _, err := clientConfig.Namespace()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create dynamic client: %w", errUtils.ErrKubernetesClientInit, err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create discovery client: %w", errUtils.ErrKubernetesClientInit, err)
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
			return nil, fmt.Errorf("%w: %s/%s for server-side apply: %w", errUtils.ErrKubernetesMarshal, obj.GetKind(), obj.GetName(), err)
		}

		force := true
		_, err = resource.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %s/%s: %w", errUtils.ErrKubernetesApply, obj.GetKind(), obj.GetName(), err)
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

		// Use a distinct variable name so the not-found check below inspects the
		// delete result, not the (already nil-checked) error from resourceFor.
		deleteErr := resource.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
		if deleteErr != nil && !errors.IsNotFound(deleteErr) {
			return nil, fmt.Errorf("%w: %s/%s: %w", errUtils.ErrKubernetesDelete, obj.GetKind(), obj.GetName(), deleteErr)
		}

		action := "deleted"
		if deleteErr != nil && errors.IsNotFound(deleteErr) {
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
			return nil, fmt.Errorf("%w: %s/%s for server dry-run: %w", errUtils.ErrKubernetesMarshal, obj.GetKind(), obj.GetName(), err)
		}

		force := true
		dryRunObject, err := resource.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
			DryRun:       []string{metav1.DryRunAll},
		})
		if err != nil {
			return nil, fmt.Errorf("%w: server dry-run apply %s/%s: %w", errUtils.ErrKubernetesDiff, obj.GetKind(), obj.GetName(), err)
		}

		liveObject, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
		action := "changed"
		switch {
		case errors.IsNotFound(err):
			action = "create"
		case err != nil:
			return nil, fmt.Errorf("%w: read live %s/%s: %w", errUtils.ErrKubernetesDiff, obj.GetKind(), obj.GetName(), err)
		case objectsEqualForDiff(liveObject, dryRunObject):
			action = "no-change"
		}

		objResult := objectResult{
			Action:    action,
			Resource:  resourceID(obj),
			Namespace: namespace,
			Name:      obj.GetName(),
		}
		// Capture the unified diff for changed/created objects. Secrets are
		// omitted so their data never reaches the unmasked CI job summary, and
		// no-change objects have nothing to show. On create, liveObject is nil
		// (NotFound), yielding an all-additions diff.
		if action != "no-change" && !isSecret(obj) {
			objResult.Diff = buildUnifiedDiff(liveObject, dryRunObject)
		}
		results = append(results, objResult)
	}
	return results, nil
}

// Validate performs a server-side dry-run apply for each object and aggregates
// per-object failures (rather than stopping at the first) so every invalid
// manifest is reported in a single run. Objects the server accepts are returned
// with a "valid" action.
func (c *sdkClient) Validate(ctx context.Context, objects []*unstructured.Unstructured) ([]objectResult, error) {
	defer perf.Track(nil, "kubernetes.sdkClient.Validate")()

	results := make([]objectResult, 0, len(objects))
	var errs []error
	for _, obj := range objects {
		resource, namespace, err := c.resourceFor(obj)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		data, err := json.Marshal(obj.Object)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s/%s for server dry-run: %w", errUtils.ErrKubernetesMarshal, obj.GetKind(), obj.GetName(), err))
			continue
		}

		force := true
		_, err = resource.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
			DryRun:       []string{metav1.DryRunAll},
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s/%s: %w", errUtils.ErrKubernetesValidate, obj.GetKind(), obj.GetName(), err))
			continue
		}

		results = append(results, objectResult{
			Action:    "valid",
			Resource:  resourceID(obj),
			Namespace: namespace,
			Name:      obj.GetName(),
		})
	}

	return results, stderrors.Join(errs...)
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
		return nil, "", fmt.Errorf("%w: GVK %s: %w", errUtils.ErrKubernetesResolveResource, gvk.String(), err)
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
