package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apivalidation "k8s.io/apimachinery/pkg/util/validation"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
// Kustomize's own config objects (see isKustomizeConfigObject) are exempt from
// the metadata.name presence requirement only — a name, if given, is still
// validated, and the GVK check remains unconditional.
func structuralErrorsForObject(index int, obj *unstructured.Unstructured) []error {
	var errs []error
	ref := objectRef(index, obj)

	name := obj.GetName()
	if name == "" {
		if !isKustomizeConfigObject(obj) {
			errs = append(errs, fmt.Errorf("%s: %w", ref, errUtils.ErrKubernetesMissingMetadataName))
		}
	} else if msgs := apivalidation.IsDNS1123Subdomain(name); len(msgs) > 0 {
		// Backtick-fenced as a code span: the k8s validation message embeds an
		// unbroken regex (no spaces to wrap on) containing '[', ']', '(', ')' --
		// rendered as plain markdown text, the CLI's glamour renderer both
		// mangles those characters (they collide with link syntax) and hard-wraps
		// mid-token at terminal width. A code span renders verbatim, monospaced,
		// and unwrapped.
		errs = append(errs, fmt.Errorf("%s: %w: `%s`", ref, errUtils.ErrKubernetesManifestInvalidName, strings.Join(msgs, "; ")))
	}

	if obj.GroupVersionKind().Empty() {
		errs = append(errs, fmt.Errorf("%s: %w", ref, errUtils.ErrKubernetesMissingGVK))
	}

	return errs
}

// isKustomizeConfigObject reports whether obj is one of Kustomize's own reserved
// config-object kinds (Kustomization or Component). These are matched against
// Kustomize's own exported kind/version constants (sigs.k8s.io/kustomize/api/types,
// already vendored by this repo's native kustomize provider) rather than a guessed
// string, mirroring exactly what Kustomize's own EnforceFields validation checks.
// Such objects are never submitted to the Kubernetes API — they are local build
// input consumed by the kustomize tool itself — and Kustomize does not require
// (or, historically, even permit) a metadata.name on them.
func isKustomizeConfigObject(obj *unstructured.Unstructured) bool {
	apiVersion, kind := obj.GetAPIVersion(), obj.GetKind()
	switch {
	case apiVersion == kustomizetypes.KustomizationVersion && kind == kustomizetypes.KustomizationKind:
		return true
	case apiVersion == kustomizetypes.ComponentVersion && kind == kustomizetypes.ComponentKind:
		return true
	default:
		return false
	}
}

// resolveComponentValidateEnabled reports whether structural validation is
// enabled for this component. Component-level `validate: false` opts out of all
// automatic (apply/deploy auto-gate) and explicit (`atmos kubernetes validate`)
// structural checks; it does not affect --server, which validates against the
// live cluster's own API rather than Atmos's offline opinion.
//
// A present-but-non-bool value (e.g. a quoted `validate: "false"`) is a
// fail-closed error rather than a silent ignore: `atmos kubernetes
// validate/apply/deploy` resolve this section directly and never go through
// the stricter Atmos manifest JSON Schema check that `atmos validate
// stacks`/`describe stacks` run, so this is the only gate on this path that
// catches the mistake.
func resolveComponentValidateEnabled(componentSection map[string]any) (bool, error) {
	raw, present := componentSection[cfg.ValidateSectionName]
	if !present {
		return true, nil
	}
	v, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%w: got %T", errUtils.ErrKubernetesValidateSectionInvalid, raw)
	}
	return v, nil
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
