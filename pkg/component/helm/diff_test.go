package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const baseConfigMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: demo
data:
  replicas: "2"
`

func TestUnifiedDiff_DetectsChange(t *testing.T) {
	newManifest := strings.Replace(baseConfigMap, `replicas: "2"`, `replicas: "3"`, 1)

	out, changed, err := unifiedDiff(baseConfigMap, newManifest, "demo", 0)
	require.NoError(t, err)
	assert.True(t, changed, "expected a change to be detected")
	assert.Contains(t, out, "has changed")
	assert.Contains(t, out, `-   replicas: "2"`)
	assert.Contains(t, out, `+   replicas: "3"`)
}

func TestUnifiedDiff_NoChange(t *testing.T) {
	out, changed, err := unifiedDiff(baseConfigMap, baseConfigMap, "demo", 0)
	require.NoError(t, err)
	assert.False(t, changed, "expected no change for identical manifests")
	assert.Empty(t, strings.TrimSpace(out))
}

func TestUnifiedDiff_NewObjectAllAdded(t *testing.T) {
	// Empty baseline (e.g. release not found / no GitOps target yet) => everything added.
	out, changed, err := unifiedDiff("", baseConfigMap, "demo", 0)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Contains(t, out, "app-config")
}

func TestUnifiedDiff_RedactsSecretValues(t *testing.T) {
	oldSecret := `apiVersion: v1
kind: Secret
metadata:
  name: app-secret
  namespace: demo
type: Opaque
data:
  password: b2xkcGFzcw==
`
	newSecret := strings.Replace(oldSecret, "b2xkcGFzcw==", "bmV3cGFzcw==", 1)

	out, changed, err := unifiedDiff(oldSecret, newSecret, "demo", 0)
	require.NoError(t, err)
	assert.True(t, changed, "expected the secret change to be detected")
	// The raw base64 secret values must never appear in the diff output.
	assert.NotContains(t, out, "b2xkcGFzcw==")
	assert.NotContains(t, out, "bmV3cGFzcw==")
}

func TestColorizeUnifiedDiffAddsANSIDiffColors(t *testing.T) {
	ui.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { ui.SetColorProfile(termenv.Ascii) })

	diffText := "--- a/demo.yaml\n+++ b/demo.yaml\n@@ -1 +1 @@\n-old\n+new\n"
	out := colorizeUnifiedDiff(diffText)

	assert.Contains(t, out, "\x1b[")
	assert.Contains(t, out, "-old")
	assert.Contains(t, out, "+new")
}

func TestColorizeUnifiedDiffNoColorLeavesPlainText(t *testing.T) {
	ui.SetColorProfile(termenv.Ascii)

	diffText := "--- a/demo.yaml\n+++ b/demo.yaml\n@@ -1 +1 @@\n-old\n+new\n"
	out := colorizeUnifiedDiff(diffText)

	assert.Equal(t, diffText, out)
	assert.NotContains(t, out, "\x1b[")
}

func TestJoinManifests_SortedWithSeparators(t *testing.T) {
	files := map[string][]byte{
		"b.yaml": []byte("kind: B\n"),
		"a.yaml": []byte("kind: A\n"),
		"c.yaml": []byte("   "), // whitespace-only entries are skipped.
	}
	out := joinManifests(files)
	// Sorted by key: A before B; the blank c.yaml is omitted.
	assert.Equal(t, "kind: A\n---\nkind: B", out)
}

func TestResolveDiffBaseline_FromManifestFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.yaml")
	require.NoError(t, os.WriteFile(path, []byte(baseConfigMap), 0o600))

	spec := &chartSpec{ReleaseName: "app", Namespace: "demo"}
	flags := map[string]any{flagFromManifest: path}

	got, err := resolveDiffBaseline(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, flags, spec)
	require.NoError(t, err)
	assert.Equal(t, baseConfigMap, got)
}

func TestResolveDiffBaseline_FromManifestFileMissing(t *testing.T) {
	spec := &chartSpec{ReleaseName: "app", Namespace: "demo"}
	flags := map[string]any{flagFromManifest: filepath.Join(t.TempDir(), "nope.yaml")}

	_, err := resolveDiffBaseline(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, flags, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrHelmBaselineRead)
}
