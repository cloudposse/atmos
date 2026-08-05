package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apivalidation "k8s.io/apimachinery/pkg/util/validation"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// validateOptions controls how rendered manifests are validated.
type validateOptions struct {
	// Server enables a server-side dry-run apply against the live cluster in
	// addition to the default offline structural checks.
	Server bool
}

// resolveValidateOptions extracts validate options from the CLI flag map.
func resolveValidateOptions(flags map[string]any) validateOptions {
	options := validateOptions{}
	if value, ok := flags["server"].(bool); ok && value {
		options.Server = true
	}
	return options
}

// runValidate validates the rendered objects. Offline structural checks always
// run; the cluster dry-run only runs when --server is set. All failures are
// collected and reported together rather than stopping at the first.
func runValidate(objects []*unstructured.Unstructured, options validateOptions) ([]objectResult, error) {
	defer perf.Track(nil, "kubernetes.runValidate")()

	if err := validateObjectsStructural(objects); err != nil {
		return nil, err
	}

	if options.Server {
		return runServerValidate(objects)
	}

	ui.Successf("validated %d Kubernetes object(s)", len(objects))
	return objectsToResults("valid", objects), nil
}

// validateObjectsStructural runs offline structural validation over every object
// and returns a single aggregate error describing all failures, or nil if every
// object is valid. It is reused by the apply/deploy auto-gate.
func validateObjectsStructural(objects []*unstructured.Unstructured) error {
	var errs []error
	for i, obj := range objects {
		errs = append(errs, structuralErrorsForObject(i, obj)...)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", errUtils.ErrKubernetesValidationFailed, errors.Join(errs...))
	}
	return nil
}

// structuralErrorsForObject returns the offline validation errors for a single
// object: a present, DNS-1123-conformant metadata.name and a resolvable GVK.
// (apiVersion/kind presence is already guaranteed upstream by decodeObjects.)
func structuralErrorsForObject(index int, obj *unstructured.Unstructured) []error {
	var errs []error
	ref := objectRef(index, obj)

	name := obj.GetName()
	if name == "" {
		errs = append(errs, fmt.Errorf("%s: %w", ref, errUtils.ErrKubernetesMissingMetadataName))
	} else if msgs := apivalidation.IsDNS1123Subdomain(name); len(msgs) > 0 {
		errs = append(errs, fmt.Errorf("%s: %w: %s", ref, errUtils.ErrKubernetesManifestInvalidName, strings.Join(msgs, "; ")))
	}

	if obj.GroupVersionKind().Empty() {
		errs = append(errs, fmt.Errorf("%s: %w", ref, errUtils.ErrKubernetesMissingGVK))
	}

	return errs
}

// objectRef builds a human-readable identifier for an object in validation
// messages, falling back to a positional reference when the name is missing.
func objectRef(index int, obj *unstructured.Unstructured) string {
	kind := obj.GetKind()
	if kind == "" {
		kind = "object"
	}
	if name := obj.GetName(); name != "" {
		return fmt.Sprintf("%s/%s", kind, name)
	}
	return fmt.Sprintf("%s[%d]", kind, index)
}

// runServerValidate validates the objects against the live cluster using a
// server-side dry-run apply.
func runServerValidate(objects []*unstructured.Unstructured) ([]objectResult, error) {
	client, err := newKubernetesSDKClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	results, err := client.Validate(ctx, objects)
	if err != nil {
		return results, fmt.Errorf("%w: %w", errUtils.ErrKubernetesValidationFailed, err)
	}

	printResults(results)
	return results, nil
}
