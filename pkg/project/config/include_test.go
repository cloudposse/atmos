package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

// writeScaffoldIncludeFixture writes licenses.yaml, the local file every
// test in this file points its !include tag at.
func writeScaffoldIncludeFixture(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "licenses.yaml"), []byte(content), 0o600))
}

// TestLoadScaffoldConfigFromContent_IncludeInOptions proves a YQ-filtered
// !include populates a select/multiselect field's options: with real
// {label, value} pairs, end to end through the real ScaffoldSpec schema --
// not just that resolveFieldOptions accepts a hand-built []any of maps.
// Reuses the license vocabulary TestLoadScaffoldConfigFromContent's own
// license: MIT/Apache/GPL example already establishes in this package.
func TestLoadScaffoldConfigFromContent_IncludeInOptions(t *testing.T) {
	dir := t.TempDir()
	writeScaffoldIncludeFixture(t, dir, `MIT: {full_name: MIT License}
Apache-2.0: {full_name: Apache License 2.0}
`)

	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-options-test
spec:
  fields:
    - name: license
      type: select
      options: !include "./licenses.yaml '. | to_entries | map({\"label\": .value.full_name, \"value\": .key})'"
`

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content, WithSourceDir(dir))
	require.NoError(t, err)

	options, err := resolveFieldOptions(&scaffoldConfig.Spec.Fields[0], nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.ElementsMatch(t, []ResolvedOption{
		{Label: "MIT License", Value: "MIT"},
		{Label: "Apache License 2.0", Value: "Apache-2.0"},
	}, options)
}

// TestLoadScaffoldConfigFromContent_IncludeInComputedValue proves an
// unfiltered !include on a computed field's value: lands the raw included
// structure at .Config.<name> (via ComputeFields' literal-value path),
// exactly like a hand-authored literal would.
func TestLoadScaffoldConfigFromContent_IncludeInComputedValue(t *testing.T) {
	dir := t.TempDir()
	writeScaffoldIncludeFixture(t, dir, `MIT: {url: "https://opensource.org/licenses/MIT"}
Apache-2.0: {url: "https://www.apache.org/licenses/LICENSE-2.0"}
`)

	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-computed-test
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ./licenses.yaml
`

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content, WithSourceDir(dir))
	require.NoError(t, err)

	values := map[string]interface{}{}
	require.NoError(t, ComputeFields(scaffoldConfig, values, nil))
	assert.Equal(t, map[string]interface{}{
		"MIT":        map[string]interface{}{"url": "https://opensource.org/licenses/MIT"},
		"Apache-2.0": map[string]interface{}{"url": "https://www.apache.org/licenses/LICENSE-2.0"},
	}, values["license_lookup"])
}

// TestLoadScaffoldConfigFromContent_IncludeInMatrixAxis proves a
// YQ-filtered !include populates a files[].matrix axis with a real string
// list decoded through the ScaffoldSpec schema -- resolveMatrixAxis's own
// []any/[]string handling is unit-tested elsewhere; this proves loading
// actually produces that shape via !include.
func TestLoadScaffoldConfigFromContent_IncludeInMatrixAxis(t *testing.T) {
	dir := t.TempDir()
	writeScaffoldIncludeFixture(t, dir, `MIT: {url: "https://opensource.org/licenses/MIT"}
Apache-2.0: {url: "https://www.apache.org/licenses/LICENSE-2.0"}
`)

	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-matrix-test
spec:
  files:
    - path: "{{ matrix.license }}.txt"
      target: "{{ matrix.license }}.txt"
      matrix:
        license: !include "./licenses.yaml '. | keys'"
`

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content, WithSourceDir(dir))
	require.NoError(t, err)

	require.Len(t, scaffoldConfig.Spec.Files, 1)
	assert.ElementsMatch(t, []interface{}{"MIT", "Apache-2.0"}, scaffoldConfig.Spec.Files[0].Matrix["license"])
}

// TestLoadScaffoldConfigFromContent_WithIncludedPaths proves
// WithIncludedPaths, alongside WithSourceDir, collects the raw path
// argument of every !include/!include.raw tag resolved -- the mechanism
// ui.go's generation loop uses to exclude those files from output.
func TestLoadScaffoldConfigFromContent_WithIncludedPaths(t *testing.T) {
	dir := t.TempDir()
	writeScaffoldIncludeFixture(t, dir, "MIT: {}\n")

	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-included-paths-test
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ./licenses.yaml
`

	var includedPaths []string
	_, err := LoadScaffoldConfigFromContent(content, WithSourceDir(dir), WithIncludedPaths(&includedPaths))
	require.NoError(t, err)
	assert.Equal(t, []string{"./licenses.yaml"}, includedPaths)
}

// TestLoadScaffoldConfigFromContent_WithIncludedPaths_WithoutSourceDir
// proves WithIncludedPaths has no effect without WithSourceDir -- there is
// nothing to resolve, so nothing to collect either.
func TestLoadScaffoldConfigFromContent_WithIncludedPaths_WithoutSourceDir(t *testing.T) {
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-included-paths-no-sourcedir-test
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ./licenses.yaml
`

	var includedPaths []string
	_, err := LoadScaffoldConfigFromContent(content, WithIncludedPaths(&includedPaths))
	require.NoError(t, err)
	assert.Empty(t, includedPaths)
}

// TestLoadScaffoldConfigFromContent_WithoutSourceDir_LeavesIncludeUnresolved
// proves omitting WithSourceDir (every call site that predates this
// feature, and any in-memory-only content with no meaningful directory)
// behaves exactly as before -- the raw !include tag decodes as a plain
// unresolved string, not an error.
func TestLoadScaffoldConfigFromContent_WithoutSourceDir_LeavesIncludeUnresolved(t *testing.T) {
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-no-sourcedir-test
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ./licenses.yaml
`

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	assert.Equal(t, "./licenses.yaml", scaffoldConfig.Spec.Fields[0].Value)
}

// TestLoadScaffoldConfigFromContent_IncludeRemoteHTTP proves a remote
// (https://) !include target is dispatched correctly from scaffold
// context, using the same mock-HTTP-client injection point pkg/utils' own
// !include tests use -- no real network, no real GitHub credentials.
func TestLoadScaffoldConfigFromContent_IncludeRemoteHTTP(t *testing.T) {
	oldWaiter := github.RateLimitWaiter
	github.RateLimitWaiter = func(_ context.Context, _ int) error { return nil }
	t.Cleanup(func() { github.RateLimitWaiter = oldWaiter })

	mock := httpmock.NewGitHubMockServer(t)
	mock.RegisterFile("scaffold-fixtures/licenses.yaml", "MIT: {}\nApache-2.0: {}\n")

	oldClient := utils.TestHTTPClient
	utils.TestHTTPClient = mock.HTTPClient()
	t.Cleanup(func() { utils.TestHTTPClient = oldClient })

	dir := t.TempDir()
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-remote-test
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include https://raw.githubusercontent.com/scaffold-fixtures/licenses.yaml
`

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content, WithSourceDir(dir))
	require.NoError(t, err)

	values := map[string]interface{}{}
	require.NoError(t, ComputeFields(scaffoldConfig, values, nil))
	assert.Equal(t, map[string]interface{}{"MIT": map[string]interface{}{}, "Apache-2.0": map[string]interface{}{}}, values["license_lookup"])
}
