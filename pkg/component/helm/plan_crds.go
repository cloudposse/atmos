package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	helmchart "helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prepareDiff keeps offline comparisons complete, but makes release plans match
// Helm's lifecycle: crds/ files are installed only when a release is absent and
// only when the CRD itself does not already exist. Templates remain untouched.
func prepareDiff(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo,
	flags map[string]any, spec *chartSpec,
) (string, *chartSpec, error) {
	renderSpec := *spec
	against := flagString(flags, flagAgainst)
	if flagString(flags, flagFromManifest) != "" || (against != "" && against != againstRelease) {
		baseline, err := resolveDiffBaseline(ctx, config, info, flags, spec)
		return baseline, &renderSpec, err
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	deployed, err := getDeployedRelease(spec.ReleaseName, spec.Namespace, spec.recovery)
	if err != nil {
		return "", nil, err
	}
	renderSpec.IncludeCRDs = false
	if deployed != nil {
		return renderReleaseManifest(deployed), &renderSpec, nil
	}
	if spec.IncludeCRDs {
		lookup := newCRDExistenceLookup(spec)
		renderSpec.planCRDs = func(ctx context.Context, chart helmchart.Charter) (string, error) {
			return missingCRDManifests(ctx, chart, lookup)
		}
	}
	return "", &renderSpec, nil
}

// newCRDExistenceLookup initializes its client lazily so charts without crds/
// require no CRD API access. GET avoids requiring cluster-wide list permission.
func newCRDExistenceLookup(spec *chartSpec) func(context.Context, string) (bool, error) {
	var client dynamic.Interface
	return func(ctx context.Context, name string) (bool, error) {
		if client == nil {
			settings := newSettingsForNamespace(spec.Namespace)
			configureRecoveryTransport(settings, spec.recovery)
			restConfig, err := settings.RESTClientGetter().ToRESTConfig()
			if err != nil {
				return false, err
			}
			client, err = dynamic.NewForConfig(restConfig)
			if err != nil {
				return false, err
			}
		}
		_, err := client.Resource(kubeschema.GroupVersionResource{
			Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
		}).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}
}

// missingCRDManifests uses Helm's processed chart (including enabled subcharts),
// not a kind-based filter that could also suppress managed template resources.
func missingCRDManifests(ctx context.Context, chart helmchart.Charter,
	exists func(context.Context, string) (bool, error),
) (string, error) {
	loaded, ok := chart.(*chartv2.Chart)
	if !ok {
		return "", fmt.Errorf("%w: cannot plan CRDs for chart type %T", errUtils.ErrHelmDiffFailed, chart)
	}
	var out bytes.Buffer
	for _, crd := range loaded.CRDObjects() {
		content, err := missingCRDFile(ctx, crd, exists)
		if err != nil {
			return "", err
		}
		out.WriteString(content)
	}
	return out.String(), nil
}

const crdDecoderBufferSize = 4096

func missingCRDFile(ctx context.Context, crd chartv2.CRD,
	exists func(context.Context, string) (bool, error),
) (string, error) {
	var out bytes.Buffer
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(crd.File.Data), crdDecoderBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var object unstructured.Unstructured
		if err := decoder.Decode(&object); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return "", fmt.Errorf("decode CRD file %q: %w", crd.Filename, err)
		}
		content, err := missingCRDDocument(ctx, &object, crd.Filename, exists)
		if err != nil {
			return "", fmt.Errorf("plan CRD file %q: %w", crd.Filename, err)
		}
		out.WriteString(content)
	}
	return out.String(), nil
}

// missingCRDDocument expands Lists just as Helm's Kubernetes resource builder
// does, including nested Lists, before checking individual CRDs.
func missingCRDDocument(ctx context.Context, object *unstructured.Unstructured, source string,
	exists func(context.Context, string) (bool, error),
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if object.IsList() {
		list, err := object.ToList()
		if err != nil {
			return "", err
		}
		var out bytes.Buffer
		for i := range list.Items {
			content, err := missingCRDDocument(ctx, &list.Items[i], source, exists)
			if err != nil {
				return "", err
			}
			out.WriteString(content)
		}
		return out.String(), nil
	}
	content, err := missingCRDObject(ctx, object, exists)
	if err != nil || len(content) == 0 {
		return "", err
	}
	return fmt.Sprintf("---\n# Source: %s\n%s\n", source, content), nil
}

func missingCRDObject(ctx context.Context, object *unstructured.Unstructured,
	exists func(context.Context, string) (bool, error),
) ([]byte, error) {
	if len(object.Object) == 0 {
		return nil, nil
	}
	if object.GetKind() != "CustomResourceDefinition" || object.GetName() == "" {
		return nil, fmt.Errorf("%w: expected a named CustomResourceDefinition", errUtils.ErrHelmDiffFailed)
	}
	found, err := exists(ctx, object.GetName())
	if err != nil {
		return nil, fmt.Errorf("check existing CRD %q: %w", object.GetName(), err)
	}
	if found {
		return nil, nil
	}
	return object.MarshalJSON()
}
