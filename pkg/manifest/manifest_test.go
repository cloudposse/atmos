package manifest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
