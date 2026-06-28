package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

const vendorFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    # VPC component.
    - component: "vpc"
      source: "oci://ghcr.io/cloudposse/atmos/mock:{{.Version}}"
      version: "v0"  # pinned version
      targets:
        - "components/terraform/vpc"
    - component: "eks"
      source: "github.com/cloudposse/terraform-aws-eks?ref={{.Version}}"
      version: "1.2.3"
`

func writeVendor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte(vendorFixture), 0o644))
	return file
}

func TestGetComponentVersion(t *testing.T) {
	file := writeVendor(t)

	v, err := GetComponentVersion(file, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "v0", v)

	v, err = GetComponentVersion(file, "eks")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", v)
}

func TestGetComponentVersion_NotFound(t *testing.T) {
	file := writeVendor(t)
	_, err := GetComponentVersion(file, "missing")
	require.Error(t, err)
}

func TestSetComponentVersion_PreservesFormatting(t *testing.T) {
	file := writeVendor(t)

	require.NoError(t, SetComponentVersion(file, "vpc", "v1.5.0"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(got)

	assert.Contains(t, s, `version: "v1.5.0"`, "vpc version updated")
	assert.Contains(t, s, "# VPC component.", "comment preserved")
	assert.Contains(t, s, "# pinned version", "inline comment preserved")
	assert.Contains(t, s, "{{.Version}}", "template in source preserved")
	// eks untouched.
	v, err := GetComponentVersion(file, "eks")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", v)
}

func TestSetComponentVersion_NotFound(t *testing.T) {
	file := writeVendor(t)
	err := SetComponentVersion(file, "nope", "v9")
	require.Error(t, err)
	// A genuinely missing component is reported as path-not-found.
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

// TestSetComponentVersion_SurfacesRealError verifies that a non "component
// missing" failure (here, an unreadable/nonexistent manifest file) is returned
// as-is rather than being rewritten as "component not found".
func TestSetComponentVersion_SurfacesRealError(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	err := SetComponentVersion(missingFile, "vpc", "v9")
	require.Error(t, err)
	// The real cause (read failure) must surface, not a bogus "component not found".
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
	assert.NotErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

// TestSetComponentVersion_InvalidYAML verifies that invalid YAML surfaces a parse
// error, not a "component not found" message.
func TestSetComponentVersion_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte("spec: {sources: [ unclosed\n"), 0o644))

	err := SetComponentVersion(file, "vpc", "v9")
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
	assert.NotErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound, "invalid YAML must not be reported as component-not-found")
}
