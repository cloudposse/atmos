package manifest

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestScaffoldTagPolicy_HandlersMatchScaffoldYAML guards against the
// scaffold tag policy's Handlers map and fntag.ScaffoldYAML's allow-list
// drifting apart -- they're meant to describe exactly the same set, kept as
// two call sites (one for dispatch, one for documentation/error messages)
// rather than one deriving from the other, so nothing enforces this except
// this test.
func TestScaffoldTagPolicy_HandlersMatchScaffoldYAML(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	manifestFile := filepath.Join(dir, "manifest.yaml")
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}

	// Every tag ScaffoldYAML claims to support must actually resolve (or at
	// least not be rejected as unsupported) when used; every tag it doesn't
	// list must be rejected. Exercise this indirectly through Load since
	// the Handlers map itself is unexported to pkg/manifest.
	supported := make(map[string]bool, len(fntag.ScaffoldYAML()))
	for _, tag := range fntag.ScaffoldYAML() {
		supported[tag] = true
	}

	for _, tag := range fntag.AllYAML() {
		data := []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  source: " + tag + " x\n")
		_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))

		if supported[tag] {
			assert.NotErrorIsf(t, err, errUtils.ErrUnsupportedYamlTag, "tag %s is in ScaffoldYAML but was rejected as unsupported", tag)
		} else {
			assert.ErrorIsf(t, err, errUtils.ErrUnsupportedYamlTag, "tag %s is not in ScaffoldYAML but was not rejected", tag)
		}
	}
}

// TestLoad_WithIncludeResolution_EnvTagResolves proves !env -- previously
// rejected (well, previously just left untouched since the old
// include-only walker never recognized it, which for an invalid-looking
// value would have surfaced as a schema validation failure) -- now resolves
// through scaffold.yaml the same way it does in a stack manifest.
func TestLoad_WithIncludeResolution_EnvTagResolves(t *testing.T) {
	registerTestKind(t)
	t.Setenv("ATMOS_TEST_SCAFFOLD_ENV_TAG", "hello-from-env")

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !env ATMOS_TEST_SCAFFOLD_ENV_TAG
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	assert.Equal(t, "hello-from-env", m.Spec.Source)
}

// TestLoad_WithIncludeResolution_ExecTagResolves proves !exec resolves.
func TestLoad_WithIncludeResolution_ExecTagResolves(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !exec echo scaffold-exec-ok
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	assert.Equal(t, "scaffold-exec-ok", m.Spec.Source)
}

// TestLoad_WithIncludeResolution_RandomTagResolves proves !random resolves
// into a real decoded value rather than being left as a literal tag string.
func TestLoad_WithIncludeResolution_RandomTagResolves(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  values:
    n: !random 1 2
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	n, ok := m.Spec.Values["n"].(int)
	require.True(t, ok, "expected an int, got %T: %v", m.Spec.Values["n"], m.Spec.Values["n"])
	assert.Contains(t, []int{1, 2}, n)
}

// TestLoad_WithIncludeResolution_CwdTagResolves proves !cwd resolves to a
// non-empty path rather than being left as a literal tag string.
func TestLoad_WithIncludeResolution_CwdTagResolves(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !cwd
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	assert.NotEmpty(t, m.Spec.Source)
}

// TestLoad_WithIncludeResolution_GitFamilyResolves proves the !git.*/
// !repo-root family resolves. Each use supplies a trailing default value
// per that tag's own documented fallback-on-failure contract (see
// pkg/git/yaml_tags.go), so the assertion holds whether or not the test
// happens to run inside a real git worktree: when real git info resolves,
// the result is simply some other non-empty string instead of the default.
func TestLoad_WithIncludeResolution_GitFamilyResolves(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "repo-root", tag: "!repo-root fallback-root"},
		{name: "git.root", tag: "!git.root fallback-root"},
		{name: "git.sha", tag: "!git.sha fallback-sha"},
		{name: "git.ref", tag: "!git.ref fallback-sha"},
		{name: "git.branch", tag: "!git.branch fallback-branch"},
		{name: "git.repository", tag: "!git.repository fallback-repo"},
		{name: "git.owner", tag: "!git.owner fallback-owner"},
		{name: "git.name", tag: "!git.name fallback-name"},
		{name: "git.host", tag: "!git.host fallback-host"},
		{name: "git.url", tag: "!git.url fallback-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerTestKind(t)

			dir := t.TempDir()
			data := []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  source: " + tt.tag + "\n")
			atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
			manifestFile := filepath.Join(dir, "manifest.yaml")

			m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
			require.NoError(t, err)
			assert.NotEmpty(t, m.Spec.Source)
		})
	}
}

// TestLoad_WithIncludeResolution_LiteralTagPreservesValue proves !literal
// survives untouched -- in particular, a value that looks like a Go
// template expression (which scaffold's own form-rendering would otherwise
// evaluate) is preserved exactly as written.
func TestLoad_WithIncludeResolution_LiteralTagPreservesValue(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !literal '{{ not_a_real_template_var }}'
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	m, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.NoError(t, err)
	assert.Equal(t, "{{ not_a_real_template_var }}", m.Spec.Source)
}

// TestLoad_WithIncludeResolution_UnsetRejected and
// TestLoad_WithIncludeResolution_AppendRejected prove !unset/!append --
// stack-inheritance-override concepts scaffold.yaml's field/answer merge has
// no equivalent for -- are rejected outright rather than silently accepted
// as a no-op or left inert.
func TestLoad_WithIncludeResolution_UnsetRejected(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !unset
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrUnsupportedYamlTag))
}

func TestLoad_WithIncludeResolution_AppendRejected(t *testing.T) {
	registerTestKind(t)

	dir := t.TempDir()
	data := []byte(`apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: !append foo
`)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	manifestFile := filepath.Join(dir, "manifest.yaml")

	_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrUnsupportedYamlTag))
}

// TestLoad_WithIncludeResolution_StackOnlyTagRejected proves every tag that
// genuinely needs real stack/component/backend context (unavailable while
// loading scaffold.yaml) is rejected with a clean, specific error -- never a
// panic, and never silently left as an unprocessed "<tag> <value>" string
// that would land unexpectedly in the decoded config.
func TestLoad_WithIncludeResolution_StackOnlyTagRejected(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "terraform.state", tag: "!terraform.state component stack .output"},
		{name: "terraform.output", tag: "!terraform.output component stack .output"},
		{name: "store", tag: "!store backend stack component key"},
		{name: "store.get", tag: "!store.get backend key"},
		{name: "secret", tag: "!secret my-secret"},
		{name: "cel", tag: "!cel 1 + 1"},
		{name: "aws.region", tag: "!aws.region"},
		{name: "template", tag: "!template {}"},
		{name: "emulator", tag: "!emulator aws"},
		{name: "version", tag: "!version"},
		{name: "tags", tag: "!tags"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerTestKind(t)

			dir := t.TempDir()
			data := []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  source: " + tt.tag + "\n")
			atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
			manifestFile := filepath.Join(dir, "manifest.yaml")

			_, err := Load[testSpec](testKind, data, WithIncludeResolution(atmosConfig, manifestFile, nil))
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrUnsupportedYamlTag))
		})
	}
}
