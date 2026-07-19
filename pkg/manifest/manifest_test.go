package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
)

// testSpec is a representative spec type exercising strings, lists, nested
// structs, and free-form maps.
type testSpec struct {
	Source string         `yaml:"source,omitempty" json:"source,omitempty"`
	Fields []testField    `yaml:"fields,omitempty" json:"fields,omitempty"`
	Values map[string]any `yaml:"values,omitempty" json:"values,omitempty"`
}

type testField struct {
	Name    string `yaml:"name" json:"name"`
	Type    string `yaml:"type,omitempty" json:"type,omitempty" jsonschema:"enum=input,enum=select,enum=confirm,enum=multiselect"`
	Default any    `yaml:"default,omitempty" json:"default,omitempty"`
}

const testKind = "AtmosTestConfig"

func registerTestKind(t *testing.T) {
	t.Helper()
	resetRegistry()
	t.Cleanup(resetRegistry)
	require.NoError(t, Register(testKind, DefaultAPIVersion, &testSpec{}))
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		apiVersion string
		prototype  any
		wantErr    error
	}{
		{
			name:       "valid registration",
			kind:       testKind,
			apiVersion: DefaultAPIVersion,
			prototype:  &testSpec{},
		},
		{
			name:       "empty apiVersion defaults to atmos/v1",
			kind:       testKind,
			apiVersion: "",
			prototype:  &testSpec{},
		},
		{
			name:      "empty kind rejected",
			kind:      "",
			prototype: &testSpec{},
			wantErr:   errUtils.ErrManifestKindEmpty,
		},
		{
			name:    "nil prototype rejected",
			kind:    testKind,
			wantErr: errUtils.ErrManifestPrototypeNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRegistry()
			t.Cleanup(resetRegistry)

			err := Register(tt.kind, tt.apiVersion, tt.prototype)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			def, ok := GetDefinition(tt.kind)
			require.True(t, ok)
			assert.Equal(t, tt.kind, def.Kind)
			assert.Equal(t, DefaultAPIVersion, def.APIVersion)
			assert.Contains(t, def.SchemaJSON(), `"const": "`+tt.kind+`"`)
		})
	}
}

func TestMustRegisterPanicsOnError(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	assert.Panics(t, func() {
		MustRegister("", "", &testSpec{})
	})
}

func TestKindsSorted(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	require.NoError(t, Register("ZetaKind", "", &testSpec{}))
	require.NoError(t, Register("AlphaKind", "", &testSpec{}))

	assert.Equal(t, []string{"AlphaKind", "ZetaKind"}, Kinds())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		wantKind       string
		wantAPIVersion string
		wantErr        error
	}{
		{
			name:           "valid envelope",
			data:           "apiVersion: atmos/v1\nkind: AtmosTestConfig\n",
			wantKind:       testKind,
			wantAPIVersion: DefaultAPIVersion,
		},
		{
			name: "missing envelope fields",
			data: "name: something\n",
		},
		{
			name:    "invalid yaml",
			data:    "kind: [unclosed",
			wantErr: errUtils.ErrManifestParse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, apiVersion, err := Detect([]byte(tt.data))
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantAPIVersion, apiVersion)
		})
	}
}

