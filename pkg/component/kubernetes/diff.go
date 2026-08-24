package kubernetes

import (
	"fmt"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/cloudposse/atmos/pkg/perf"
)

// secretKind is the core Kubernetes kind whose contents are deliberately
// excluded from diffs. The CI summary writer does not mask content, so a
// Secret's data would otherwise land in the job summary in plaintext.
const secretKind = "Secret"

// isSecret reports whether obj is a core (group "") v1 Secret.
func isSecret(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	gvk := obj.GroupVersionKind()
	return gvk.Group == "" && gvk.Kind == secretKind
}

// buildUnifiedDiff returns a unified diff (GitHub ```diff syntax) between the
// live object and the dry-run (desired) object, after stripping server-managed
// noise via normalizeForDiff. A nil/empty live object (create) yields an
// all-additions diff. An empty diff is returned when the normalized objects are
// identical.
func buildUnifiedDiff(live, dryRun *unstructured.Unstructured) string {
	defer perf.Track(nil, "kubernetes.buildUnifiedDiff")()

	before := normalizedYAML(live)
	after := normalizedYAML(dryRun)
	if before == after {
		return ""
	}

	name := diffName(dryRun, live)
	edits := myers.ComputeEdits(span.URIFromPath(name), before, after)
	return fmt.Sprint(gotextdiff.ToUnified("a/"+name, "b/"+name, before, edits))
}

// normalizedYAML marshals obj to canonical (sorted-key) YAML after removing
// server-managed fields. A nil or empty object marshals to an empty string so
// that creates/deletes render as pure additions/removals.
func normalizedYAML(obj *unstructured.Unstructured) string {
	if obj == nil || len(obj.Object) == 0 {
		return ""
	}
	clone := obj.DeepCopy()
	normalizeForDiff(clone)
	out, err := sigsyaml.Marshal(clone.Object)
	if err != nil {
		return ""
	}
	return string(out)
}

// diffName builds a stable label for the unified diff header, preferring the
// desired object and falling back to the live object (for deletes).
func diffName(primary, fallback *unstructured.Unstructured) string {
	obj := primary
	if obj == nil || len(obj.Object) == 0 {
		obj = fallback
	}
	if obj == nil {
		return "object.yaml"
	}
	if ns := obj.GetNamespace(); ns != "" {
		return fmt.Sprintf("%s_%s_%s.yaml", resourceID(obj), ns, obj.GetName())
	}
	return fmt.Sprintf("%s_%s.yaml", resourceID(obj), obj.GetName())
}
