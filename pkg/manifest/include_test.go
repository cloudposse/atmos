package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeIncludeFixture writes a local file a test's !include tag points at.
func writeIncludeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

// TestLoad_WithIncludeResolution_LocalFile proves a local !include target
// resolves into a real Go value (a map, here) before Load ever decodes the
// document into the typed Manifest[S] -- not just that it doesn't error.
func TestLoad_WithIncludeResolution_LocalFile(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	writeIncludeFixture(t, dir, "greeting.yaml", "hello: world\nfoo: bar\n")

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  values: !include ./greeting.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, map[string]any{"hello": "world", "foo": "bar"}, m.Spec.Values)
}

// TestLoad_WithoutIncludeResolution_LeavesTagUnresolved proves omitting
// WithIncludeResolution (every existing Load caller, and any future one
// that doesn't opt in) behaves exactly as before this option existed -- the
// raw !include tag's argument decodes as a plain, unresolved string, not an
// error and not a resolved value.
func TestLoad_WithoutIncludeResolution_LeavesTagUnresolved(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	writeIncludeFixture(t, dir, "greeting.yaml", "hello: world\n")

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !include ./greeting.yaml
`)

	m, err := Load[testSpec](testKind, data)
	require.NoError(t, err)
	assert.Equal(t, "./greeting.yaml", m.Spec.Source)
}

// TestLoad_WithIncludeResolution_ResolvesBeforeValidate proves resolution
// happens before schema validation, not just before the final decode: Type
// is schema-constrained to a fixed enum, and the resolved value
// ("multiselect") only satisfies that enum after !include resolves it --
// the tag's own raw argument ("./type.yaml") would fail the same enum
// check. If resolve-before-validate ever regressed to validate-before-
// resolve, this would start failing with a schema validation error instead
// of succeeding.
func TestLoad_WithIncludeResolution_ResolvesBeforeValidate(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	writeIncludeFixture(t, dir, "type.yaml", "multiselect\n")

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  fields:
    - name: f
      type: !include ./type.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	require.Len(t, m.Spec.Fields, 1)
	assert.Equal(t, "multiselect", m.Spec.Fields[0].Type)
}

// TestLoad_WithIncludeResolution_WrongShapeRejectedByValidate proves schema
// validation still meaningfully checks the *resolved* value, not just that
// resolution always succeeds -- a !include target resolving to a value the
// schema doesn't allow (an enum violation) is correctly rejected, the same
// way a hand-authored bad value would be.
func TestLoad_WithIncludeResolution_WrongShapeRejectedByValidate(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	writeIncludeFixture(t, dir, "type.yaml", "bogus-type\n")

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  fields:
    - name: f
      type: !include ./type.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestValidation)
}

// TestLoad_WithIncludeResolution_RelativeToManifestFile proves a local
// !include target resolves relative to the manifest file's own directory
// (the file argument), not the process's current working directory --
// resolution must still succeed after chdir-ing somewhere else entirely.
func TestLoad_WithIncludeResolution_RelativeToManifestFile(t *testing.T) {
	registerTestKind(t)

	manifestDir := t.TempDir()
	writeIncludeFixture(t, manifestDir, "greeting.yaml", "hello: world\n")

	elsewhere := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(elsewhere))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  values: !include ./greeting.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: manifestDir, BasePathAbsolute: manifestDir}
	manifestFile := filepath.Join(manifestDir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"hello": "world"}, m.Spec.Values)
}

// TestLoad_WithIncludeResolution_CollectsConsumedPaths proves the
// consumedPaths accumulator collects every tag's raw path argument exactly
// as written, across multiple !include/!include.raw tags anywhere in the
// document -- the mechanism a caller uses to find which local files exist
// solely to be included.
func TestLoad_WithIncludeResolution_CollectsConsumedPaths(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	writeIncludeFixture(t, dir, "greeting.yaml", "hello: world\n")
	writeIncludeFixture(t, dir, "raw.txt", "hello")

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !include.raw ./raw.txt
  values: !include ./greeting.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	var consumedPaths []string
	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, &consumedPaths))
	require.NoError(t, err)
	assert.Equal(t, "hello", m.Spec.Source)
	assert.ElementsMatch(t, []string{"./greeting.yaml", "./raw.txt"}, consumedPaths)
}

// TestLoad_WithIncludeResolution_MissingLocalFile proves a local !include
// target that genuinely doesn't exist fails loudly and clearly, rather than
// silently decoding as some placeholder value.
func TestLoad_WithIncludeResolution_MissingLocalFile(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()

	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  values: !include ./does-not-exist.yaml
`)

	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.Error(t, err)
}