func TestValidate(t *testing.T) {
	validManifest := `
apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: simple
  description: A test manifest
  version: 1.0.0
spec:
  source: embedded
  fields:
    - name: project_name
      type: input
      default: my-project
  values:
    project_name: demo
`

	tests := []struct {
		name    string
		kind    string
		data    string
		wantErr error
	}{
		{
			name: "valid manifest",
			kind: testKind,
			data: validManifest,
		},
		{
			name:    "unknown kind",
			kind:    "NoSuchKind",
			data:    validManifest,
			wantErr: errUtils.ErrManifestKindUnknown,
		},
		{
			name:    "kind mismatch",
			kind:    testKind,
			data:    "apiVersion: atmos/v1\nkind: OtherKind\nmetadata:\n  name: x\n",
			wantErr: errUtils.ErrManifestKindMismatch,
		},
		{
			name:    "wrong apiVersion",
			kind:    testKind,
			data:    "apiVersion: atmos/v2\nkind: AtmosTestConfig\nmetadata:\n  name: x\n",
			wantErr: errUtils.ErrManifestAPIVersion,
		},
		{
			name:    "missing metadata",
			kind:    testKind,
			data:    "apiVersion: atmos/v1\nkind: AtmosTestConfig\n",
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name:    "missing metadata name",
			kind:    testKind,
			data:    "apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  description: no name\n",
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name: "unknown top-level property rejected",
			kind: testKind,
			data: validManifest + "\nextra: true\n",
			// The envelope schema sets additionalProperties: false.
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name:    "invalid enum value in spec",
			kind:    testKind,
			data:    "apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  fields:\n    - name: f\n      type: bogus\n",
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name:    "field missing required name",
			kind:    testKind,
			data:    "apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  fields:\n    - type: input\n",
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name:    "invalid yaml",
			kind:    testKind,
			data:    "kind: [unclosed",
			wantErr: errUtils.ErrManifestParse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerTestKind(t)

			err := Validate(tt.kind, []byte(tt.data))
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLoad(t *testing.T) {
	registerTestKind(t)

	data := []byte(`
apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: simple
  version: 2.0.0
spec:
  source: embedded
  fields:
    - name: project_name
      type: input
      default: my-project
  values:
    project_name: demo
`)

	m, err := Load[testSpec](testKind, data)
	require.NoError(t, err)

	assert.Equal(t, DefaultAPIVersion, m.APIVersion)
	assert.Equal(t, testKind, m.Kind)
	assert.Equal(t, "simple", m.Metadata.Name)
	assert.Equal(t, "2.0.0", m.Metadata.Version)
	assert.Equal(t, "embedded", m.Spec.Source)
	require.Len(t, m.Spec.Fields, 1)
	assert.Equal(t, "project_name", m.Spec.Fields[0].Name)
	assert.Equal(t, "input", m.Spec.Fields[0].Type)
	assert.Equal(t, "demo", m.Spec.Values["project_name"])
}

func TestLoadRejectsInvalidManifest(t *testing.T) {
	registerTestKind(t)

	_, err := Load[testSpec](testKind, []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\n"))
	assert.ErrorIs(t, err, errUtils.ErrManifestValidation)
}

// TestLoad_RejectsInterfaceTypedSpecMismatch covers reflect.TypeOf(zero)'s
// nil-for-interface-type gap: with S = any (an interface type), the zero
// value is a nil interface, and reflect.TypeOf on a nil interface returns
// nil — so the spec-type mismatch check would be silently skipped rather
// than catching a real mismatch against the registered testSpec type.
func TestLoad_RejectsInterfaceTypedSpecMismatch(t *testing.T) {
	registerTestKind(t)

	data := []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\n")

	_, err := Load[any](testKind, data)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestKindMismatch)
}

func TestSchemaJSONExportable(t *testing.T) {
	registerTestKind(t)

	def, ok := GetDefinition(testKind)
	require.True(t, ok)

	schema := def.SchemaJSON()
	assert.True(t, strings.HasPrefix(schema, "{"))
	assert.Contains(t, schema, `"$schema"`)
	assert.Contains(t, schema, `"apiVersion"`)
	assert.Contains(t, schema, `"metadata"`)
	assert.Contains(t, schema, `"spec"`)
	// Field ordering preserved as a list schema with items.
	assert.Contains(t, schema, `"fields"`)
}

const twoDocYAML = `apiVersion: v1
kind: Service
metadata:
  name: svc
  namespace: demo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dep
  namespace: demo
`

func TestDecodeObjects_MultiDoc(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	require.Len(t, objects, 2)
	assert.Equal(t, "Service", objects[0].GetKind())
	assert.Equal(t, "svc", objects[0].GetName())
	assert.Equal(t, "Deployment", objects[1].GetKind())
	assert.Equal(t, "dep", objects[1].GetName())
}

func TestDecodeObjects_MissingAPIVersion(t *testing.T) {
	_, err := DecodeObjects([]byte("kind: Service\nmetadata:\n  name: x\n"))
	assert.ErrorIs(t, err, errUtils.ErrManifestMissingAPIVersionKind)
}

func TestObjectFileName_Deterministic(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web", "namespace": "prod"},
	}}
	name := ObjectFileName(0, obj)
	assert.Equal(t, "001_apps_apps_v1_Deployment_prod_web.yaml", name)
}

func TestArtifactFiles(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	files, err := ArtifactFiles(objects)
	require.NoError(t, err)
	require.Len(t, files, 2)
	_, hasSvc := files["001_v1_Service_demo_svc.yaml"]
	_, hasDep := files["002_apps_apps_v1_Deployment_demo_dep.yaml"]
	assert.True(t, hasSvc)
	assert.True(t, hasDep)
}

func TestMultiDocumentYAML_RoundTrip(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	data, err := MultiDocumentYAML(objects)
	require.NoError(t, err)
	again, err := DecodeObjects(data)
	require.NoError(t, err)
	require.Len(t, again, 2)
	assert.Equal(t, "svc", again[0].GetName())
	assert.Equal(t, "dep", again[1].GetName())
}

func TestValidateRenderOptions(t *testing.T) {
	assert.NoError(t, ValidateRenderOptions(RenderOptions{}))
	assert.NoError(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml"}))
	assert.NoError(t, ValidateRenderOptions(RenderOptions{OutputDir: "d", Split: true}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml", OutputDir: "d"}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml", Split: true}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Split: true}))
}

func TestWriteObjects_SplitFiles(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, WriteObjects(objects, RenderOptions{OutputDir: dir, Split: true, Noun: "Helm"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "001_v1_Service_demo_svc.yaml", entries[0].Name())
	assert.Equal(t, "002_apps_apps_v1_Deployment_demo_dep.yaml", entries[1].Name())
}

func TestWriteObjects_SingleFile(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	dir := t.TempDir()
	out := filepath.Join(dir, "all.yaml")
	require.NoError(t, WriteObjects(objects, RenderOptions{Output: out, Noun: "Helm"}))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	decoded, err := DecodeObjects(data)
	require.NoError(t, err)
	require.Len(t, decoded, 2)
}
