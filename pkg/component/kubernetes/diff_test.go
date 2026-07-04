package kubernetes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func configMap(data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "settings",
			"namespace": "default",
		},
		"data": data,
	}}
}

func TestIsSecret(t *testing.T) {
	secret := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
	}}
	assert.True(t, isSecret(secret))

	assert.False(t, isSecret(configMap(map[string]any{"a": "b"})))
	assert.False(t, isSecret(nil))

	// A "Secret" in a non-core group is not the core v1 Secret.
	customSecret := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Secret",
	}}
	assert.False(t, isSecret(customSecret))
}

func TestBuildUnifiedDiffChanged(t *testing.T) {
	live := configMap(map[string]any{"key": "old"})
	dryRun := configMap(map[string]any{"key": "new"})

	diff := buildUnifiedDiff(live, dryRun)

	assert.Contains(t, diff, "-  key: old")
	assert.Contains(t, diff, "+  key: new")
}

func TestBuildUnifiedDiffCreateIsAllAdditions(t *testing.T) {
	// On create, the live object is nil (NotFound) -> all additions.
	dryRun := configMap(map[string]any{"key": "value"})

	diff := buildUnifiedDiff(nil, dryRun)

	assert.NotEmpty(t, diff)
	assert.Contains(t, diff, "+kind: ConfigMap")
	assert.Contains(t, diff, "+  key: value")
	assert.NotContains(t, diff, "\n-") // no removals when creating.
}

func TestBuildUnifiedDiffNoChangeIsEmpty(t *testing.T) {
	live := configMap(map[string]any{"key": "value"})
	dryRun := configMap(map[string]any{"key": "value"})

	assert.Empty(t, buildUnifiedDiff(live, dryRun))
}

func TestBuildUnifiedDiffIgnoresServerManagedFields(t *testing.T) {
	// Only the server-managed metadata/status differ; the desired spec is equal,
	// so the diff must be empty (normalizeForDiff strips the noise).
	live := configMap(map[string]any{"key": "value"})
	live.Object["metadata"] = map[string]any{
		"name":              "settings",
		"namespace":         "default",
		"creationTimestamp": "now",
		"generation":        int64(2),
		"managedFields":     []any{map[string]any{"manager": "kubectl"}},
		"resourceVersion":   "10",
		"uid":               "abc",
	}
	live.Object["status"] = map[string]any{"observedGeneration": int64(2)}
	dryRun := configMap(map[string]any{"key": "value"})

	assert.Empty(t, buildUnifiedDiff(live, dryRun))
}

func TestBuildUnifiedDiffHeaderUsesObjectIdentity(t *testing.T) {
	live := configMap(map[string]any{"key": "old"})
	dryRun := configMap(map[string]any{"key": "new"})

	diff := buildUnifiedDiff(live, dryRun)
	firstLine, _, _ := strings.Cut(diff, "\n")

	assert.Contains(t, firstLine, "v1/ConfigMap_default_settings.yaml")
}
